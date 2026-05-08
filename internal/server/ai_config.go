package server

import (
	"net/http"
	"strings"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
)

// aiConfigDTO is the JSON shape exchanged with the dashboard. We expose only
// the fields that make sense to mutate from the UI; structural fields stay
// in the YAML file.
type aiConfigDTO struct {
	Enabled              bool              `json:"enabled"`
	Provider             string            `json:"provider"`
	APIKey               string            `json:"api_key"`
	Model                string            `json:"model"`
	Endpoint             string            `json:"endpoint"`
	ExtraHeaders         map[string]string `json:"extra_headers"`
	Language             string            `json:"language"`
	MaxTokens            int               `json:"max_tokens"`
	MaxRequestsPerMinute int               `json:"max_requests_per_minute"`
	IncludeProcessTree   bool              `json:"include_process_tree"`
	IncludePortMap       bool              `json:"include_port_map"`
}

// aiPreset describes a one-click provider example shown in the UI. The user
// still needs to paste their own api key.
type aiPreset struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Provider     string            `json:"provider"`
	Endpoint     string            `json:"endpoint"`
	Model        string            `json:"model"`
	APIKeyHint   string            `json:"api_key_hint"`
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
	Notes        string            `json:"notes,omitempty"`
}

// aiPresets is the curated list of provider/model combinations the dashboard
// offers as starter templates. Adding a new one is a one-line change.
// #nosec G101 -- APIKeyHint contains public key formats/examples, not secrets.
var aiPresets = []aiPreset{
	{
		ID:         "anthropic",
		Label:      "Anthropic Claude",
		Provider:   "anthropic",
		Endpoint:   "https://api.anthropic.com/v1/messages",
		Model:      "claude-sonnet-4-20250514",
		APIKeyHint: "sk-ant-...",
	},
	{
		ID:         "zai",
		Label:      "Z.AI / Zhipu (Anthropic-compatible)",
		Provider:   "anthropic",
		Endpoint:   "https://api.z.ai/api/anthropic",
		Model:      "glm-5.1",
		APIKeyHint: "z.ai api key",
		Notes:      "Z.AI coding plans expose an Anthropic-compatible endpoint. Popular models: glm-5.1, glm-5-turbo, glm-4.7, glm-4.6. The alternate paas endpoint https://api.z.ai/api/paas/v4/v1/messages also works.",
	},
	{
		ID:         "openai",
		Label:      "OpenAI GPT",
		Provider:   "openai",
		Endpoint:   "https://api.openai.com/v1/chat/completions",
		Model:      "gpt-5-mini",
		APIKeyHint: "sk-...",
	},
	{
		ID:         "openrouter",
		Label:      "OpenRouter (multi-model)",
		Provider:   "openai",
		Endpoint:   "https://openrouter.ai/api/v1/chat/completions",
		Model:      "openrouter/auto",
		APIKeyHint: "sk-or-v1-...",
		ExtraHeaders: map[string]string{
			"HTTP-Referer": "http://localhost",
			"X-Title":      "WTM",
		},
		Notes: "Single key, hundreds of models. openrouter/auto routes to current top models; browse https://openrouter.ai/models for explicit picks.",
	},
	{
		ID:         "groq",
		Label:      "Groq (Llama / Mixtral, fast)",
		Provider:   "openai",
		Endpoint:   "https://api.groq.com/openai/v1/chat/completions",
		Model:      "llama-3.3-70b-versatile",
		APIKeyHint: "gsk_...",
	},
	{
		ID:         "deepseek",
		Label:      "DeepSeek",
		Provider:   "openai",
		Endpoint:   "https://api.deepseek.com/v1/chat/completions",
		Model:      "deepseek-chat",
		APIKeyHint: "sk-...",
	},
	{
		ID:         "together",
		Label:      "Together AI",
		Provider:   "openai",
		Endpoint:   "https://api.together.xyz/v1/chat/completions",
		Model:      "moonshotai/Kimi-K2.5",
		APIKeyHint: "...",
	},
	{
		ID:         "mistral",
		Label:      "Mistral La Plateforme",
		Provider:   "openai",
		Endpoint:   "https://api.mistral.ai/v1/chat/completions",
		Model:      "mistral-medium-2508",
		APIKeyHint: "...",
	},
	{
		ID:         "fireworks",
		Label:      "Fireworks AI",
		Provider:   "openai",
		Endpoint:   "https://api.fireworks.ai/inference/v1/chat/completions",
		Model:      "accounts/fireworks/models/llama-v3p3-70b-instruct",
		APIKeyHint: "fw_...",
	},
	{
		ID:         "xai",
		Label:      "xAI Grok",
		Provider:   "openai",
		Endpoint:   "https://api.x.ai/v1/chat/completions",
		Model:      "grok-4.20-reasoning",
		APIKeyHint: "xai-...",
	},
	{
		ID:         "ollama",
		Label:      "Ollama (local)",
		Provider:   "openai",
		Endpoint:   "http://localhost:11434/v1/chat/completions",
		Model:      "llama3.1",
		APIKeyHint: "ollama (any value)",
		Notes:      "Run `ollama serve` then `ollama pull llama3.1`",
	},
	{
		ID:         "lmstudio",
		Label:      "LM Studio (local)",
		Provider:   "openai",
		Endpoint:   "http://localhost:1234/v1/chat/completions",
		Model:      "local-model",
		APIKeyHint: "lm-studio (any value)",
		Notes:      "Start LM Studio's local server and load a model",
	},
	{
		ID:         "custom",
		Label:      "Custom OpenAI-compatible",
		Provider:   "openai",
		Endpoint:   "",
		Model:      "",
		APIKeyHint: "",
		Notes:      "Any service exposing /v1/chat/completions",
	},
}

