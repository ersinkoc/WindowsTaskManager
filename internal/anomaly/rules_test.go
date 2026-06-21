package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestRulesDetectorNoRules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Rules = nil
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p"}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("no rules -> no alerts")
	}
}

func TestRulesDetectorSkipsDisabledAndEmptyMatch(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Rules = []config.Rule{
		{Name: "off", Enabled: false, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 1},
		{Name: "empty", Enabled: true, Match: "   ", Metric: "cpu_percent", Op: ">", Threshold: 1},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p", CPUPercent: 99}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled/empty-match rules should not alert")
	}
}

func TestRulesDetectorClearsOnNoHit(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 90, For: 0, Action: "alert"},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx) // raises
	if findActiveAlert(ctx.Alerts, "rule:r", 1) == nil {
		t.Fatal("expected rule alert raised")
	}
	// Drop CPU below threshold.
	ctx.Snapshot.Processes[0].CPUPercent = 1
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, "rule:r", 1); a != nil && a.Cleared == nil {
		t.Fatal("expected alert cleared on no-hit")
	}
}

func TestRulesDetectorRespectsForDuration(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 5 * time.Second, Action: "alert"},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, "rule:r", 1) != nil {
		t.Fatal("should not alert before For window elapses")
	}
	ctx.Now = t0.Add(10 * time.Second)
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, "rule:r", 1) == nil {
		t.Fatal("expected alert after For window")
	}
}

func TestRulesDetectorKillActionWithActuator(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "kill", Cooldown: 0},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	act := &fakeActuator{}
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(act.killed) != 1 || act.killed[0] != 1 {
		t.Fatalf("killed=%v want [1]", act.killed)
	}
	if a := findActiveAlert(ctx.Alerts, "rule:r", 1); a == nil || a.Severity != SeverityCritical {
		t.Fatalf("expected critical kill alert, got %+v", a)
	}
}

func TestRulesDetectorKillActionError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "kill", Cooldown: 0},
	}
	d := NewRulesDetector()
	act := &fakeActuator{killErr: errBoom()}
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx) // kill fails — just logs
	if len(act.killed) != 0 {
		t.Fatal("failed kill should not append to killed list")
	}
}

func TestRulesDetectorSuspendActionWithActuator(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "suspend", Cooldown: 0},
	}
	d := NewRulesDetector()
	act := &fakeActuator{}
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(act.suspended) != 1 || act.suspended[0] != 1 {
		t.Fatalf("suspended=%v want [1]", act.suspended)
	}
}

func TestRulesDetectorSuspendActionError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "suspend", Cooldown: 0},
	}
	d := NewRulesDetector()
	act := &fakeActuator{suspErr: errBoom()}
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(act.suspended) != 0 {
		t.Fatal("failed suspend should not append")
	}
}

func TestRulesDetectorRespectsCooldown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "kill", Cooldown: time.Hour},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	act := &fakeActuator{}
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(act.killed) != 1 {
		t.Fatal("first kill should happen")
	}
	ctx.Now = t0.Add(time.Second)
	d.Analyze(ctx)
	if len(act.killed) != 1 {
		t.Fatal("second kill should be suppressed by cooldown")
	}
}

func TestRulesDetectorDefaultCooldown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "kill", Cooldown: 0},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	act := &fakeActuator{}
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Actuator: act, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(act.killed) != 1 {
		t.Fatal("first kill should happen")
	}
	ctx.Now = t0.Add(time.Second)
	d.Analyze(ctx)
	if len(act.killed) != 1 {
		t.Fatal("default cooldown (1m) should suppress")
	}
}

func TestRulesDetectorNoActuatorSkipsAction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "kill"},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	// Just an alert, no action (no actuator).
	if a := findActiveAlert(ctx.Alerts, "rule:r", 1); a == nil {
		t.Fatal("expected alert even without actuator")
	}
}

func TestRulesDetectorUnknownActionDefaultsToAlert(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: ""},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, "rule:r", 1); a == nil {
		t.Fatal("expected alert (action defaults to alert)")
	}
}

