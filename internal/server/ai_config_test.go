package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
)

// newTestServer builds a Server with just enough scaffolding to exercise the
// AI config handlers — the SSE hub still needs a live Emitter to avoid a nil
// deref inside NewSSEHub, but we don't actually publish anything in these tests.
func newTestServer(t *testing.T, cfgPath string, cfg *config.Config) (*Server, *[]*config.Config) {
	t.Helper()
	var applied []*config.Config
	s, err := New(Options{
		Cfg:     cfg,
		CfgPath: cfgPath,
		Emitter: event.NewEmitter(),
		Version: "test-version",
		OnCfgApply: func(c *config.Config) {
			applied = append(applied, c)
		},
	})
	if err != nil {
		t.Fatalf("newTestServer: %v", err)
	}
	return s, &applied
}

// TestAIConfigSaveRoundTrip exercises the full POST → disk write → GET flow.
// This is the regression net for the "save etmiyor gibi" report: a success
// here means the handler reliably persists to disk, not just to memory.
func TestAIConfigSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")

	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	s, applied := newTestServer(t, cfgPath, cfg)

	body := aiConfigDTO{
		Enabled:              true,
		Provider:             "anthropic",
		APIKey:               "sk-test-round-trip-12345",
		Model:                "glm-5.1",
		Endpoint:             "https://api.z.ai/api/anthropic",
		Language:             "tr",
		MaxTokens:            2048,
		MaxRequestsPerMinute: 10,
		IncludeProcessTree:   true,
		IncludePortMap:       true,
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/ai/config", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)

	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 1) response shape — key is masked, values reflect new state.
	var resp struct {
		OK     bool        `json:"ok"`
		Config aiConfigDTO `json:"config"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Fatalf("ok=false")
	}
	if resp.Config.Model != "glm-5.1" {
		t.Errorf("model=%q want glm-5.1", resp.Config.Model)
	}
	if !strings.HasPrefix(resp.Config.APIKey, "****") || !strings.HasSuffix(resp.Config.APIKey, "2345") {
		t.Errorf("api_key=%q want ****…2345", resp.Config.APIKey)
	}

	// 2) onCfgApply hook fired.
	if len(*applied) == 0 {
		t.Fatal("OnCfgApply not invoked")
	}
	last := (*applied)[len(*applied)-1]
	if last.AI.Model != "glm-5.1" || last.AI.APIKey != "sk-test-round-trip-12345" {
		t.Errorf("applied config missing fields: %+v", last.AI)
	}

	// 3) disk file was actually written with the plaintext key.
	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.AI.Model != "glm-5.1" {
		t.Errorf("disk model=%q want glm-5.1", reloaded.AI.Model)
	}
	if reloaded.AI.APIKey != "sk-test-round-trip-12345" {
		t.Errorf("disk api_key=%q want plaintext", reloaded.AI.APIKey)
	}
	if reloaded.AI.Endpoint != "https://api.z.ai/api/anthropic" {
		t.Errorf("disk endpoint=%q", reloaded.AI.Endpoint)
	}
	if !reloaded.AI.Enabled {
		t.Errorf("disk enabled=false want true")
	}

	// 4) GET reflects the new state (and still masks the key).
	getReq := httptest.NewRequest("GET", "/api/v1/ai/config", nil)
	getRR := httptest.NewRecorder()
	s.handleAIConfigGet(getRR, getReq)
	var got aiConfigDTO
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Model != "glm-5.1" {
		t.Errorf("get model=%q", got.Model)
	}
	if !strings.HasPrefix(got.APIKey, "****") {
		t.Errorf("get api_key not masked: %q", got.APIKey)
	}
}

// TestAIConfigEmptyKeyPreserved verifies that POST with empty api_key keeps the
// existing stored key. This is the "leave blank to keep current" contract.
func TestAIConfigEmptyKeyPreserved(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")

	cfg := config.DefaultConfig()
	cfg.AI.APIKey = "sk-existing-secret"
	cfg.AI.Model = "glm-5.1"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	s, _ := newTestServer(t, cfgPath, cfg)

	body := aiConfigDTO{
		Enabled:              true,
		Provider:             "anthropic",
		APIKey:               "", // blank → keep current
		Model:                "glm-4.6",
		MaxTokens:            1024,
		MaxRequestsPerMinute: 5,
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/ai/config", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	reloaded, _ := config.Load(cfgPath)
	if reloaded.AI.APIKey != "sk-existing-secret" {
		t.Errorf("api_key=%q want sk-existing-secret (preserved)", reloaded.AI.APIKey)
	}
	if reloaded.AI.Model != "glm-4.6" {
		t.Errorf("model=%q want glm-4.6", reloaded.AI.Model)
	}
}

// TestAIConfigBadProvider rejects unknown provider values without touching disk.
func TestAIConfigBadProvider(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	seeded, _ := os.ReadFile(cfgPath)

	s, _ := newTestServer(t, cfgPath, cfg)
	body := `{"provider":"bogus","model":"x"}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)

	if rr.Code != 400 {
		t.Errorf("status=%d want 400", rr.Code)
	}
	after, _ := os.ReadFile(cfgPath)
	if !bytes.Equal(seeded, after) {
		t.Errorf("disk file was mutated on bad-provider POST")
	}
}

