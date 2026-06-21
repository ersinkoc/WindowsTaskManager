package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestNormalizeAnthropicEndpointEmpty(t *testing.T) {
	if got := normalizeAnthropicEndpoint(""); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeAnthropicEndpointAlreadyMessages(t *testing.T) {
	if got := normalizeAnthropicEndpoint("https://example.com/v1/messages"); got != "https://example.com/v1/messages" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeAnthropicEndpointBaseURLAppendsV1Messages(t *testing.T) {
	if got := normalizeAnthropicEndpoint("https://api.z.ai/api/anthropic"); got != "https://api.z.ai/api/anthropic/v1/messages" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeAnthropicEndpointTrailingSlash(t *testing.T) {
	if got := normalizeAnthropicEndpoint("https://example.com/v1/messages/"); got != "https://example.com/v1/messages" {
		t.Errorf("got %q", got)
	}
}

func TestAnthropicCallEmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "no text content") {
		t.Errorf("expected no-text-content error, got %v", err)
	}
}

func TestAnthropicCallFallsBackToNonTextType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First content block is non-text with empty text; second has text but type isn't "text".
		_, _ = w.Write([]byte(`{"content":[{"type":"thinking","text":""},{"type":"output","text":"fallback answer"}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	res, err := a.Analyze(context.Background(), "x")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Answer != "fallback answer" {
		t.Errorf("answer = %q", res.Answer)
	}
}

func TestAnthropicCallErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "rate_limit_error") {
		t.Errorf("expected anthropic error, got %v", err)
	}
}

func TestAnthropicCallHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "anthropic 400") {
		t.Errorf("expected anthropic 400 error, got %v", err)
	}
}

func TestAnthropicCallMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "parse response") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestAnthropicCallTransportError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: "http://127.0.0.1:1/v1/messages", // unreachable
		Model:    "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := a.Analyze(ctx, "x")
	if err == nil {
		t.Error("expected transport error")
	}
}

func TestAnthropicCallExtraHeaders(t *testing.T) {
	var seenExtra string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenExtra = r.Header.Get("X-Custom")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
		ExtraHeaders: map[string]string{"X-Custom": "yes"},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Analyze(context.Background(), "x"); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if seenExtra != "yes" {
		t.Errorf("X-Custom = %q", seenExtra)
	}
}

func TestAnthropicCallTruncatedErrorBody(t *testing.T) {
	// Response is HTTP error with a long body — the truncated text should be in the error.
	longBody := strings.Repeat("x", 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Errorf("expected truncated body marker, got %v", err)
	}
}

func TestAnthropicCallNewRequestError(t *testing.T) {
	// Use a URL with control characters to make http.NewRequestWithContext fail.
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: "http://[::1]:bad", Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil {
		t.Fatal("expected NewRequest error")
	}
}

func TestAnthropicCallReadBodyError(t *testing.T) {
	// Build a server that closes the connection mid-body. Use a flusher pattern.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Length", "999999")
		w.WriteHeader(200)
		// Write partial body then hijack & close
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"a`))
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil {
		t.Fatal("expected read error")
	}
}
