package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestAdvisorEnabledRequiresKey(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{Enabled: true, MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	if a.Enabled() {
		t.Error("expected false when APIKey is empty")
	}
	a.SetConfig(&config.Config{AI: config.AIConfig{Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5}})
	if !a.Enabled() {
		t.Error("expected true when enabled and key set")
	}
	a.SetConfig(&config.Config{AI: config.AIConfig{Enabled: false, APIKey: "k", MaxRequestsPerMinute: 5}})
	if a.Enabled() {
		t.Error("expected false when disabled")
	}
}

func TestAdvisorStatusPopulated(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		APIKey:               "k",
		Provider:             "openai",
		Endpoint:             "",
		Model:                "gpt-x",
		Language:             "en",
		MaxRequestsPerMinute: 7,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	st := a.Status()
	if st["enabled"] != true {
		t.Errorf("enabled = %v", st["enabled"])
	}
	if st["configured"] != true {
		t.Errorf("configured = %v", st["configured"])
	}
	if st["provider"] != "openai" {
		t.Errorf("provider = %v", st["provider"])
	}
	if st["endpoint"] != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("endpoint = %v", st["endpoint"])
	}
	if st["model"] != "gpt-x" {
		t.Errorf("model = %v", st["model"])
	}
	if st["language"] != "en" {
		t.Errorf("language = %v", st["language"])
	}
	if st["max_per_minute"] != 7 {
		t.Errorf("max_per_minute = %v", st["max_per_minute"])
	}
	if st["cache_size"] != 0 {
		t.Errorf("cache_size = %v", st["cache_size"])
	}
	if st["total_requests"] != uint64(0) {
		t.Errorf("total_requests = %v", st["total_requests"])
	}
	if st["cache_hits"] != uint64(0) {
		t.Errorf("cache_hits = %v", st["cache_hits"])
	}
	if c, ok := st["cache_hit_rate"].(float64); !ok || c != 0 {
		t.Errorf("cache_hit_rate = %v (%T)", st["cache_hit_rate"], st["cache_hit_rate"])
	}
	if st["last_error"] != "" {
		t.Errorf("last_error = %v", st["last_error"])
	}
}

func TestAdvisorStatusCacheHitRateNonZero(t *testing.T) {
	// Run a cached call to bump cache_hits and total_requests.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Analyze(context.Background(), "q"); err != nil {
		t.Fatalf("first Analyze: %v", err)
	}
	if _, err := a.Analyze(context.Background(), "q"); err != nil {
		t.Fatalf("second Analyze (cached): %v", err)
	}
	st := a.Status()
	if st["total_requests"].(uint64) < 1 {
		t.Errorf("total_requests = %v", st["total_requests"])
	}
	if st["cache_hits"].(uint64) < 1 {
		t.Errorf("cache_hits = %v", st["cache_hits"])
	}
}

func TestStartNilContextFallsBackToBackground(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	a.Start(nil)
	got := a.backgroundContext()
	if got == nil {
		t.Fatal("backgroundContext returned nil")
	}
	if got.Err() != nil {
		t.Errorf("backgroundContext already done: %v", got.Err())
	}
}

func TestStartWithRealContext(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.Start(ctx)
	got := a.backgroundContext()
	if got != ctx {
		t.Error("backgroundContext should return the stored context")
	}
}

func TestBackgroundContextNilRootFallback(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	// Force rootCtx to nil to exercise the fallback branch (dead code in practice
	// because NewAdvisor always sets it to context.Background()).
	a.mu.Lock()
	a.rootCtx = nil
	a.mu.Unlock()
	got := a.backgroundContext()
	if got == nil {
		t.Fatal("backgroundContext returned nil")
	}
	if got.Err() != nil {
		t.Errorf("backgroundContext already done: %v", got.Err())
	}
}

func TestChatEmptyMessageReturnsError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		MaxTokens: 64, MaxRequestsPerMinute: 60,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Chat(context.Background(), "   "); err == nil || !strings.Contains(err.Error(), "chat message required") {
		t.Errorf("expected chat message required error, got %v", err)
	}
}

func TestChatDisabledReturnsError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: false, MaxRequestsPerMinute: 5,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Chat(context.Background(), "hi"); err == nil {
		t.Error("expected error when disabled")
	}
}

func TestChatHistoryTrimmedAt10(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"x"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 600, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	// Send 12 messages - history should keep only last 10.
	for i := 0; i < 12; i++ {
		if _, err := a.Chat(context.Background(), "msg"); err != nil {
			t.Fatalf("chat %d: %v", i, err)
		}
	}
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if len(a.chatHistory) != 10 {
		t.Errorf("chatHistory length = %d, want 10 (capped)", len(a.chatHistory))
	}
}

func TestRunPromptRateLimitExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 1, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	// First call succeeds.
	if _, err := a.Analyze(context.Background(), "first"); err != nil {
		t.Fatalf("first: %v", err)
	}
	// Second call from cache (different prompt is OK; same prompt hits cache).
	// To trigger rate limit, use a unique prompt that won't hit cache.
	for i := 0; i < 5; i++ {
		_, err := a.Analyze(context.Background(), "trigger-rate-limit")
		if err != nil {
			// First should succeed; subsequent may hit rate limit OR return cached.
			if strings.Contains(err.Error(), "rate limit exceeded") {
				return
			}
		}
	}
	// Even if we didn't hit the explicit rate limit path, the test isn't broken —
	// but we should at least verify the path by exhausting the bucket directly.
	a.rl.Take() // drain remaining
	_, err := a.Analyze(context.Background(), "after-drain")
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Errorf("expected rate limit error after drain, got %v", err)
	}
}

func TestRunPromptRecordsLastErrorOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	st := a.Status()
	if st["last_error"] == "" {
		t.Error("expected last_error to be set")
	}
}

func TestStatusErrorMessage(t *testing.T) {
	if got := statusErrorMessage(nil); got != "" {
		t.Errorf("nil err: got %q, want empty", got)
	}
	if got := statusErrorMessage(errors.New("rate limit exceeded")); got != "AI rate limit exceeded; try again later" {
		t.Errorf("rate limit: got %q", got)
	}
	if got := statusErrorMessage(errors.New("AI advisor disabled")); got != "AI advisor disabled" {
		t.Errorf("disabled: got %q", got)
	}
	if got := statusErrorMessage(errors.New("something else failed")); got != "AI provider request failed" {
		t.Errorf("generic: got %q", got)
	}
	// Case-insensitive match
	if got := statusErrorMessage(errors.New("RATE LIMIT ERROR")); got != "AI rate limit exceeded; try again later" {
		t.Errorf("uppercase rate limit: got %q", got)
	}
}

func TestReadProviderBodyUnderLimit(t *testing.T) {
	body, err := readProviderBody(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("got %q", body)
	}
}

func TestReadProviderBodyExactLimit(t *testing.T) {
	in := strings.Repeat("x", maxProviderResponseBytes)
	body, err := readProviderBody(strings.NewReader(in))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(body) != maxProviderResponseBytes {
		t.Errorf("got len %d, want %d", len(body), maxProviderResponseBytes)
	}
}

func TestReadProviderBodyOverLimit(t *testing.T) {
	in := strings.Repeat("x", maxProviderResponseBytes+10)
	_, err := readProviderBody(strings.NewReader(in))
	if err == nil || !strings.Contains(err.Error(), "provider response exceeds") {
		t.Errorf("expected oversize error, got %v", err)
	}
}

func TestReadProviderBodyReadError(t *testing.T) {
	// io.ErrUnexpectedEOF wrapped reader
	r := &errReader{err: io.ErrUnexpectedEOF}
	if _, err := readProviderBody(r); err == nil {
		t.Error("expected read error")
	}
}

type errReader struct {
	err error
}

func (e *errReader) Read(p []byte) (int, error) {
	return 0, e.err
}

func TestAnalyzeWithNilConfig(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		MaxTokens: 64, MaxRequestsPerMinute: 60,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.analyzeWithConfig(context.Background(), nil, "x"); err == nil {
		t.Error("expected error when cfg is nil")
	}
}

func TestAnalyzeWithAlertsSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	called := false
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert {
		called = true
		return []anomaly.Alert{{Type: "x", Severity: anomaly.SeverityWarning, Title: "y"}}
	}, nil)
	if _, err := a.Analyze(context.Background(), "x"); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if !called {
		t.Error("expected alertsRef to be called")
	}
}

func TestBackgroundStateWhenCfgNil(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	a.mu.Lock()
	a.cfg = nil
	a.mu.Unlock()
	st := a.BackgroundState()
	if st.Configured {
		t.Errorf("expected Configured=false when cfg nil, got %+v", st)
	}
	if st.LastRun != nil {
		t.Errorf("expected nil LastRun, got %+v", st.LastRun)
	}
}

func TestRunPromptCacheHitPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"cache me"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	res, err := a.Analyze(context.Background(), "same")
	if err != nil {
		t.Fatalf("first Analyze: %v", err)
	}
	if res.Cached {
		t.Error("first call should not be cached")
	}
	res2, err := a.Analyze(context.Background(), "same")
	if err != nil {
		t.Fatalf("second Analyze: %v", err)
	}
	if !res2.Cached {
		t.Error("second call should be cached")
	}
	if a.Status()["cache_hits"].(uint64) == 0 {
		t.Error("expected cache_hits > 0")
	}
}

func TestAnthropicCallRecordsUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":42,"output_tokens":7}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "claude-test", MaxTokens: 128, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Analyze(context.Background(), "x"); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	st := a.Status()
	if st["prompt_tokens"].(uint64) != 42 {
		t.Errorf("prompt_tokens = %v", st["prompt_tokens"])
	}
	if st["completion_tokens"].(uint64) != 7 {
		t.Errorf("completion_tokens = %v", st["completion_tokens"])
	}
	if st["total_tokens"].(uint64) != 49 {
		t.Errorf("total_tokens = %v", st["total_tokens"])
	}
}

func TestRunPromptFailureUpdatesLastReqAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`nope`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	before := a.lastReqAt
	time.Sleep(10 * time.Millisecond)
	if _, err := a.Analyze(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
	if !a.lastReqAt.After(before) {
		t.Error("expected lastReqAt to advance after failed call")
	}
}

func TestChatRunPromptErrorPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Chat(context.Background(), "hi"); err == nil {
		t.Fatal("expected error")
	}
	// History should NOT have been updated on error.
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if len(a.chatHistory) != 0 {
		t.Errorf("expected empty history on error, got %d entries", len(a.chatHistory))
	}
}

func TestEffectiveEndpointUnknownProvider(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Provider: "weird-provider", Endpoint: ""}}
	if got := effectiveEndpoint(cfg); got != "" {
		t.Errorf("expected empty string for unknown provider, got %q", got)
	}
}
