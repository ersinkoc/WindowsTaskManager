package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestAISuggestionStoreRememberAndConsume(t *testing.T) {
	store := newAISuggestionStore(time.Minute)
	store.remember([]ai.Suggestion{{ID: "x", Type: "kill", PID: 1}})
	store.remember(nil)                                             // nil/empty → no-op
	store.remember([]ai.Suggestion{{ID: "", Type: "kill", PID: 1}}) // blank id → skipped

	if err := store.consume(ai.Suggestion{ID: "x", Type: "kill", PID: 1}); err != nil {
		t.Fatalf("consume: %v", err)
	}
	// Already consumed → not found.
	err := store.consume(ai.Suggestion{ID: "x", Type: "kill", PID: 1})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err=%v", err)
	}
}

func TestAISuggestionStoreEmptyID(t *testing.T) {
	store := newAISuggestionStore(time.Minute)
	err := store.consume(ai.Suggestion{ID: ""})
	if err == nil || !strings.Contains(err.Error(), "id required") {
		t.Errorf("err=%v", err)
	}
}

func TestAISuggestionStorePayloadMismatch(t *testing.T) {
	store := newAISuggestionStore(time.Minute)
	store.remember([]ai.Suggestion{{ID: "x", Type: "kill", PID: 1}})
	err := store.consume(ai.Suggestion{ID: "x", Type: "kill", PID: 2})
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("err=%v", err)
	}
}

func TestAISuggestionStoreNilSafe(t *testing.T) {
	var store *aiSuggestionStore
	store.remember([]ai.Suggestion{{ID: "x"}})
	if err := store.consume(ai.Suggestion{ID: "x"}); err != nil {
		t.Errorf("consume on nil: %v", err)
	}
}

func TestAISuggestionStorePrune(t *testing.T) {
	store := newAISuggestionStore(time.Minute)
	store.remember([]ai.Suggestion{{ID: "x", Type: "kill"}})
	// Force expiry by manipulating the map directly.
	store.mu.Lock()
	store.items["x"] = issuedAISuggestion{
		suggestion: ai.Suggestion{ID: "x", Type: "kill"},
		expiresAt:  time.Now().Add(-time.Second),
	}
	store.mu.Unlock()
	if err := store.consume(ai.Suggestion{ID: "x", Type: "kill"}); err == nil {
		t.Error("expected expiry to evict")
	}
}

func TestCloneAISuggestion(t *testing.T) {
	in := ai.Suggestion{
		ID:     "x",
		Type:   "add_rule",
		Rule:   &ai.RuleSuggestion{Name: "r"},
		Policy: &ai.AutoPolicy{Status: "ok"},
	}
	clone := cloneAISuggestion(in)
	if clone.Rule == in.Rule {
		t.Error("Rule should be cloned")
	}
	if clone.Policy == in.Policy {
		t.Error("Policy should be cloned")
	}

	// No rule / policy → same nil pointers.
	plain := ai.Suggestion{ID: "y"}
	if cloneAISuggestion(plain).Rule != nil {
		t.Error("expected nil rule")
	}
}

func TestSameAISuggestionBranches(t *testing.T) {
	a := ai.Suggestion{ID: "x", Type: "kill", PID: 1, Name: "alpha.exe"}
	// Equal
	b := a
	if !sameAISuggestion(a, b) {
		t.Error("equal suggestions should match")
	}
	// ID differs
	c := a
	c.ID = "y"
	if sameAISuggestion(a, c) {
		t.Error("different IDs should not match")
	}
	// Type differs (case-insensitive)
	d := a
	d.Type = "KILL"
	if !sameAISuggestion(a, d) {
		t.Error("type match should be case-insensitive")
	}
	// Type differs (different value)
	d2 := a
	d2.Type = "suspend"
	if sameAISuggestion(a, d2) {
		t.Error("different types should not match")
	}
	// PID differs
	e := a
	e.PID = 2
	if sameAISuggestion(a, e) {
		t.Error("different PIDs should not match")
	}
	// Name differs
	f := a
	f.Name = "beta"
	if sameAISuggestion(a, f) {
		t.Error("different names should not match")
	}
	// Rule set vs unset
	g := a
	g.Rule = &ai.RuleSuggestion{Name: "r"}
	if sameAISuggestion(a, g) {
		t.Error("rule set vs unset should not match")
	}
}