// TestAIConfigNoCfgPath returns 503 when the server wasn't wired with a path.
func TestAIConfigNoCfgPath(t *testing.T) {
	s, _ := newTestServer(t, "", config.DefaultConfig()) // no CfgPath
	body := `{"provider":"anthropic","model":"x"}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != 503 {
		t.Errorf("status=%d want 503", rr.Code)
	}
}

// TestAIConfigInvalidJSON covers the readJSON failure branch in
// handleAIConfigUpdate.
func TestAIConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t, cfgPath, cfg)
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

// TestAIConfigEmptyProviderDefaultsToAnthropic covers the
// `if provider == ""` branch.
func TestAIConfigEmptyProviderDefaultsToAnthropic(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t, cfgPath, cfg)
	body := `{"provider":"","model":"x"}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAIConfigWithExtraHeaders covers the `if body.ExtraHeaders != nil`
// branch.
func TestAIConfigWithExtraHeaders(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	cfg := config.DefaultConfig()
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t, cfgPath, cfg)
	body := `{"provider":"anthropic","model":"x","extra_headers":{"X-Test":"v"}}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	reloaded, _ := config.Load(cfgPath)
	if reloaded.AI.ExtraHeaders["X-Test"] != "v" {
		t.Errorf("ExtraHeaders=%v", reloaded.AI.ExtraHeaders)
	}
}

// TestAIConfigValidationFailure covers the `next.Validate()` failure branch.
// We pre-seed the cfg with an invalid Server.Port so validate rejects.
func TestAIConfigValidationFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	cfg := config.DefaultConfig()
	cfg.Server.Port = 70000 // out of range — Validate will reject
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t, cfgPath, cfg)
	body := `{"provider":"anthropic","model":"x"}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// TestAIConfigSaveFailure covers the config.Save failure branch by pointing
// cfgPath at a non-existent Windows drive.
func TestAIConfigSaveFailure(t *testing.T) {
	cfgPath := `Z:\nonexistent\wtm.yaml`
	cfg := config.DefaultConfig()
	s, _ := newTestServer(t, cfgPath, cfg)
	body := `{"provider":"anthropic","model":"x"}`
	req := httptest.NewRequest("POST", "/api/v1/ai/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleAIConfigUpdate(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "****"},
		{"abcd", "****"},
		{"sk-abcdefgh1234", "****1234"},
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestAIConfigGetRedactsSensitiveHeaders(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.ExtraHeaders = map[string]string{
		"Authorization": "Bearer super-secret-token",
		"X-Title":       "WTM",
	}
	s, _ := newTestServer(t, "", cfg)

	req := httptest.NewRequest("GET", "/api/v1/ai/config", nil)
	rr := httptest.NewRecorder()
	s.handleAIConfigGet(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got aiConfigDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(got.ExtraHeaders["Authorization"], "****") {
		t.Fatalf("authorization header not redacted: %q", got.ExtraHeaders["Authorization"])
	}
	if got.ExtraHeaders["X-Title"] != "WTM" {
		t.Fatalf("non-sensitive header mutated: %q", got.ExtraHeaders["X-Title"])
	}
}
