package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestCloneRunNil(t *testing.T) {
	if got := cloneRun(nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCloneRunCopiesActions(t *testing.T) {
	orig := &BackgroundRun{
		Trigger: "t",
		Actions: []Suggestion{{ID: "x", Type: "ignore", Name: "n"}},
	}
	cp := cloneRun(orig)
	if cp == orig {
		t.Error("expected new pointer")
	}
	if len(cp.Actions) != 1 || cp.Actions[0].ID != "x" {
		t.Errorf("actions not copied: %+v", cp.Actions)
	}
	cp.Actions[0].Name = "mutated"
	if orig.Actions[0].Name == "mutated" {
		t.Error("mutation leaked to original")
	}
}

func TestCloneRunNoActions(t *testing.T) {
	orig := &BackgroundRun{Trigger: "t"}
	cp := cloneRun(orig)
	if cp == nil || cp.Trigger != "t" {
		t.Errorf("clone broken: %+v", cp)
	}
	if cp.Actions != nil {
		t.Errorf("expected nil actions, got %+v", cp.Actions)
	}
}

func TestCountRecentCycles(t *testing.T) {
	now := time.Now()
	starts := []time.Time{
		now.Add(-5 * time.Minute),
		now.Add(-30 * time.Minute),
		now.Add(-59 * time.Minute),
		now.Add(-61 * time.Minute),
		now.Add(-2 * time.Hour),
	}
	if got := countRecentCycles(starts, now); got != 3 {
		t.Errorf("got %d, want 3 (within last hour)", got)
	}
	if got := countRecentCycles(nil, now); got != 0 {
		t.Errorf("empty: got %d", got)
	}
}

func TestApplyConfigTruncatesHistory(t *testing.T) {
	b := &backgroundTracker{}
	for i := 0; i < 10; i++ {
		b.recentRuns = append(b.recentRuns, BackgroundRun{Trigger: "t"})
	}
	cfg := &config.Config{AI: config.AIConfig{Scheduler: config.AISchedulerConfig{HistoryLimit: 3}}}
	b.applyConfig(cfg)
	if len(b.recentRuns) != 3 {
		t.Errorf("history size = %d, want 3", len(b.recentRuns))
	}
}

func TestApplyConfigNilCfgDefaultsToOne(t *testing.T) {
	b := &backgroundTracker{}
	b.applyConfig(nil)
	if b.historyLimit != 1 {
		t.Errorf("historyLimit = %d, want 1", b.historyLimit)
	}
}

func TestApplyConfigNegativeLimitClampedToOne(t *testing.T) {
	b := &backgroundTracker{}
	cfg := &config.Config{AI: config.AIConfig{Scheduler: config.AISchedulerConfig{HistoryLimit: -3}}}
	b.applyConfig(cfg)
	if b.historyLimit != 1 {
		t.Errorf("historyLimit = %d, want 1 (clamped)", b.historyLimit)
	}
}

func TestTrimCycleStarts(t *testing.T) {
	now := time.Now()
	starts := []time.Time{
		now.Add(-5 * time.Minute),
		now.Add(-2 * time.Hour),
		now.Add(-30 * time.Minute),
	}
	got := trimCycleStarts(starts, now)
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestResetReservedDayChangesOnNewDay(t *testing.T) {
	b := &backgroundTracker{
		reservedDay:         "2020-01-01",
		reservedTokensToday: 500,
	}
	now := time.Now()
	resetReservedDay(b, now)
	if b.reservedDay != now.Format("2006-01-02") {
		t.Errorf("reservedDay = %q", b.reservedDay)
	}
	if b.reservedTokensToday != 0 {
		t.Errorf("reservedTokensToday = %d, want 0", b.reservedTokensToday)
	}
}

func TestResetReservedDayNoChangeSameDay(t *testing.T) {
	now := time.Now()
	b := &backgroundTracker{
		reservedDay:         now.Format("2006-01-02"),
		reservedTokensToday: 500,
	}
	resetReservedDay(b, now)
	if b.reservedTokensToday != 500 {
		t.Errorf("reservedTokensToday = %d, want 500 (unchanged)", b.reservedTokensToday)
	}
}

func TestRecordBackgroundSkip(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	now := time.Now()
	a.recordBackgroundSkip(now, "test_reason")
	if a.bg.lastSkipReason != "test_reason" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
	if !a.bg.lastEventAt.Equal(now) {
		t.Errorf("lastEventAt = %v, want %v", a.bg.lastEventAt, now)
	}
}

func TestHandleRaisedAlertIgnoresNonAlert(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	a.handleRaisedAlert("not an alert")
	a.handleRaisedAlert(nil)
	if !a.bg.lastEventAt.IsZero() {
		t.Errorf("lastEventAt should be zero, got %v", a.bg.lastEventAt)
	}
}

func TestMaybeScheduleBackgroundNoConfig(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	a.mu.Lock()
	a.cfg = nil
	a.mu.Unlock()
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "no_config" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundSchedulerDisabled(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5, Scheduler: config.AISchedulerConfig{Enabled: false}}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "scheduler_disabled" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundAutoAnalyzeDisabled(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true},
		AutoAnalyzeOnCritical: false,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "auto_analyze_disabled" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundAINotConfigured(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: false, APIKey: "", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "ai_not_configured" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundNonCriticalAlert(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityWarning})
	if a.bg.lastSkipReason != "non_critical_alert" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundInFlight(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 5000, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.bgMu.Lock()
	a.bg.inFlight = true
	a.bgMu.Unlock()

	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical, Type: "runaway_cpu", Title: "x", PID: 1, ProcessName: "a.exe"})

	if a.bg.lastSkipReason != "background_inflight" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundErrorCooldown(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 5000, CooldownAfterError: 1 * time.Minute, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.bgMu.Lock()
	a.bg.cooldownUntil = time.Now().Add(1 * time.Hour)
	a.bgMu.Unlock()
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "error_cooldown" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundMinInterval(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 1 * time.Hour, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 5000, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.bgMu.Lock()
	a.bg.lastStartedAt = time.Now().Add(-1 * time.Minute)
	a.bgMu.Unlock()
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "min_interval" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundCycleLimit(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 1, MaxReservedTokensPerDay: 5000, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	now := time.Now()
	a.bgMu.Lock()
	a.bg.cycleStarts = []time.Time{now.Add(-1 * time.Minute)}
	a.bgMu.Unlock()
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "cycle_limit" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundTokenBudget(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, APIKey: "k", MaxTokens: 1024, MaxRequestsPerMinute: 5,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 1024, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.bgMu.Lock()
	a.bg.reservedDay = time.Now().Format("2006-01-02")
	a.bg.reservedTokensToday = 500
	a.bgMu.Unlock()
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical})
	if a.bg.lastSkipReason != "token_budget" {
		t.Errorf("lastSkipReason = %q", a.bg.lastSkipReason)
	}
}

func TestMaybeScheduleBackgroundReservedTokenFloor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 0, MaxRequestsPerMinute: 60,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 5000, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical, Type: "runaway_cpu", Title: "x", PID: 1, ProcessName: "a.exe"})

	waitFor(t, 2*time.Second, func() bool {
		return a.BackgroundState().LastRun != nil
	})

	if a.bg.reservedTokensToday != 1 {
		t.Errorf("reservedTokensToday = %d, want 1 (floor)", a.bg.reservedTokensToday)
	}
}

func TestMaybeScheduleBackgroundSchedulesRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"scheduled"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
		Scheduler:             config.AISchedulerConfig{Enabled: true, MinInterval: 0, MaxCyclesPerHour: 5, MaxReservedTokensPerDay: 5000, CooldownAfterError: 0, HistoryLimit: 4},
		AutoAnalyzeOnCritical: true,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	a.maybeScheduleBackground(anomaly.Alert{Severity: anomaly.SeverityCritical, Type: "runaway_cpu", Title: "x", PID: 1, ProcessName: "a.exe"})

	waitFor(t, 2*time.Second, func() bool {
		return a.BackgroundState().LastRun != nil
	})
	if len(a.bg.cycleStarts) != 1 {
		t.Errorf("cycleStarts = %d, want 1", len(a.bg.cycleStarts))
	}
}

