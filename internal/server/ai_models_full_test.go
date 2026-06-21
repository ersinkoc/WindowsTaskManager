package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
)

func TestInferFormatBranches(t *testing.T) {
	cases := []struct {
		npm, api, pid string
		want          string
	}{
		{"anthropic", "", "", "anthropic"},
		{"", "https://api.anthropic.com", "", "anthropic"},
		{"", "", "anthropic", "anthropic"},
		{"z-ai", "", "zhipu", "anthropic"},
		{"openai", "https://api.openai.com", "openai", "openai"},
		{"", "https://api.mistral.ai", "mistral", "openai"},
		{"", "https://api.google.com", "google", "openai"},
		{"something-else", "", "elsewhere", "openai"},
	}
	for _, c := range cases {
		if got := inferFormat(c.npm, c.api, c.pid); got != c.want {
			t.Errorf("inferFormat(%q,%q,%q)=%q want %q", c.npm, c.api, c.pid, got, c.want)
		}
	}
}

func TestNormalizedEndpointBranches(t *testing.T) {
	cases := []struct {
		api, format, want string
	}{
		{"", "openai", ""},
		{"https://api.openai.com/v1/chat/completions", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.openai.com/", "openai", "https://api.openai.com/v1/chat/completions"},
		{"https://api.anthropic.com/v1/messages", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"https://api.anthropic.com/messages", "anthropic", "https://api.anthropic.com/messages"},
		{"https://example.com/anthropic", "anthropic", "https://example.com/anthropic/v1/messages"},
		{"https://example.com/v1", "anthropic", "https://example.com/v1/messages"},
		{"https://example.com", "anthropic", "https://example.com/v1/messages"},
		{"https://unknown.example", "mystery", "https://unknown.example"},
	}
	for _, c := range cases {
		got := normalizedEndpoint(c.api, c.format)
		if got != c.want {
			t.Errorf("normalizedEndpoint(%q,%q)=%q want %q", c.api, c.format, got, c.want)
		}
	}
}

func TestParseModelsDev(t *testing.T) {
	body := []byte(`{
        "anthropic": {
            "id": "anthropic",
            "name": "Anthropic",
            "api": "https://api.anthropic.com",
            "npm": "@anthropic-ai/sdk",
            "models": {
                "claude-3": {"id": "claude-3", "name": "Claude 3", "limit": {"context": 200000, "output": 4096}}
            }
        },
        "openai": {
            "id": "openai",
            "name": "OpenAI",
            "api": "https://api.openai.com/v1/chat/completions",
            "npm": "openai",
            "models": {
                "gpt-4": {"id": "gpt-4", "name": "GPT-4", "limit": {"context": 8192, "output": 2048}},
                "gpt-3.5": {"id": "gpt-3.5", "limit": {"context": 4096, "output": 1024}}
            }
        },
        "z.ai": {
            "id": "z.ai",
            "name": "Z.AI",
            "api": "https://api.z.ai/api/anthropic",
            "npm": "",
            "models": {
                "glm-5.1": {"id": "glm-5.1", "name": "GLM-5.1", "limit": {}}
            }
        }
    }`)
	out, err := parseModelsDev(body)
	if err != nil {
		t.Fatalf("parseModelsDev: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("expected 4 models, got %d", len(out))
	}
	// Check sorted by provider then ID.
	for i := 1; i < len(out); i++ {
		if out[i-1].Provider > out[i].Provider {
			t.Errorf("not sorted at index %d: %q > %q", i, out[i-1].Provider, out[i].Provider)
		}
	}
	// Find a specific entry.
	found := false
	for _, m := range out {
		if m.ID == "claude-3" {
			found = true
			if m.Format != "anthropic" {
				t.Errorf("claude-3 format=%q", m.Format)
			}
			if m.Context != 200000 {
				t.Errorf("claude-3 context=%d", m.Context)
			}
		}
	}
	if !found {
		t.Error("claude-3 missing")
	}
}

func TestParseModelsDevBadJSON(t *testing.T) {
	_, err := parseModelsDev([]byte("not json"))
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestModelsCacheGetAndRefresh(t *testing.T) {
	// Use a fresh cache to avoid interference from other tests sharing state.
	c := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: time.Second},
	}
	// Empty cache → triggers refresh.
	snap := c.get()
	if snap.data != nil {
		t.Errorf("expected nil data on fresh cache, got %v", snap.data)
	}

	// Manually populate the cache and confirm get returns it without
	// triggering another refresh.
	c.mu.Lock()
	c.data = []modelInfo{{ID: "m1"}}
	c.loadedAt = time.Now()
	c.mu.Unlock()
	snap = c.get()
	if len(snap.data) != 1 || snap.data[0].ID != "m1" {
		t.Errorf("snap.data=%+v", snap.data)
	}
}

func TestModelsCacheRefreshSuccessAndErrors(t *testing.T) {
	// Success path: mock upstream returns a tiny but valid document.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"openai":{"id":"openai","name":"OpenAI","api":"https://api.openai.com/v1","npm":"openai","models":{"gpt-4":{"id":"gpt-4","name":"GPT-4"}}}}`))
	}))
	defer ts.Close()

	// Override modelsDevURL by constructing a fresh cache that points at the test server.
	// We do this by writing a custom refresh function on a tiny struct; but since
	// modelsDevURL is a package-level const we need a different approach.
	// Instead, exercise refresh directly with our own URL by faking the HTTP roundtrip.
	// Simplest: override sharedModelsCache.client.Transport with one that rewrites the URL.
	c := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: 2 * time.Second, Transport: rewriteTransport{from: modelsDevURL, to: ts.URL + "/api.json"}},
	}
	c.refresh()
	c.mu.Lock()
	if len(c.data) != 1 || c.data[0].ID != "gpt-4" {
		t.Errorf("expected gpt-4, got %+v", c.data)
	}
	if c.lastError != "" {
		t.Errorf("lastError=%q", c.lastError)
	}
	c.mu.Unlock()

	// HTTP 500 from upstream
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts2.Close()
	c2 := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: 2 * time.Second, Transport: rewriteTransport{from: modelsDevURL, to: ts2.URL + "/api.json"}},
	}
	c2.refresh()
	c2.mu.Lock()
	if c2.lastError == "" || !strings.Contains(c2.lastError, "status 500") {
		t.Errorf("expected status 500 error, got %q", c2.lastError)
	}
	c2.mu.Unlock()

	// Network failure → connection refused.
	c3 := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: 500 * time.Millisecond},
	}
	// Point modelsDevURL effectively at an unreachable address by re-routing via a
	// transport that fails immediately.
	c3.client = &http.Client{Timeout: 200 * time.Millisecond, Transport: failingTransport{}}
	c3.refresh()
	c3.mu.Lock()
	if c3.lastError == "" {
		t.Error("expected error from failing transport")
	}
	c3.mu.Unlock()

	// Oversized body.
	huge := strings.Repeat("a", maxModelsCatalogBytes+10)
	ts4 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(huge))
	}))
	defer ts4.Close()
	c4 := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: 2 * time.Second, Transport: rewriteTransport{from: modelsDevURL, to: ts4.URL + "/api.json"}},
	}
	c4.refresh()
	c4.mu.Lock()
	if c4.lastError == "" || !strings.Contains(c4.lastError, "exceeds") {
		t.Errorf("expected exceeds error, got %q", c4.lastError)
	}
	c4.mu.Unlock()

	// Bad JSON body.
	ts5 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts5.Close()
	c5 := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: 2 * time.Second, Transport: rewriteTransport{from: modelsDevURL, to: ts5.URL + "/api.json"}},
	}
	c5.refresh()
	c5.mu.Lock()
	if c5.lastError == "" {
		t.Error("expected JSON parse error")
	}
	c5.mu.Unlock()
}