func TestSameRuleSuggestionAllBranches(t *testing.T) {
	rule := &ai.RuleSuggestion{
		Name: "r", Enabled: true, Match: "x", Metric: "cpu_percent",
		Op: ">=", Threshold: 90, For: 30, ForSeconds: 30,
		Action: "kill", Cooldown: 60, CooldownSeconds: 60,
	}
	if !sameRuleSuggestion(nil, nil) {
		t.Error("nil == nil")
	}
	if sameRuleSuggestion(nil, rule) || sameRuleSuggestion(rule, nil) {
		t.Error("nil vs non-nil")
	}
	if !sameRuleSuggestion(rule, cloneRule(rule)) {
		t.Error("equal rules should match")
	}
	diff := cloneRule(rule)
	diff.Name = "other"
	if sameRuleSuggestion(rule, diff) {
		t.Error("different name should not match")
	}
	diff = cloneRule(rule)
	diff.Threshold = 0
	if sameRuleSuggestion(rule, diff) {
		t.Error("different threshold should not match")
	}
	diff = cloneRule(rule)
	diff.ForSeconds = 99
	if sameRuleSuggestion(rule, diff) {
		t.Error("different for_seconds should not match")
	}
	diff = cloneRule(rule)
	diff.CooldownSeconds = 99
	if sameRuleSuggestion(rule, diff) {
		t.Error("different cooldown_seconds should not match")
	}
}

func cloneRule(r *ai.RuleSuggestion) *ai.RuleSuggestion {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

func TestAiRuleToConfigAllBranches(t *testing.T) {
	cases := []struct {
		name   string
		rule   *ai.RuleSuggestion
		wantOk bool
		want   string
	}{
		{"nil rule", nil, false, "rule required"},
		{"blank name", &ai.RuleSuggestion{Match: "x", Metric: "cpu_percent"}, false, "name required"},
		{"blank match", &ai.RuleSuggestion{Name: "r", Metric: "cpu_percent"}, false, "match pattern required"},
		{"bad metric", &ai.RuleSuggestion{Name: "r", Match: "x", Metric: "bogus"}, false, "metric must be"},
		{"bad op", &ai.RuleSuggestion{Name: "r", Match: "x", Metric: "cpu_percent", Op: "=="}, false, "op must be"},
		{"bad action", &ai.RuleSuggestion{Name: "r", Match: "x", Metric: "cpu_percent", Action: "reboot"}, false, "action must be"},
		{"for too large", &ai.RuleSuggestion{Name: "r", Match: "x", Metric: "cpu_percent", ForSeconds: 99999}, false, "for_seconds must be"},
		{"valid", &ai.RuleSuggestion{
			Name: "r", Enabled: true, Match: "x", Metric: "cpu_percent",
			Op: ">=", Threshold: 90, ForSeconds: 30, Action: "kill", CooldownSeconds: 60,
		}, true, ""},
		{"defaults", &ai.RuleSuggestion{
			Name: "r", Match: "x", Metric: "memory_bytes", Action: "alert", Cooldown: 30,
		}, true, ""},
		{"for from For", &ai.RuleSuggestion{
			Name: "r", Match: "x", Metric: "cpu_percent", For: 10, Action: "alert",
		}, true, ""},
		{"negative cooldown clamped", &ai.RuleSuggestion{
			Name: "r", Match: "x", Metric: "cpu_percent", Cooldown: -10,
		}, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := aiRuleToConfig(c.rule)
			if c.wantOk {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Name == "" {
					t.Errorf("rule has empty name: %+v", got)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), c.want) {
					t.Errorf("err=%v want substring %q", err, c.want)
				}
			}
		})
	}
}

// buildServerForExec builds a server whose controller uses a fresh store
// populated with one process so ExecuteAISuggestion's kill/suspend paths
// can find a real PID.
func buildServerForExec(t *testing.T, cfgPath string, store *storage.Store) *Server {
	t.Helper()
	return buildServerForExecWithCfg(t, cfgPath, store, defaultCfg())
}

// buildServerForExecWithCfg is the same but lets the caller pass a
// pre-modified config (e.g. with a custom protected list).
func buildServerForExecWithCfg(t *testing.T, cfgPath string, store *storage.Store, cfg *config.Config) *Server {
	t.Helper()
	if store == nil {
		store = storage.NewStore(60, 10)
	}
	em := event.NewEmitter()
	s, err := New(Options{
		Cfg:        cfg,
		CfgPath:    cfgPath,
		Store:      store,
		Controller: controller.NewController(cfg, store, em),
		Alerts:     anomaly.NewAlertStore(64),
		Emitter:    em,
		Version:    "v",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func newStoreWithProcess(pid uint32) *storage.Store {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: pid, Name: "ping.exe"}},
	})
	return store
}