func TestCloneConfigNil(t *testing.T) {
	if got := cloneConfig(nil); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCloneConfigCopiesAllFields(t *testing.T) {
	orig := config.DefaultConfig()
	orig.Controller.ProtectedProcesses = []string{"a.exe", "b.exe"}
	orig.Anomaly.IgnoreProcesses = []string{"x"}
	orig.Rules = []config.Rule{{Name: "r"}}
	orig.AI.ExtraHeaders = map[string]string{"K": "V"}
	orig.WellKnownPorts = map[uint16]string{80: "http"}

	cp := cloneConfig(orig)
	if cp == orig {
		t.Error("expected new pointer")
	}
	cp.Controller.ProtectedProcesses[0] = "MUTATED"
	if orig.Controller.ProtectedProcesses[0] == "MUTATED" {
		t.Error("mutation leaked to original ProtectedProcesses")
	}
	cp.Anomaly.IgnoreProcesses[0] = "MUTATED"
	if orig.Anomaly.IgnoreProcesses[0] == "MUTATED" {
		t.Error("mutation leaked to original IgnoreProcesses")
	}
	cp.Rules[0].Name = "MUTATED"
	if orig.Rules[0].Name == "MUTATED" {
		t.Error("mutation leaked to original Rules")
	}
	cp.AI.ExtraHeaders["K"] = "MUTATED"
	if orig.AI.ExtraHeaders["K"] == "MUTATED" {
		t.Error("mutation leaked to original ExtraHeaders")
	}
	cp.WellKnownPorts[80] = "MUTATED"
	if orig.WellKnownPorts[80] == "MUTATED" {
		t.Error("mutation leaked to original WellKnownPorts")
	}
}

func TestEvaluateAutoPolicyDisabled(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{Enabled: false}}}
	p := evaluateAutoPolicy(cfg, nil, Suggestion{Type: "ignore"})
	if p.Status != "disabled" {
		t.Errorf("status = %q", p.Status)
	}
}

