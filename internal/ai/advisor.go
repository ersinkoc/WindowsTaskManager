package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// Advisor wraps one or more LLM provider backends with caching and rate
// limiting. It implements the server.AIAdvisor interface
// (Enabled / Status / Analyze).
//
// Currently supported providers:
//
//   - "anthropic"            -> Anthropic Messages API ( /v1/messages )
//   - "openai"               -> any OpenAI-compatible /v1/chat/completions
//     endpoint (OpenAI, OpenRouter, Groq, DeepSeek,
//     Together, Mistral, Ollama, LM Studio, ...)
//
// The endpoint, model, api_key and extra_headers fields can all be
// overridden in the config so the user can point this at any provider
// without code changes.
type Advisor struct {
	mu         sync.RWMutex
	cfg        *config.Config
	store      *storage.Store
	alertsRef  func() []anomaly.Alert
	rl         *TokenBucket
	cache      *Cache
	httpClient *http.Client
	emitter    *event.Emitter

	lastErr          string
	lastReqAt        time.Time
	totalReqs        uint64
	totalCacheHits   uint64
	totalTokens      uint64
	promptTokens     uint64
	completionTokens uint64

	chatMu      sync.Mutex
	chatHistory []chatTurn

	bgMu sync.RWMutex
	bg   backgroundTracker

	rootCtx context.Context
}

type chatTurn struct {
	Role    string
	Content string
}

const maxProviderResponseBytes = 2 << 20

// NewAdvisor builds a new advisor. alertSource is a closure that returns
// the current active alerts for the prompt context.
func NewAdvisor(cfg *config.Config, store *storage.Store, alertSource func() []anomaly.Alert, emitter *event.Emitter) *Advisor {
	a := &Advisor{
		cfg:        cfg,
		store:      store,
		alertsRef:  alertSource,
		rl:         NewTokenBucket(cfg.AI.MaxRequestsPerMinute),
		cache:      NewCache(60*time.Second, 64),
		httpClient: &http.Client{Timeout: 60 * time.Second},
		emitter:    emitter,
		rootCtx:    context.Background(),
	}
	a.bg.applyConfig(cfg)
	if emitter != nil {
		emitter.On(anomaly.EventAlertRaised, a.handleRaisedAlert)
	}
	return a
}

// Start binds the advisor's background work to the application lifecycle.
func (a *Advisor) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Lock()
	a.rootCtx = ctx
	a.mu.Unlock()
}

// SetConfig hot-swaps the active config.
func (a *Advisor) SetConfig(cfg *config.Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
	a.rl = NewTokenBucket(cfg.AI.MaxRequestsPerMinute)
	a.bgMu.Lock()
	a.bg.applyConfig(cfg)
	a.bgMu.Unlock()
}

// Enabled reports whether the advisor is configured to run.
func (a *Advisor) Enabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.AI.Enabled && a.cfg.AI.APIKey != ""
}

// Status returns a JSON-friendly status snapshot.
func (a *Advisor) Status() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var cacheHitRate float64
	if a.totalReqs > 0 {
		cacheHitRate = float64(a.totalCacheHits) / float64(a.totalReqs) * 100
	}
	return map[string]any{
		"enabled":           a.cfg.AI.Enabled,
		"configured":        a.cfg.AI.APIKey != "",
		"provider":          providerOf(a.cfg),
		"endpoint":          effectiveEndpoint(a.cfg),
		"model":             a.cfg.AI.Model,
		"language":          a.cfg.AI.Language,
		"max_per_minute":    a.cfg.AI.MaxRequestsPerMinute,
		"tokens_available":  a.rl.Available(),
		"cache_size":        a.cache.Size(),
		"total_requests":    a.totalReqs,
		"cache_hits":        a.totalCacheHits,
		"cache_hit_rate":    cacheHitRate,
		"total_tokens":      a.totalTokens,
		"prompt_tokens":     a.promptTokens,
		"completion_tokens": a.completionTokens,
		"last_request":      a.lastReqAt.Format(time.RFC3339),
		"last_error":        a.lastErr,
	}
}

// Analyze runs an LLM call. The user-supplied question is appended to a
// snapshot+alerts context. Cached responses are returned without making
// an HTTP call. The returned AnalyzeResult carries both the prose answer
// (with any <actions> block stripped) and the parsed structured actions.
func (a *Advisor) Analyze(ctx context.Context, userQuestion string) (*AnalyzeResult, error) {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	return a.analyzeWithConfig(ctx, cfg, userQuestion)
}

func (a *Advisor) analyzeWithConfig(ctx context.Context, cfg *config.Config, userQuestion string) (*AnalyzeResult, error) {
	if cfg == nil || !cfg.AI.Enabled || cfg.AI.APIKey == "" {
		return nil, errors.New("AI advisor disabled")
	}

	snap := a.store.Latest()
	var alerts []anomaly.Alert
	if a.alertsRef != nil {
		alerts = a.alertsRef()
	}
	prompt := BuildPrompt(cfg.AI.Language, snap, alerts, cfg.AI.IncludeProcessTree, cfg.AI.IncludePortMap, userQuestion)
	return a.runPrompt(ctx, cfg, prompt)
}