func TestExecuteAISuggestionAllBranches(t *testing.T) {
	pid, cleanup := pingSpawn(t, 5)
	defer cleanup()

	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	store := newStoreWithProcess(pid)
	s := buildServerForExec(t, cfgPath, store)

	// Unknown type
	err := s.ExecuteAISuggestion(ai.Suggestion{Type: "bogus"}, true)
	if err == nil || !strings.Contains(err.Error(), "type must be") {
		t.Errorf("unknown type err=%v", err)
	}

	// Kill with no pid → input error
	err = s.ExecuteAISuggestion(ai.Suggestion{Type: "kill"}, true)
	if err == nil || !strings.Contains(err.Error(), "pid required") {
		t.Errorf("kill no pid err=%v", err)
	}

	// Kill with valid pid → success
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "kill", PID: pid}, true); err != nil {
		t.Errorf("kill err=%v", err)
	}

	// Restart a child for suspend.
	pid2, cleanup2 := pingSpawn(t, 5)
	defer cleanup2()
	s = buildServerForExec(t, cfgPath, newStoreWithProcess(pid2))

	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "suspend"}, true); err == nil {
		t.Error("suspend no pid should fail")
	}
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "suspend", PID: pid2}, true); err != nil {
		t.Errorf("suspend err=%v", err)
	}

	// Protect without name
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "protect"}, true); err == nil {
		t.Error("protect no name should fail")
	}

	// Protect with name → success
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "protect", Name: "shield.exe"}, true); err != nil {
		t.Errorf("protect err=%v", err)
	}

	// Ignore without name
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "ignore"}, true); err == nil {
		t.Error("ignore no name should fail")
	}

	// Ignore with name → success
	if err := s.ExecuteAISuggestion(ai.Suggestion{Type: "ignore", Name: "noisy.exe"}, true); err != nil {
		t.Errorf("ignore err=%v", err)
	}

	// add_rule with nil rule
	err = s.ExecuteAISuggestion(ai.Suggestion{Type: "add_rule"}, true)
	if err == nil || !strings.Contains(err.Error(), "rule required") {
		t.Errorf("add_rule nil err=%v", err)
	}

	// add_rule with invalid rule (metric bad)
	err = s.ExecuteAISuggestion(ai.Suggestion{
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{Name: "x", Match: "y", Metric: "bogus"},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "metric must be") {
		t.Errorf("add_rule invalid err=%v", err)
	}

	// add_rule success
	err = s.ExecuteAISuggestion(ai.Suggestion{
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{
			Name: "alpha-mem-cap", Match: "alpha.exe", Metric: "memory_bytes",
			Op: ">=", Threshold: 4000000000, ForSeconds: 30, Action: "kill",
		},
	}, true)
	if err != nil {
		t.Errorf("add_rule err=%v", err)
	}

	// add_rule duplicate
	err = s.ExecuteAISuggestion(ai.Suggestion{
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{
			Name: "alpha-mem-cap", Match: "alpha.exe", Metric: "memory_bytes",
			Op: ">=", Threshold: 4000000000, ForSeconds: 30, Action: "kill",
		},
	}, true)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("add_rule dup err=%v", err)
	}
}

func TestExecuteAISuggestionNoCfgPath(t *testing.T) {
	cfg := defaultCfg()
	s, _ := fullTestServer(t, "", cfg, nil)
	err := s.ExecuteAISuggestion(ai.Suggestion{Type: "protect", Name: "x"}, true)
	if err == nil {
		t.Error("expected error for no cfgPath")
	}
}

func TestHandleConfigProtectToggleSaveFails(t *testing.T) {
	cfgPath := `Z:\nonexistent\wtm.yaml`
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","protect":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleConfigIgnoreToggleSaveFails(t *testing.T) {
	cfgPath := `Z:\nonexistent\wtm.yaml`
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","ignore":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigIgnoreToggle(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMutateConfigSaveFails(t *testing.T) {
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := dir + "/seed.yaml"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	cfgPath2 := `Z:\nonexistent\wtm.yaml`
	s, _ := fullTestServer(t, cfgPath2, cfg, nil)
	err := s.mutateConfig(func(c *config.Config) error { return nil })
	if err == nil {
		t.Error("expected error for save failure")
	}
}