func TestEvaluateAutoPolicyNotDryRun(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{Enabled: true, DryRun: false}}}
	p := evaluateAutoPolicy(cfg, nil, Suggestion{Type: "ignore"})
	if p.Status != "blocked" {
		t.Errorf("status = %q", p.Status)
	}
	if !strings.Contains(p.Reason, "live_auto_execute_not_implemented") {
		t.Errorf("reason = %q", p.Reason)
	}
}

func TestEvaluateAutoPolicyDestructiveActionBlocked(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{
		Enabled: true, DryRun: true, AllowedActions: []string{"kill"},
	}}}
	for _, typ := range []string{"kill", "suspend"} {
		p := evaluateAutoPolicy(cfg, nil, Suggestion{Type: typ})
		if p.Status != "blocked" {
			t.Errorf("%s: status = %q", typ, p.Status)
		}
		if !strings.Contains(p.Reason, "destructive_actions_not_supported") {
			t.Errorf("%s: reason = %q", typ, p.Reason)
		}
	}
}

func TestEvaluateAutoPolicyActionNotAllowlisted(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{
		Enabled: true, DryRun: true, AllowedActions: []string{"ignore"},
	}}}
	p := evaluateAutoPolicy(cfg, nil, Suggestion{Type: "protect"})
	if p.Status != "blocked" {
		t.Errorf("status = %q", p.Status)
	}
	if !strings.Contains(p.Reason, "action_not_allowlisted") {
		t.Errorf("reason = %q", p.Reason)
	}
}