// rewriteTransport rewrites requests from one URL prefix to another, used
// only in tests so we don't need to hit models.dev directly.
type rewriteTransport struct {
	from, to string
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == r.from {
		newReq := req.Clone(req.Context())
		newURL := r.to
		// Parse "to" and replace scheme/host.
		parsed, err := http.NewRequest(req.Method, newURL, nil)
		if err != nil {
			return nil, err
		}
		newURL2 := parsed.URL
		newReq.URL = newURL2
		newReq.Host = newURL2.Host
		return http.DefaultTransport.RoundTrip(newReq)
	}
	return http.DefaultTransport.RoundTrip(req)
}

type failingTransport struct{}

func (failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, errNetworkDown
}

var errNetworkDown = errors.New("synthetic network failure")

// bodyErroringTransport returns a 200 response with a body whose Read
// always returns an error. This exercises the io.ReadAll error branch
// in modelsCache.refresh.
type bodyErroringTransport struct{}

func (bodyErroringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       &errReaderBody{},
		Header:     make(http.Header),
	}, nil
}

type errReaderBody struct{}

func (errReaderBody) Read(p []byte) (int, error) {
	return 0, errors.New("body read failure")
}
func (errReaderBody) Close() error { return nil }

// TestModelsCacheRefreshBodyReadError covers the io.ReadAll error branch
// (line 953 in ai_models.go).
func TestModelsCacheRefreshBodyReadError(t *testing.T) {
	c := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: time.Second, Transport: bodyErroringTransport{}},
	}
	c.refresh()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastError == "" || !strings.Contains(c.lastError, "body read failure") {
		t.Errorf("expected body read error, got %q", c.lastError)
	}
}

func TestModelsCacheSetError(t *testing.T) {
	c := &modelsCache{}
	c.setError(fmt.Errorf("simulated"))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastError != "simulated" {
		t.Errorf("lastError=%q", c.lastError)
	}
}

