package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestPortConflictDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = false
	d := NewPortConflictDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{{PID: 1, State: "time-wait", Since: now.Add(-time.Hour).Unix()}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled detector should not alert")
	}
}

func TestPortConflictDetectorNilBindings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = true
	d := NewPortConflictDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("nil bindings should not alert")
	}
}

func TestPortConflictDetectorUnknownState(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = true
	d := NewPortConflictDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{{PID: 1, State: "established", Since: now.Add(-time.Hour).Unix()}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("unknown state should not alert")
	}
}

func TestPortConflictDetectorBelowThreshold(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = true
	d := NewPortConflictDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{{
				PID: 1, State: "time-wait", Since: now.Add(-time.Second).Unix(),
			}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("below threshold should not alert")
	}
}

func TestPortConflictDetectorTimeWait(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = true
	cfg.Anomaly.PortConflict.TimeWaitThreshold = 60 * time.Second
	d := NewPortConflictDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{{
				PID: 1, Process: "p.exe", State: "time-wait", Since: now.Add(-2 * time.Minute).Unix(),
				Protocol: "tcp", LocalPort: 80,
			}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 1); a == nil {
		t.Fatal("expected time-wait alert")
	}
}

func TestPortConflictDetectorCloseWait(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.PortConflict.Enabled = true
	cfg.Anomaly.PortConflict.CloseWaitThreshold = 30 * time.Second
	d := NewPortConflictDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now,
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{{
				PID: 2, Process: "p.exe", State: "close-wait", Since: now.Add(-time.Minute).Unix(),
				Protocol: "tcp", LocalPort: 443,
			}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 2); a == nil {
		t.Fatal("expected close-wait alert")
	}
}