func TestEvaluateAutoPolicyNeedsRepeat(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{
		Enabled: true, DryRun: true, AllowedActions: []string{"ignore"}, RequireRepeatCycles: 3,
	}}}
	runs := []BackgroundRun{{Actions: []Suggestion{{ID: "x", Type: "ignore"}}}}
	p := evaluateAutoPolicy(cfg, runs, Suggestion{ID: "x", Type: "ignore"})
	if p.Status != "needs_repeat" {
		t.Errorf("status = %q", p.Status)
	}
	if p.RepeatCount != 2 {
		t.Errorf("RepeatCount = %d", p.RepeatCount)
	}
	if p.RequiredRepeatCount != 3 {
		t.Errorf("RequiredRepeatCount = %d", p.RequiredRepeatCount)
	}
}

func TestEvaluateAutoPolicyDryRunEligible(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{
		Enabled: true, DryRun: true, AllowedActions: []string{"ignore"}, RequireRepeatCycles: 1,
	}}}
	runs := []BackgroundRun{
		{Actions: []Suggestion{{ID: "x", Type: "ignore"}}},
		{Actions: []Suggestion{{ID: "x", Type: "ignore"}}},
	}
	p := evaluateAutoPolicy(cfg, runs, Suggestion{ID: "x", Type: "ignore"})
	if p.Status != "dry_run_eligible" {
		t.Errorf("status = %q", p.Status)
	}
	if p.RepeatCount != 3 {
		t.Errorf("RepeatCount = %d", p.RepeatCount)
	}
}

func TestEvaluateAutoPolicyRequireRepeatClampedToOne(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{
		Enabled: true, DryRun: true, AllowedActions: []string{"ignore"}, RequireRepeatCycles: 0,
	}}}
	p := evaluateAutoPolicy(cfg, nil, Suggestion{Type: "ignore"})
	if p.RequiredRepeatCount != 1 {
		t.Errorf("RequiredRepeatCount = %d, want 1 (clamped)", p.RequiredRepeatCount)
	}
}

func TestEvaluateAutoPolicyNilCfg(t *testing.T) {
	p := evaluateAutoPolicy(nil, nil, Suggestion{Type: "ignore"})
	if p.Status != "disabled" {
		t.Errorf("status = %q", p.Status)
	}
}