func TestHandleConfigProtectToggleAllBranches(t *testing.T) {
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)

	// Bad JSON
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d", rr.Code)
	}

	// No cfgPath → 500
	s2, _ := fullTestServer(t, "", cfg, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","protect":true}`))
	req.Header.Set("Content-Type", "application/json")
	s2.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("no cfgPath status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Empty name → 400
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"","protect":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty name status=%d", rr.Code)
	}

	// Add
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alpha.exe","protect":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", rr.Code, rr.Body.String())
	}
	reloaded, _ := config.Load(cfgPath)
	found := false
	for _, n := range reloaded.Controller.ProtectedProcesses {
		if strings.EqualFold(n, "alpha.exe") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected alpha.exe in protected; got %v", reloaded.Controller.ProtectedProcesses)
	}

	// Remove
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"alpha.exe","protect":false}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigProtectToggle(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove status=%d", rr.Code)
	}
	reloaded, _ = config.Load(cfgPath)
	for _, n := range reloaded.Controller.ProtectedProcesses {
		if strings.EqualFold(n, "alpha.exe") {
			t.Errorf("alpha.exe should have been removed: %v", reloaded.Controller.ProtectedProcesses)
		}
	}
}

func TestHandleConfigIgnoreToggleAllBranches(t *testing.T) {
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)

	// Bad JSON
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigIgnoreToggle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d", rr.Code)
	}

	// Empty name
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"","ignore":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigIgnoreToggle(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty name status=%d", rr.Code)
	}

	// Add
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"noisy.exe","ignore":true}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigIgnoreToggle(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", rr.Code, rr.Body.String())
	}
	reloaded, _ := config.Load(cfgPath)
	found := false
	for _, n := range reloaded.Anomaly.IgnoreProcesses {
		if strings.EqualFold(n, "noisy.exe") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected noisy.exe; got %v", reloaded.Anomaly.IgnoreProcesses)
	}

	// Remove
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"noisy.exe","ignore":false}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigIgnoreToggle(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove status=%d", rr.Code)
	}
	reloaded, _ = config.Load(cfgPath)
	for _, n := range reloaded.Anomaly.IgnoreProcesses {
		if strings.EqualFold(n, "noisy.exe") {
			t.Errorf("noisy.exe should have been removed: %v", reloaded.Anomaly.IgnoreProcesses)
		}
	}
}

func TestMutateConfigNoCfgPath(t *testing.T) {
	cfg := defaultCfg()
	s, _ := fullTestServer(t, "", cfg, nil)
	err := s.mutateConfig(func(c *config.Config) error { return nil })
	if err == nil {
		t.Error("expected error for no cfgPath")
	}
}

func TestMutateConfigMutationError(t *testing.T) {
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)
	want := errors.New("boom")
	err := s.mutateConfig(func(c *config.Config) error { return want })
	if err != want {
		t.Errorf("err=%v want %v", err, want)
	}
}

func TestMutateConfigValidationError(t *testing.T) {
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)
	err := s.mutateConfig(func(c *config.Config) error {
		c.Monitoring.Interval = time.Millisecond // invalid (< 100ms)
		return nil
	})
	if err == nil {
		t.Error("expected validation error")
	}
}

func TestAppendUniqueFoldAndRemoveFold(t *testing.T) {
	if got := appendUniqueFold([]string{"a"}, "b"); len(got) != 2 {
		t.Errorf("add new: %v", got)
	}
	if got := appendUniqueFold([]string{"alpha"}, "ALPHA"); len(got) != 1 {
		t.Errorf("add duplicate: %v", got)
	}
	if got := appendUniqueFold([]string{"a"}, ""); len(got) != 1 {
		t.Errorf("empty add: %v", got)
	}
	if got := appendUniqueFold(nil, "x"); len(got) != 1 {
		t.Errorf("nil add: %v", got)
	}

	out := removeFold([]string{"alpha", "beta"}, "ALPHA")
	if len(out) != 1 || out[0] != "beta" {
		t.Errorf("remove fold: %v", out)
	}
	out = removeFold([]string{}, "x")
	if len(out) != 0 {
		t.Errorf("remove from empty: %v", out)
	}
}