// Chat runs a multi-turn AI exchange against the current system snapshot.
// Conversation history is kept in-memory and trimmed to the last 10 messages.
func (a *Advisor) Chat(ctx context.Context, userMessage string) (*AnalyzeResult, error) {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	if !cfg.AI.Enabled || cfg.AI.APIKey == "" {
		return nil, errors.New("AI advisor disabled")
	}
	if strings.TrimSpace(userMessage) == "" {
		return nil, errors.New("chat message required")
	}

	snap := a.store.Latest()
	var alerts []anomaly.Alert
	if a.alertsRef != nil {
		alerts = a.alertsRef()
	}

	a.chatMu.Lock()
	history := append([]chatTurn(nil), a.chatHistory...)
	a.chatMu.Unlock()

	prompt := BuildChatPrompt(cfg.AI.Language, snap, alerts, cfg.AI.IncludeProcessTree, cfg.AI.IncludePortMap, history, userMessage)
	resp, err := a.runPrompt(ctx, cfg, prompt)
	if err != nil {
		return nil, err
	}

	a.chatMu.Lock()
	a.chatHistory = append(a.chatHistory,
		chatTurn{Role: "user", Content: strings.TrimSpace(userMessage)},
		chatTurn{Role: "assistant", Content: strings.TrimSpace(resp.Answer)},
	)
	if len(a.chatHistory) > 10 {
		a.chatHistory = append([]chatTurn(nil), a.chatHistory[len(a.chatHistory)-10:]...)
	}
	a.chatMu.Unlock()
	return resp, nil
}

func (a *Advisor) runPrompt(ctx context.Context, cfg *config.Config, prompt string) (*AnalyzeResult, error) {
	if cached, ok := a.cache.Get(prompt); ok {
		a.mu.Lock()
		a.totalCacheHits++
		a.mu.Unlock()
		cleaned, actions := parseActionsBlock(cached)
		return &AnalyzeResult{Answer: cleaned, Actions: actions, Cached: true}, nil
	}

	if !a.rl.Take() {
		return nil, errors.New("AI rate limit exceeded; try again later")
	}

	answer, tokenUsage, err := a.callProvider(ctx, cfg, prompt)
	a.mu.Lock()
	a.totalReqs++
	a.lastReqAt = time.Now()
	if tokenUsage != nil {
		a.totalTokens += tokenUsage.Total
		a.promptTokens += tokenUsage.Prompt
		a.completionTokens += tokenUsage.Completion
	}
	if err != nil {
		a.lastErr = statusErrorMessage(err)
	} else {
		a.lastErr = ""
	}
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}

	a.cache.Set(prompt, answer)
	cleaned, actions := parseActionsBlock(answer)
	return &AnalyzeResult{Answer: cleaned, Actions: actions}, nil
}

// TokenUsage holds token counts from a provider response.
type TokenUsage struct {
	Prompt     uint64
	Completion uint64
	Total      uint64
}

// callProvider dispatches the LLM call based on cfg.AI.Provider.
func (a *Advisor) callProvider(ctx context.Context, cfg *config.Config, prompt string) (string, *TokenUsage, error) {
	switch providerOf(cfg) {
	case "anthropic":
		ans, usage, err := a.callAnthropic(ctx, cfg, prompt)
		return ans, usage, err
	case "openai":
		ans, usage, err := a.callOpenAI(ctx, cfg, prompt)
		return ans, usage, err
	default:
		return "", nil, fmt.Errorf("unknown AI provider %q (use 'anthropic' or 'openai')", cfg.AI.Provider)
	}
}

// providerOf normalizes the provider name. Empty string defaults to anthropic
// for backwards compatibility with older configs.
func providerOf(cfg *config.Config) string {
	p := strings.ToLower(strings.TrimSpace(cfg.AI.Provider))
	switch p {
	case "":
		return "anthropic"
	case "openai-compatible", "openai_compatible", "openrouter", "groq",
		"deepseek", "together", "mistral", "ollama", "lmstudio", "lm-studio":
		return "openai"
	}
	return p
}

// effectiveEndpoint returns the URL the advisor will actually hit, factoring
// in the provider default when the user left endpoint blank.
func effectiveEndpoint(cfg *config.Config) string {
	if cfg.AI.Endpoint != "" {
		return cfg.AI.Endpoint
	}
	switch providerOf(cfg) {
	case "anthropic":
		return "https://api.anthropic.com/v1/messages"
	case "openai":
		return "https://api.openai.com/v1/chat/completions"
	}
	return ""
}

// BackgroundState returns the scheduler's current status plus recent runs.
func (a *Advisor) BackgroundState() BackgroundState {
	a.mu.RLock()
	cfg := a.cfg
	configured := a.cfg != nil && a.cfg.AI.Enabled && a.cfg.AI.APIKey != ""
	a.mu.RUnlock()

	a.bgMu.RLock()
	defer a.bgMu.RUnlock()
	return a.bg.snapshot(cfg, configured)
}

func (a *Advisor) backgroundContext() context.Context {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.rootCtx != nil {
		return a.rootCtx
	}
	return context.Background()
}

func readProviderBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxProviderResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxProviderResponseBytes {
		return nil, fmt.Errorf("provider response exceeds %d bytes", maxProviderResponseBytes)
	}
	return body, nil
}

func statusErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate limit"):
		return "AI rate limit exceeded; try again later"
	case strings.Contains(msg, "disabled"):
		return "AI advisor disabled"
	default:
		return "AI provider request failed"
	}
}