func TestCountSuggestionRepeatsEmptyID(t *testing.T) {
	runs := []BackgroundRun{{Actions: []Suggestion{{ID: "x"}}}}
	if got := countSuggestionRepeats(runs, ""); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountSuggestionRepeatsCounts(t *testing.T) {
	runs := []BackgroundRun{
		{Actions: []Suggestion{{ID: "x"}, {ID: "y"}}},
		{Actions: []Suggestion{{ID: "x"}}},
	}
	if got := countSuggestionRepeats(runs, "x"); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := countSuggestionRepeats(runs, "z"); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestCountAutoCandidates(t *testing.T) {
	actions := []Suggestion{
		{Policy: &AutoPolicy{Status: "dry_run_eligible"}},
		{Policy: &AutoPolicy{Status: "blocked"}},
		{Policy: &AutoPolicy{Status: "dry_run_eligible"}},
		{Policy: nil},
		{Policy: &AutoPolicy{Status: "needs_repeat"}},
	}
	if got := countAutoCandidates(actions); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}

func TestContainsFold(t *testing.T) {
	if !containsFold([]string{"ignore", "protect"}, "ignore") {
		t.Error("expected match for ignore")
	}
	if !containsFold([]string{"IGNORE", "PROTECT"}, "ignore") {
		t.Error("expected case-insensitive match")
	}
	if !containsFold([]string{"  ignore  "}, "ignore") {
		t.Error("expected whitespace-insensitive match")
	}
	if containsFold([]string{"ignore"}, "protect") {
		t.Error("did not expect match")
	}
	if containsFold(nil, "ignore") {
		t.Error("did not expect match for nil")
	}
	if !containsFold([]string{"ignore"}, "  ignore  ") {
		t.Error("expected target trim to match")
	}
}

func TestBuildBackgroundPromptAllFields(t *testing.T) {
	a := anomaly.Alert{
		Type:        "runaway_cpu",
		Title:       "Runaway CPU",
		Description: "node.exe burning CPU",
		PID:         123,
		ProcessName: "node.exe",
	}
	prompt := buildBackgroundPrompt(a)
	wantSubs := []string{
		"CRITICAL SAFETY RULES",
		"Alert type: runaway_cpu",
		"Title: Runaway CPU",
		"Description: node.exe burning CPU",
		"PID: 123",
		"Process: node.exe",
	}
	for _, sub := range wantSubs {
		if !strings.Contains(prompt, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestBuildBackgroundPromptMinimalAlert(t *testing.T) {
	a := anomaly.Alert{Type: "x", Title: "t"}
	prompt := buildBackgroundPrompt(a)
	if !strings.Contains(prompt, "Alert type: x") {
		t.Error("missing Alert type")
	}
	if strings.Contains(prompt, "Description:") {
		t.Error("did not expect Description line")
	}
	if strings.Contains(prompt, "PID:") {
		t.Error("did not expect PID line")
	}
	if strings.Contains(prompt, "Process:") {
		t.Error("did not expect Process line")
	}
}

func TestApplyAutoPolicyLockedEmpty(t *testing.T) {
	a := NewAdvisor(&config.Config{AI: config.AIConfig{MaxRequestsPerMinute: 5}}, storage.NewStore(60, 10), nil, nil)
	out := a.applyAutoPolicyLocked(&config.Config{}, nil)
	if out != nil {
		t.Errorf("expected nil for empty input, got %+v", out)
	}
}

func TestApplyAutoPolicyLockedSetsPolicy(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{AutoAction: config.AIAutoActionConfig{Enabled: false}}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	actions := []Suggestion{{Type: "ignore", Name: "x"}}
	out := a.applyAutoPolicyLocked(cfg, actions)
	if len(out) != 1 || out[0].Policy == nil || out[0].Policy.Status != "disabled" {
		t.Errorf("expected disabled policy on each action, got %+v", out)
	}
}

func TestRunBackgroundAnalysisEmitsEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
	}}
	em := event.NewEmitter()
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, em)

	got := make(chan any, 1)
	em.On(EventBackgroundAnalysis, func(d any) {
		got <- d
	})

	a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
		Type: "runaway_cpu", Severity: anomaly.SeverityCritical,
		Title: "x", PID: 1, ProcessName: "a.exe",
	}, 128, time.Now())

	select {
	case d := <-got:
		run, ok := d.(*BackgroundRun)
		if !ok || run == nil {
			t.Fatalf("unexpected event payload: %+v", d)
		}
		if run.Error != "" {
			t.Errorf("unexpected error: %q", run.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for background event")
	}
}

func TestRunBackgroundAnalysisErrorCooldown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`nope`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
		Scheduler: config.AISchedulerConfig{CooldownAfterError: 1 * time.Minute},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
		Type: "x", Severity: anomaly.SeverityCritical, Title: "x", PID: 1, ProcessName: "a.exe",
	}, 128, time.Now())

	run := a.BackgroundState().LastRun
	if run == nil {
		t.Fatal("expected run")
	}
	if run.Error == "" {
		t.Error("expected error")
	}
	if a.bg.cooldownUntil.IsZero() {
		t.Error("expected cooldown to be set")
	}
	if a.bg.lastError == "" {
		t.Error("expected lastError to be set")
	}
}

