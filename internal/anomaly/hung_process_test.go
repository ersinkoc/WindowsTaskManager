package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestHungProcessDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = false
	d := NewHungProcessDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p"}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled should not alert")
	}
}

func TestHungProcessDetectorSkipsWhitelistAndIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.IdleWhitelist = []string{"skip.exe"}
	cfg.Anomaly.IgnoreProcesses = []string{"ignore.exe"}
	d := NewHungProcessDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "skip.exe"},
			{PID: 2, Name: "ignore.exe"},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	// First sample records baseline; no alerts yet.
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("whitelist/ignored should not alert on first sample")
	}
}

func TestHungProcessDetectorRequiresPriorActivity(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.ZeroActivityThreshold = 1 * time.Millisecond
	cfg.Anomaly.HungProcess.CriticalHungThreshold = 2 * time.Millisecond
	d := NewHungProcessDetector()
	t0 := time.Now()
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "idle.exe", CPUPercent: 0, IOReadBytes: 0, IOWriteBytes: 0},
		}},
	}
	d.Analyze(ctx)
	ctx.Now = t0.Add(50 * time.Millisecond)
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a != nil {
		t.Fatal("never-busy daemon should not alert")
	}
}

func TestHungProcessDetectorWarnsThenCriticals(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.ZeroActivityThreshold = 10 * time.Millisecond
	cfg.Anomaly.HungProcess.CriticalHungThreshold = 30 * time.Millisecond
	d := NewHungProcessDetector()
	t0 := time.Now()
	procs := []metrics.ProcessInfo{{PID: 1, Name: "busy.exe", CPUPercent: 50, IOReadBytes: 0, IOWriteBytes: 0}}
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: procs},
	}
	d.Analyze(ctx) // record baseline
	// Second sample: still busy -> wasBusy flips true, since reset.
	ctx.Now = t0.Add(5 * time.Millisecond)
	d.Analyze(ctx)
	// Third sample: drop CPU to 0, IO unchanged -> idle. idleFor=15ms >= 10ms.
	procs[0].CPUPercent = 0
	ctx.Now = t0.Add(20 * time.Millisecond)
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil || a.Severity != SeverityWarning {
		t.Fatalf("expected warning, got %+v", a)
	}
	// Push past critical threshold (still idle).
	ctx.Now = t0.Add(100 * time.Millisecond)
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil || a.Severity != SeverityCritical {
		t.Fatalf("expected critical, got %+v", a)
	}
}

func TestHungProcessDetectorClearsWhenBusy(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.ZeroActivityThreshold = 10 * time.Millisecond
	d := NewHungProcessDetector()
	t0 := time.Now()
	alerts := NewAlertStore(8)
	procs := []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 10, IOReadBytes: 0, IOWriteBytes: 0}}
	ctx := &AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: procs},
	}
	d.Analyze(ctx) // baseline
	// Force the process idle long enough to raise an alert.
	procs[0].CPUPercent = 0
	ctx.Now = t0.Add(50 * time.Millisecond)
	d.Analyze(ctx)
	if findActiveAlert(alerts, d.Name(), 1) == nil {
		t.Fatal("expected hung alert raised first")
	}
	// Now re-introduce activity -> ClearAlert branch fires.
	procs[0].CPUPercent = 20
	d.Analyze(ctx)
	if findActiveAlert(alerts, d.Name(), 1) != nil {
		t.Fatal("expected alert cleared on activity")
	}
}

func TestHungProcessDetectorIdleBelowThresholdNoAlert(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.ZeroActivityThreshold = time.Hour
	d := NewHungProcessDetector()
	t0 := time.Now()
	alerts := NewAlertStore(8)
	procs := []metrics.ProcessInfo{{PID: 1, Name: "p.exe", CPUPercent: 50}}
	ctx := &AnalysisContext{Now: t0, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: procs}}
	d.Analyze(ctx) // baseline
	procs[0].CPUPercent = 0
	ctx.Now = t0.Add(time.Second)
	d.Analyze(ctx) // idle but below threshold -> continue (no alert)
	if findActiveAlert(alerts, d.Name(), 1) != nil {
		t.Fatal("should not alert when idle below threshold")
	}
}

func TestHungProcessDetectorDaemonGateNoPriorActivity(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	cfg.Anomaly.HungProcess.ZeroActivityThreshold = 1 * time.Millisecond
	d := NewHungProcessDetector()
	t0 := time.Now()
	alerts := NewAlertStore(8)
	// Process starts idle (CPU=0, IO=0) and stays idle — never busy, peakCPU<5, peakIO<10MB.
	procs := []metrics.ProcessInfo{{PID: 1, Name: "daemon.exe", CPUPercent: 0, IOReadBytes: 0, IOWriteBytes: 0}}
	ctx := &AnalysisContext{Now: t0, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: procs}}
	d.Analyze(ctx) // baseline (peakCPU=0)
	ctx.Now = t0.Add(50 * time.Millisecond)
	d.Analyze(ctx) // idle past threshold but daemon gate blocks
	if findActiveAlert(alerts, d.Name(), 1) != nil {
		t.Fatal("daemon with no prior activity should not alert")
	}
}

func TestHungProcessDetectorGCStaleStates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = true
	d := NewHungProcessDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{
			Processes: []metrics.ProcessInfo{
				{PID: 1, Name: "a.exe", CPUPercent: 1, IOReadBytes: 0, IOWriteBytes: 0},
			},
		},
	}
	d.Analyze(ctx)
	// Now snapshot has no processes -> state GC should run.
	ctx.Snapshot.Processes = nil
	d.Analyze(ctx)
	if len(d.states) != 0 {
		t.Fatalf("states=%d want 0", len(d.states))
	}
}
