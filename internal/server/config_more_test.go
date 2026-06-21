package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
)

func TestHandleConfigUpdateNoCfgPath(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigUpdate(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestApplyConfigUpdateNilSafe(t *testing.T) {
	applyConfigUpdate(nil, nil)
	applyConfigUpdate(nil, &configUpdateDTO{})
	applyConfigUpdate(&config.Config{}, nil)
}

func TestApplyConfigUpdateAllBranches(t *testing.T) {
	cfg := *config.DefaultConfig()
	body := &configUpdateDTO{
		Server: &configServerUpdateDTO{OpenBrowser: ptrB(false)},
		Monitoring: &configMonitoringUpdateDTO{
			IntervalMS:            ptrI(500),
			ProcessTreeIntervalMS: ptrI(2000),
			PortScanIntervalMS:    ptrI(3000),
			GPUIntervalMS:         ptrI(1500),
			HistoryDurationSec:    ptrI(600),
			MaxProcesses:          ptrI(512),
		},
		Controller: &configControllerUpdateDTO{ConfirmKillSystem: ptrB(false)},
		Notifications: &configNotificationsUpdateDTO{
			TrayBalloon:         ptrB(true),
			BalloonRateLimitSec: ptrI(60),
			BalloonMinSeverity:  ptrS("warning"),
		},
		UI: &configUIUpdateDTO{
			Theme:                ptrS("dark"),
			DefaultSort:          ptrS("memory"),
			DefaultSortOrder:     ptrS("asc"),
			SparklinePoints:      ptrI(80),
			ProcessTablePageSize: ptrI(50),
			RefreshRateMS:        ptrI(750),
		},
	}
	applyConfigUpdate(&cfg, body)
	if cfg.Server.OpenBrowser {
		t.Error("server.open_browser not updated")
	}
	if cfg.Monitoring.Interval.Milliseconds() != 500 {
		t.Errorf("interval=%v", cfg.Monitoring.Interval)
	}
	if cfg.Monitoring.HistoryDuration.Seconds() != 600 {
		t.Errorf("history=%v", cfg.Monitoring.HistoryDuration)
	}
	if cfg.UI.Theme != "dark" {
		t.Errorf("theme=%q", cfg.UI.Theme)
	}
}

func ptrB(b bool) *bool     { return &b }
func ptrI(i int) *int       { return &i }
func ptrS(s string) *string { return &s }

// ===== cloneConfig =====

func TestCloneConfigNilSrc(t *testing.T) {
	got := cloneConfig(nil)
	if got.Server.Port == 0 {
		t.Errorf("expected default config port, got %d", got.Server.Port)
	}
}

func TestCloneConfigDeepCopy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = []string{"a.exe", "b.exe"}
	cfg.Anomaly.IgnoreProcesses = []string{"c.exe"}
	cfg.Rules = []config.Rule{{Name: "rule"}}
	cfg.AI.ExtraHeaders = map[string]string{"X": "Y"}
	cfg.WellKnownPorts = map[uint16]string{80: "HTTP"}

	clone := cloneConfig(cfg)
	// Mutate the clone.
	clone.Controller.ProtectedProcesses[0] = "MUTATED"
	clone.Anomaly.IgnoreProcesses[0] = "MUTATED"
	clone.Rules[0].Name = "MUTATED"
	clone.AI.ExtraHeaders["X"] = "MUTATED"
	clone.WellKnownPorts[80] = "MUTATED"

	// Original should be unchanged.
	if cfg.Controller.ProtectedProcesses[0] != "a.exe" {
		t.Errorf("protected list not deep-copied: %v", cfg.Controller.ProtectedProcesses)
	}
	if cfg.Anomaly.IgnoreProcesses[0] != "c.exe" {
		t.Errorf("ignore list not deep-copied: %v", cfg.Anomaly.IgnoreProcesses)
	}
	if cfg.Rules[0].Name != "rule" {
		t.Errorf("rules not deep-copied: %+v", cfg.Rules[0])
	}
	if cfg.AI.ExtraHeaders["X"] != "Y" {
		t.Errorf("AI extra headers not deep-copied: %v", cfg.AI.ExtraHeaders)
	}
	if cfg.WellKnownPorts[80] != "HTTP" {
		t.Errorf("well-known ports not deep-copied: %v", cfg.WellKnownPorts[80])
	}
}

// ===== rules.go handleRulesUpdate edge cases =====

func TestHandleRulesUpdateNoCfgPath(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rules":[]}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleRulesUpdateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleRulesUpdateInvalidRule(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rules":[{"name":"","match":"x","metric":"cpu_percent"}]}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleRulesUpdateValidatesBadRule(t *testing.T) {
	// Rules with invalid metric reach the rulesFromDTO validation branch
	// before the disk save, so we cover the validation error path here.
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rules":[{"name":"r","match":"x","metric":"bogus"}]}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleRulesUpdateConfigValidateFails exercises the next.Validate()
// branch by pre-seeding the cfg with an invalid Server.Port.
func TestHandleRulesUpdateConfigValidateFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	cfg := defaultCfg()
	cfg.Server.Port = 70000 // invalid
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	s, _ := fullTestServer(t, cfgPath, cfg, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rules":[{"name":"r","match":"x","metric":"cpu_percent"}]}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleRulesUpdateSaveFails uses a path on a non-existent Windows
// drive to force config.Save → MkdirAll → failure.
func TestHandleRulesUpdateSaveFails(t *testing.T) {
	cfgPath := `Z:\nonexistent\wtm.yaml`
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rules":[{"name":"r","match":"x","metric":"cpu_percent"}]}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleRulesUpdate(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleConfigUpdateSaveFails covers the mutateConfig Save failure
// branch (handleConfigUpdate reports it as invalid_config).
func TestHandleConfigUpdateSaveFails(t *testing.T) {
	cfgPath := `Z:\nonexistent\wtm.yaml`
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid_config") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

// TestHandleConfigUpdateInvalidJSON covers the readJSON failure branch.
func TestHandleConfigUpdateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	s, _ := fullTestServer(t, cfgPath, nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleConfigUpdate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRulesFromDTOAllBranches(t *testing.T) {
	cases := []struct {
		name string
		in   []ruleDTO
		err  string
	}{
		{"valid", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent", Op: ">=", Threshold: 90, ForSeconds: 10, Action: "kill", CooldownSeconds: 60}}, ""},
		{"empty list", []ruleDTO{}, ""},
		{"blank name", []ruleDTO{{Name: "  ", Match: "x", Metric: "cpu_percent"}}, "name required"},
		{"dup name", []ruleDTO{
			{Name: "r", Match: "x", Metric: "cpu_percent"},
			{Name: "r", Match: "y", Metric: "cpu_percent"},
		}, "duplicate"},
		{"blank match", []ruleDTO{{Name: "r", Match: "", Metric: "cpu_percent"}}, "match pattern"},
		{"bad metric", []ruleDTO{{Name: "r", Match: "x", Metric: "bogus"}}, "metric must be"},
		{"bad op", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent", Op: "=="}}, "op must be"},
		{"bad action", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent", Action: "reboot"}}, "action must be"},
		{"for too large", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent", ForSeconds: 99999}}, "for_seconds"},
		{"negative cooldown", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent", CooldownSeconds: -5}}, ""},
		{"defaults op", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent"}}, ""},
		{"defaults action", []ruleDTO{{Name: "r", Match: "x", Metric: "cpu_percent"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := rulesFromDTO(c.in)
			if c.err == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q", c.err)
				return
			}
			if !strings.Contains(err.Error(), c.err) {
				t.Errorf("err=%v want substring %q", err, c.err)
			}
		})
	}
}

func TestRulesToDTOEmpty(t *testing.T) {
	out := rulesToDTO(nil)
	if len(out) != 0 {
		t.Errorf("out=%v", out)
	}
}
