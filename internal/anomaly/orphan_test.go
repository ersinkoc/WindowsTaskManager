package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestOrphanDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = false
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, ParentPID: 99}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled detector should not alert")
	}
}

func TestOrphanDetectorSkipsParentZeroAndAlive(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 0, Name: "zero.exe"},                  // ParentPID == 0 -> skip
			{PID: 2, ParentPID: 3, Name: "alive.exe", CPUPercent: 99}, // parent 3 alive -> skip
			{PID: 3, ParentPID: 0, Name: "parent.exe"},                // parent of PID 2
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("expected no alerts for non-orphans")
	}
}

func TestOrphanDetectorSkipsIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	cfg.Anomaly.IgnoreProcesses = []string{"skipme.exe"}
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 99, Name: "skipme.exe", CPUPercent: 99, WorkingSet: 2 << 30},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("ignored process should not raise")
	}
}

func TestOrphanDetectorSkipsLowResource(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 99, Name: "quiet.exe", CPUPercent: 1, WorkingSet: 1 << 20},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("low-resource orphan should not alert")
	}
}

func TestOrphanDetectorRaisesHighCPU(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 99, Name: "orphanhi.exe", CPUPercent: 50},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil {
		t.Fatal("expected high-cpu orphan alert")
	}
}

func TestOrphanDetectorRaisesHighMemory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	cfg.Anomaly.Orphan.ResourceThresholdMemory = "2GB"
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 99, Name: "orphanmem.exe", CPUPercent: 1, WorkingSet: 3 << 30},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil {
		t.Fatal("expected high-mem orphan alert")
	}
}

func TestOrphanDetectorFloorsThresholdsBelowMin(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.Orphan.Enabled = true
	cfg.Anomaly.Orphan.ResourceThresholdCPU = 1       // below min
	cfg.Anomaly.Orphan.ResourceThresholdMemory = "1B" // below 1GB min
	d := NewOrphanDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 99, Name: "tiny.exe", CPUPercent: 11, WorkingSet: 1<<30 + 1},
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil {
		t.Fatal("expected orphan alert with floored thresholds")
	}
}