func (s *Server) handleAIPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, aiPresets)
}

func (s *Server) handleAIConfigGet(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, dtoFromConfig(&cfg.AI))
}

func (s *Server) handleAIConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if s.cfgPath == "" {
		writeError(w, http.StatusServiceUnavailable, "no_config", "config file path not set")
		return
	}

	var body aiConfigDTO
	if !readJSON(w, r, &body) {
		return
	}

	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider == "" {
		provider = "anthropic"
	}
	if provider != "anthropic" && provider != "openai" {
		writeError(w, http.StatusBadRequest, "bad_provider", "provider must be 'anthropic' or 'openai'")
		return
	}

	s.mu.RLock()
	current := cloneConfig(s.cfg)
	s.mu.RUnlock()

	// Build the next config: clone the current one, then overwrite the AI block.
	next := current
	next.AI.Enabled = body.Enabled
	next.AI.Provider = provider
	if body.APIKey != "" {
		next.AI.APIKey = body.APIKey
	}
	next.AI.Model = strings.TrimSpace(body.Model)
	next.AI.Endpoint = strings.TrimSpace(body.Endpoint)
	if body.ExtraHeaders != nil {
		next.AI.ExtraHeaders = body.ExtraHeaders
	}
	if body.Language != "" {
		next.AI.Language = body.Language
	}
	if body.MaxTokens > 0 {
		next.AI.MaxTokens = body.MaxTokens
	}
	if body.MaxRequestsPerMinute > 0 {
		next.AI.MaxRequestsPerMinute = body.MaxRequestsPerMinute
	}
	next.AI.IncludeProcessTree = body.IncludeProcessTree
	next.AI.IncludePortMap = body.IncludePortMap

	if err := next.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_config", err.Error())
		return
	}

	if err := config.Save(s.cfgPath, &next); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}

	// Apply in-process immediately so the response reflects the new state
	// without waiting on the file watcher's poll interval.
	s.SetConfig(&next)
	if s.onCfgApply != nil {
		s.onCfgApply(&next)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"config": dtoFromConfig(&next.AI),
	})
}

func dtoFromConfig(ai *config.AIConfig) aiConfigDTO {
	return aiConfigDTO{
		Enabled:              ai.Enabled,
		Provider:             ai.Provider,
		APIKey:               maskSecret(ai.APIKey),
		Model:                ai.Model,
		Endpoint:             ai.Endpoint,
		ExtraHeaders:         redactHeaderValues(ai.ExtraHeaders),
		Language:             ai.Language,
		MaxTokens:            ai.MaxTokens,
		MaxRequestsPerMinute: ai.MaxRequestsPerMinute,
		IncludeProcessTree:   ai.IncludeProcessTree,
		IncludePortMap:       ai.IncludePortMap,
	}
}

// maskSecret keeps the last 4 chars so the UI can show "configured" without
// echoing the full key back over HTTP.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}

func redactHeaderValues(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		if isSensitiveHeaderKey(k) {
			out[k] = maskSecret(v)
			continue
		}
		out[k] = v
	}
	return out
}

func isSensitiveHeaderKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch {
	case k == "authorization":
		return true
	case k == "proxy-authorization":
		return true
	case strings.Contains(k, "api-key"):
		return true
	case strings.Contains(k, "api_key"):
		return true
	case strings.HasSuffix(k, "key"):
		return true
	case strings.Contains(k, "token"):
		return true
	case strings.Contains(k, "secret"):
		return true
	default:
		return false
	}
}

func (s *Server) handleAITest(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	if cfg.AI.APIKey == "" {
		writeError(w, http.StatusBadRequest, "no_api_key", "no API key configured")
		return
	}
	if cfg.AI.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "no_endpoint", "no endpoint configured")
		return
	}
	if !cfg.AI.Enabled {
		writeError(w, http.StatusBadRequest, "ai_disabled", "AI is disabled")
		return
	}

	ctx := r.Context()
	result, err := s.advisor.Chat(ctx, "Reply with exactly the word 'ok' if you can read this.")
	if err != nil {
		writeError(w, http.StatusBadGateway, "ai_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "response": result.Answer})
}