func TestRememberAISuggestionsNilSafe(t *testing.T) {
	var s *Server
	s.rememberAISuggestions([]ai.Suggestion{{ID: "x"}}) // must not panic
	if err := s.consumeAISuggestion(ai.Suggestion{ID: "x"}); err != nil {
		t.Errorf("err=%v", err)
	}
}

func TestHandleAIExecuteSaveFailedBranch(t *testing.T) {
	// Trigger the `default:` branch in handleAIExecute's switch by
	// returning a non-inputErr, non-controller error from the mutate
	// callback inside ExecuteAISuggestion's add_rule handler.
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)
	// Seed an existing rule.
	s.cfg.Rules = []config.Rule{{Name: "dup", Match: "x", Metric: "cpu_percent", Op: ">=", Threshold: 1, For: 1 * time.Second, Action: "alert", Cooldown: 1 * time.Second}}
	if err := config.Save(cfgPath, s.cfg); err != nil {
		t.Fatal(err)
	}
	suggestion := ai.Suggestion{
		ID:   "dup-rule",
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{
			Name: "dup", Match: "x", Metric: "cpu_percent",
			Op: ">=", Threshold: 1, ForSeconds: 1, Action: "alert",
		},
	}
	s.rememberAISuggestions([]ai.Suggestion{suggestion})
	body, _ := json.Marshal(suggestion)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "save_failed") && !strings.Contains(rr.Body.String(), "already exists") {
		t.Errorf("expected save_failed/already exists, got %s", rr.Body.String())
	}
}

func TestHandleAIExecuteAllPaths(t *testing.T) {
	// Bad JSON
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// Unknown suggestion → bad request
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"type":"kill"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown suggestion status=%d", rr.Code)
	}

	// Issue + execute add_rule
	cfg := defaultCfg()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ = fullTestServer(t, cfgPath, cfg, nil)
	suggestion := ai.Suggestion{
		ID:   "rule-1",
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{
			Name: "rule-a", Match: "x", Metric: "cpu_percent",
			Op: ">=", Threshold: 50, ForSeconds: 10, Action: "kill",
		},
	}
	s.rememberAISuggestions([]ai.Suggestion{suggestion})

	body, _ := json.Marshal(suggestion)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("add_rule status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Replay should fail.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("replay status=%d", rr.Code)
	}
}

func TestHandleAIExecuteControllerErrors(t *testing.T) {
	// Issue a kill suggestion and force controllerErr = ErrProtected.
	cfg := defaultCfg()
	cfg.Controller.ProtectedProcesses = []string{"protected.exe"}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	pid, cleanup := pingSpawn(t, 5)
	defer cleanup()
	store := newStoreWithProcess(pid)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: pid, Name: "protected.exe"}},
	})
	s := buildServerForExecWithCfg(t, cfgPath, store, cfg)

	suggestion := ai.Suggestion{ID: "kill-1", Type: "kill", PID: pid}
	s.rememberAISuggestions([]ai.Suggestion{suggestion})
	body, _ := json.Marshal(suggestion)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (protected), got %d body=%s", rr.Code, rr.Body.String())
	}

	// Save_failed path: issue a rule with bad metric → bad_request 400
	suggestion2 := ai.Suggestion{
		ID:   "rule-bad",
		Type: "add_rule",
		Rule: &ai.RuleSuggestion{Name: "rule-bad", Match: "x", Metric: "bogus"},
	}
	s.rememberAISuggestions([]ai.Suggestion{suggestion2})
	body2, _ := json.Marshal(suggestion2)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body2)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleAIExecuteConfirmNeeded(t *testing.T) {
	cfg := defaultCfg()
	cfg.Controller.ConfirmKillSystem = true
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wtm.yaml")
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	pid, cleanup := pingSpawn(t, 5)
	defer cleanup()
	store := newStoreWithProcess(pid)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{
			PID: pid, Name: "ping.exe",
			ExePath: `C:\Windows\System32\ping.exe`,
		}},
	})
	s := buildServerForExecWithCfg(t, cfgPath, store, cfg)

	suggestion := ai.Suggestion{ID: "kill-system", Type: "kill", PID: pid}
	s.rememberAISuggestions([]ai.Suggestion{suggestion})
	body, _ := json.Marshal(suggestion)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIExecute(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 confirm, got %d body=%s", rr.Code, rr.Body.String())
	}
}