func TestHandleAIModelsNoData(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	// Save/restore shared cache so we don't pollute other tests.
	saved := sharedModelsCache
	defer func() { sharedModelsCache = saved }()
	c := &modelsCache{ttl: time.Minute, client: &http.Client{Timeout: time.Second}}
	sharedModelsCache = c

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/models", nil)
	rr := httptest.NewRecorder()
	s.handleAIModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	// Empty data + no error → Retry-After should be set.
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on empty cache")
	}
}

func TestHandleAIModelsWithData(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	saved := sharedModelsCache
	defer func() { sharedModelsCache = saved }()
	c := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: time.Second},
	}
	c.mu.Lock()
	c.data = []modelInfo{
		{ID: "m1", ProviderID: "openai", Provider: "OpenAI", Format: "openai"},
		{ID: "m2", ProviderID: "anthropic", Provider: "Anthropic", Format: "anthropic"},
	}
	c.loadedAt = time.Now()
	c.mu.Unlock()
	sharedModelsCache = c

	// Unfiltered
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/models", nil)
	rr := httptest.NewRecorder()
	s.handleAIModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "OpenAI") {
		t.Errorf("body missing OpenAI: %s", rr.Body.String())
	}

	// Filter by provider
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/models?provider=anthropic", nil)
	rr = httptest.NewRecorder()
	s.handleAIModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), `"id":"m1"`) {
		t.Errorf("openai leaked into anthropic filter: %s", rr.Body.String())
	}

	// Filter by format
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/models?provider=openai", nil)
	rr = httptest.NewRecorder()
	s.handleAIModels(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}

	// lastError propagated.
	c.mu.Lock()
	c.lastError = "upstream flaked"
	c.mu.Unlock()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ai/models", nil)
	rr = httptest.NewRecorder()
	s.handleAIModels(rr, req)
	if !strings.Contains(rr.Body.String(), "upstream flaked") {
		t.Errorf("body missing error: %s", rr.Body.String())
	}
}

func TestEffectiveAIEndpointBranches(t *testing.T) {
	// nil cfg
	s := &Server{}
	if got := s.effectiveAIEndpoint(); got != "" {
		t.Errorf("nil cfg endpoint=%q want empty", got)
	}

	// anthropic with empty endpoint
	cfg := config.DefaultConfig()
	cfg.AI.Provider = "anthropic"
	cfg.AI.Endpoint = ""
	s.cfg = cfg
	if got := s.effectiveAIEndpoint(); got != "https://api.anthropic.com/v1/messages" {
		t.Errorf("default anthropic=%q", got)
	}

	// anthropic with explicit endpoint
	cfg.AI.Endpoint = "https://api.z.ai/api/anthropic"
	if got := s.effectiveAIEndpoint(); got != "https://api.z.ai/api/anthropic/v1/messages" {
		t.Errorf("explicit anthropic=%q", got)
	}

	// openai with empty endpoint
	cfg.AI.Provider = "openai"
	cfg.AI.Endpoint = ""
	if got := s.effectiveAIEndpoint(); got != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("default openai=%q", got)
	}

	// openai with chat/completions
	cfg.AI.Endpoint = "https://api.openai.com/v1/chat/completions"
	if got := s.effectiveAIEndpoint(); got != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("openai with completions=%q", got)
	}

	// openai with other
	cfg.AI.Endpoint = "https://api.deepseek.com"
	if got := s.effectiveAIEndpoint(); got != "https://api.deepseek.com/v1/chat/completions" {
		t.Errorf("openai other=%q", got)
	}
}

func TestHandleAIPresets(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/presets", nil)
	rr := httptest.NewRecorder()
	s.handleAIPresets(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var presets []aiPreset
	if err := json.Unmarshal(rr.Body.Bytes(), &presets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(presets) == 0 {
		t.Fatal("expected at least one preset")
	}
	// Check a known entry.
	found := false
	for _, p := range presets {
		if p.ID == "anthropic" && p.Provider == "anthropic" {
			found = true
			break
		}
	}
	if !found {
		t.Error("anthropic preset missing")
	}
}

func TestIsSensitiveHeaderKeyAllBranches(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{" Proxy-Authorization ", true},
		{"X-Api-Key", true},
		{"API_KEY", true},
		{"X-Token", true},
		{"X-Secret", true},
		{"MyKey", true},
		{"Content-Type", false},
		{"X-Title", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isSensitiveHeaderKey(c.in); got != c.want {
			t.Errorf("isSensitiveHeaderKey(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

// TestModelsCacheRefreshInvalidURL covers the http.NewRequest error branch
// at the top of refresh() — exercised by pointing modelsDevURL at a URL
// that the http client refuses to parse.
func TestModelsCacheRefreshInvalidURL(t *testing.T) {
	saved := modelsDevURL
	defer func() { modelsDevURL = saved }()

	// "://no-scheme" makes http.NewRequest fail before the transport is touched.
	modelsDevURL = "://no-scheme"

	c := &modelsCache{
		ttl:    time.Minute,
		client: &http.Client{Timeout: time.Second},
	}
	c.refresh()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastError == "" {
		t.Fatal("expected error from invalid URL")
	}
}