func TestRulesDetectorGCStaleStates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "alert"},
	}
	d := NewRulesDetector()
	t0 := time.Now()
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if len(d.states) == 0 {
		t.Fatal("expected state")
	}
	// Remove the process.
	ctx.Snapshot.Processes = nil
	d.Analyze(ctx)
	if len(d.states) != 0 {
		t.Fatal("expected states GC'd")
	}
}

func TestRulesDetectorName(t *testing.T) {
	if got := NewRulesDetector().Name(); got != "rules" {
		t.Fatalf("Name()=%q want rules", got)
	}
}

func TestRulesDetectorSkipsNonMatchingProcess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "target", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "alert"},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		// Process name does not contain "target".
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "nomatch.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, "rule:r", 1) != nil {
		t.Fatal("non-matching process should not trigger rule")
	}
}

func TestRuleMetricValueAllCases(t *testing.T) {
	p := &metrics.ProcessInfo{CPUPercent: 12.5, WorkingSet: 1000, PrivateBytes: 2000, ThreadCount: 17}
	cases := []struct {
		metric string
		want   float64
		ok     bool
	}{
		{"cpu_percent", 12.5, true},
		{"cpu", 12.5, true},
		{"CPU", 12.5, true}, // case-insensitive
		{"memory_bytes", 1000, true},
		{"memory", 1000, true},
		{"working_set", 1000, true},
		{"private_bytes", 2000, true},
		{"thread_count", 17, true},
		{"threads", 17, true},
		{"unknown", 0, false},
		{"  cpu  ", 12.5, true}, // trimmed
	}
	for _, c := range cases {
		got, ok := ruleMetricValue(c.metric, p)
		if ok != c.ok || got != c.want {
			t.Fatalf("ruleMetricValue(%q) = (%v, %v) want (%v, %v)", c.metric, got, ok, c.want, c.ok)
		}
	}
}

func TestCompareRuleAllOps(t *testing.T) {
	cases := []struct {
		op    string
		v, th float64
		want  bool
	}{
		{">", 10, 5, true},
		{">", 5, 10, false},
		{">=", 5, 5, true},
		{">=", 4, 5, false},
		{"<", 4, 5, true},
		{"<", 5, 4, false},
		{"<=", 5, 5, true},
		{"<=", 6, 5, false},
		{"bogus", 1, 1, false}, // unknown op
		{"", 5, 5, true},       // default ">="
		{"   ", 5, 5, true},    // whitespace -> default ">="
	}
	for _, c := range cases {
		if got := compareRule(c.v, c.op, c.th); got != c.want {
			t.Fatalf("compareRule(%v,%q,%v) = %v want %v", c.v, c.op, c.th, got, c.want)
		}
	}
}

func TestRuleOpOrDefault(t *testing.T) {
	if got := ruleOpOrDefault(">"); got != ">" {
		t.Fatalf("ruleOpOrDefault(>)=%q", got)
	}
	if got := ruleOpOrDefault(""); got != ">=" {
		t.Fatalf("ruleOpOrDefault('')=%q", got)
	}
	if got := ruleOpOrDefault("  > "); got != ">" {
		t.Fatalf("ruleOpOrDefault('  > ')=%q", got)
	}
}

func TestRulesDetectorIgnoresIgnoredProcess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = []string{"skip.exe"}
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "skip", Metric: "cpu_percent", Op: ">", Threshold: 50, For: 0, Action: "alert"},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "skip.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, "rule:r", 1) != nil {
		t.Fatal("ignored process should not trigger rule")
	}
}

func TestRulesDetectorUnknownMetricSkips(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.IgnoreProcesses = nil
	cfg.Rules = []config.Rule{
		{Name: "r", Enabled: true, Match: "p", Metric: "bogus", Op: ">", Threshold: 50, For: 0, Action: "alert"},
	}
	d := NewRulesDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 99}}},
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, "rule:r", 1) != nil {
		t.Fatal("unknown metric should skip rule")
	}
}

// errBoom returns a simple error for fakeActuator.
func errBoom() error { return &simpleErr{s: "boom"} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