func TestRunBackgroundAnalysisHistoryLimitTrims(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
		Scheduler: config.AISchedulerConfig{HistoryLimit: 2},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	for i := 0; i < 5; i++ {
		a.bgMu.Lock()
		a.bg.recentRuns = append(a.bg.recentRuns, BackgroundRun{Trigger: "seed"})
		a.bgMu.Unlock()
	}

	a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
		Type: "x", Severity: anomaly.SeverityCritical, Title: "x", PID: 1, ProcessName: "a.exe",
	}, 128, time.Now())

	if got := len(a.bg.recentRuns); got != 2 {
		t.Errorf("recentRuns = %d, want 2 (history limit)", got)
	}
}

func TestRunBackgroundAnalysisCachedReturnsTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"cached answer"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
		Scheduler: config.AISchedulerConfig{HistoryLimit: 4},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	// Prime the cache with the exact prompt that runBackgroundAnalysis will use.
	bgPrompt := buildBackgroundPrompt(anomaly.Alert{
		Type: "x", Severity: anomaly.SeverityCritical, Title: "x", PID: 1, ProcessName: "a.exe",
	})
	if _, err := a.Analyze(context.Background(), bgPrompt); err != nil {
		t.Fatalf("prime: %v", err)
	}
	a.bgMu.Lock()
	a.bg.reservedDay = time.Now().Format("2006-01-02")
	a.bg.reservedTokensToday = 128
	a.bgMu.Unlock()

	a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
		Type: "x", Severity: anomaly.SeverityCritical, Title: "x", PID: 1, ProcessName: "a.exe",
	}, 128, time.Now())

	run := a.BackgroundState().LastRun
	if run == nil {
		t.Fatal("expected run")
	}
	if !run.Cached {
		t.Errorf("expected cached=true, got %+v", run)
	}
}

func TestRunBackgroundAnalysisNoEmitterOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, MaxTokens: 256, MaxRequestsPerMinute: 60,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
		Type: "x", Severity: anomaly.SeverityCritical, Title: "x", PID: 1, ProcessName: "a.exe",
	}, 128, time.Now())
	run := a.BackgroundState().LastRun
	if run == nil {
		t.Fatal("expected run")
	}
}

func TestSnapshotBudgetFields(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Scheduler: config.AISchedulerConfig{
			Enabled:                 true,
			MaxCyclesPerHour:        10,
			MaxReservedTokensPerDay: 2000,
			MinInterval:             30 * time.Second,
		},
		AutoAnalyzeOnCritical: true,
		AutoAction:            config.AIAutoActionConfig{Enabled: true, DryRun: true},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	st := a.BackgroundState()
	if !st.Enabled {
		t.Errorf("Enabled = %v", st.Enabled)
	}
	if !st.AutoAnalyzeOnCritical {
		t.Errorf("AutoAnalyzeOnCritical = %v", st.AutoAnalyzeOnCritical)
	}
	if !st.AutoActionEnabled {
		t.Errorf("AutoActionEnabled = %v", st.AutoActionEnabled)
	}
	if !st.AutoActionDryRun {
		t.Errorf("AutoActionDryRun = %v", st.AutoActionDryRun)
	}
	if st.Budget.MaxCyclesPerHour != 10 {
		t.Errorf("MaxCyclesPerHour = %d", st.Budget.MaxCyclesPerHour)
	}
	if st.Budget.MaxReservedTokensPerDay != 2000 {
		t.Errorf("MaxReservedTokensPerDay = %d", st.Budget.MaxReservedTokensPerDay)
	}
	if st.Budget.MinIntervalSeconds != 30 {
		t.Errorf("MinIntervalSeconds = %d", st.Budget.MinIntervalSeconds)
	}
}

func TestSnapshotConfiguredFalseWhenKeyEmpty(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Enabled: true, MaxRequestsPerMinute: 5}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if a.BackgroundState().Configured {
		t.Error("expected Configured=false when key empty")
	}
}
