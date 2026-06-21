package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestRunawayCPUDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.RunawayCPU.Enabled = false
	d := NewRunawayCPUDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p", CPUPercent: 99}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled should not alert")
	}
}

func TestRunawayCPUDetectorSkipsWhitelistAndIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.RunawayCPU.Enabled = true
	cfg.Anomaly.RunawayCPU.HighCPUWhitelist = []string{"white.exe"}
	cfg.Anomaly.IgnoreProcesses = []string{"ignore.exe"}
	d := NewRunawayCPUDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "white.exe", CPUPercent: 99},
			{PID: 2, Name: "ignore.exe", CPUPercent: 99},
		}},
	}
	d.Analyze(ctx)
	d.Analyze(ctx) // give time
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("whitelist/ignored should not alert")
	}
}

func TestRunawayCPUDetectorClearsWhenBelowThreshold(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.RunawayCPU.Enabled = true
	cfg.Anomaly.RunawayCPU.CPUThreshold = 80
	cfg.Anomaly.RunawayCPU.DurationThreshold = 1 * time.Millisecond
	d := NewRunawayCPUDetector()
	alerts := NewAlertStore(8)
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "p.exe", CPUPercent: 90},
		}},
	}
	d.Analyze(ctx) // records state
	d.Analyze(ctx) // alerts (warning)
	ctx.Now = now.Add(50 * time.Millisecond)
	ctx.Snapshot.Processes[0].CPUPercent = 10
	d.Analyze(ctx)
	if a := findActiveAlert(alerts, d.Name(), 1); a != nil && a.Cleared == nil {
		t.Fatal("expected alert cleared after CPU dropped")
	}
}

func TestRunawayCPUDetectorGCStaleStates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.RunawayCPU.Enabled = true
	d := NewRunawayCPUDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "p.exe", CPUPercent: 95},
		}},
	}
	d.Analyze(ctx)
	if _, ok := d.states[1]; !ok {
		t.Fatal("expected state recorded")
	}
	ctx.Snapshot.Processes = nil
	d.Analyze(ctx)
	if _, ok := d.states[1]; ok {
		t.Fatal("expected state GC'd")
	}
}
