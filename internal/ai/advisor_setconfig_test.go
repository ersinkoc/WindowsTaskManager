package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// newStubAnthropicServer returns a provider stub that answers every request
// with a minimal valid Anthropic Messages response.
func newStubAnthropicServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestSetConfigPreservesRateLimitAcrossUnchangedReload pins the contract
// that Advisor.SetConfig — which fires on every config save (any section)
// via applyConfig — must not refill the per-minute rate limiter when
// cfg.AI.MaxRequestsPerMinute is unchanged. Before the fix, an unrelated
// settings save handed out a fresh bucket, making the cost guard bypassable
// by alternating a config PUT with an AI call.
func TestSetConfigPreservesRateLimitAcrossUnchangedReload(t *testing.T) {
	srv := newStubAnthropicServer(t)

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.APIKey = "test-key"
	cfg.AI.Provider = "anthropic"
	cfg.AI.Endpoint = srv.URL + "/messages" // suffix kept by normalizeAnthropicEndpoint
	cfg.AI.MaxRequestsPerMinute = 2

	advisor := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	ctx := context.Background()

	for i, q := range []string{"question one", "question two"} {
		if _, err := advisor.Analyze(ctx, q); err != nil {
			t.Fatalf("call #%d should succeed against the stub provider: %v", i+1, err)
		}
	}
	if _, err := advisor.Analyze(ctx, "question three"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("third call within the same minute should be rate-limited, got err=%v", err)
	}

	reloaded := *cfg // identical AI settings, as produced by cloneConfig on every save
	advisor.SetConfig(&reloaded)

	if _, err := advisor.Analyze(ctx, "question four"); err == nil {
		t.Fatal("unchanged config reload must not refill the rate limiter: fourth call succeeded inside the same minute window")
	} else if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("fourth call failed for an unexpected reason (want rate limit): %v", err)
	}
}

// TestSetConfigRebuildsRateLimitOnLimitChange covers the other branch: when
// the per-minute limit genuinely changes, the bucket is rebuilt with the new
// capacity (a raised cap takes effect immediately rather than staying
// pinned to the old depleted bucket).
func TestSetConfigRebuildsRateLimitOnLimitChange(t *testing.T) {
	srv := newStubAnthropicServer(t)

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.APIKey = "test-key"
	cfg.AI.Provider = "anthropic"
	cfg.AI.Endpoint = srv.URL + "/messages"
	cfg.AI.MaxRequestsPerMinute = 1

	advisor := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	ctx := context.Background()

	if _, err := advisor.Analyze(ctx, "first question"); err != nil {
		t.Fatalf("first call should succeed: %v", err)
	}
	if _, err := advisor.Analyze(ctx, "second question"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("second call should be rate-limited under MaxRequestsPerMinute=1, got err=%v", err)
	}

	raised := *cfg
	raised.AI.MaxRequestsPerMinute = 3
	advisor.SetConfig(&raised)

	if _, err := advisor.Analyze(ctx, "third question"); err != nil {
		t.Fatalf("raising the limit should rebuild the bucket and allow another call: %v", err)
	}
	if _, err := advisor.Analyze(ctx, "fourth question"); err != nil {
		t.Fatalf("raised cap of 3 should still have budget: %v", err)
	}
	if _, err := advisor.Analyze(ctx, "fifth question"); err != nil {
		t.Fatalf("a rebuilt bucket of 3 allows exactly three post-rebuild calls: %v", err)
	}
	if _, err := advisor.Analyze(ctx, "sixth question"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("raised cap of 3 must be enforced after three post-rebuild calls, got err=%v", err)
	}
}
