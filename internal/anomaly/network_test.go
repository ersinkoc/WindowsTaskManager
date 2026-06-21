package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestNetworkAnomalyDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = false
	d := NewNetworkAnomalyDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{PortBindings: repeatBindings(1, "p", 50)},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled should not alert")
	}
}

func TestNetworkAnomalyDetectorNilBindings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	d := NewNetworkAnomalyDetector()
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

func TestNetworkAnomalyDetectorIgnoresZeroPIDBindings(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 1000
	d := NewNetworkAnomalyDetector()
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{PortBindings: []metrics.PortBinding{
			{PID: 0, Process: "kernel"}, // skipped
		}},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("zero-pid bindings should be skipped")
	}
}

func TestNetworkAnomalyDetectorClearsSystemAlert(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 1000
	d := NewNetworkAnomalyDetector()
	alerts := NewAlertStore(8)
	// First raise a system alert.
	alerts.Raise(Alert{Type: d.Name() + "_system", Severity: SeverityCritical})
	ctx := &AnalysisContext{
		Now: time.Now(),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			Processes:    []metrics.ProcessInfo{{PID: 5, Name: "p"}},
			PortBindings: []metrics.PortBinding{}, // non-nil, below cap -> clears
		},
		Alerts: alerts,
	}
	d.Analyze(ctx)
	if findActiveAlert(alerts, d.Name()+"_system", 0) != nil {
		t.Fatal("system alert should be cleared when below cap")
	}
}

func TestNetworkAnomalyDetectorRequiresMinimumBurstAndCount(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.ConnectionSigma = 1
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 100000
	d := NewNetworkAnomalyDetector()
	now := time.Now()
	// Establish baseline < 30 conns for >10 samples so Welford.Count() >= 10.
	for i := 0; i < 12; i++ {
		ctx := &AnalysisContext{
			Now: now.Add(time.Duration(i) * time.Second),
			Cfg: cfg,
			Snapshot: &metrics.SystemSnapshot{
				Processes:    []metrics.ProcessInfo{{PID: 9, Name: "stable.exe"}},
				PortBindings: repeatBindings(9, "stable.exe", 5),
			},
			Alerts: NewAlertStore(8),
		}
		d.Analyze(ctx)
	}
	// Small spike below minBurstConnections (30) — should not alert.
	ctx := &AnalysisContext{
		Now: now.Add(20 * time.Second),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			Processes:    []metrics.ProcessInfo{{PID: 9, Name: "stable.exe"}},
			PortBindings: repeatBindings(9, "stable.exe", 15),
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, d.Name(), 9) != nil {
		t.Fatal("below-minimum burst should not alert")
	}
}

func TestNetworkAnomalyDetectorIgnoresIgnoredProcess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.ConnectionSigma = 1
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 100000
	cfg.Anomaly.IgnoreProcesses = []string{"ignored.exe"}
	d := NewNetworkAnomalyDetector()
	now := time.Now()
	alerts := NewAlertStore(8)
	for i := 0; i < 12; i++ {
		d.Analyze(&AnalysisContext{
			Now: now.Add(time.Duration(i) * time.Second),
			Cfg: cfg,
			Snapshot: &metrics.SystemSnapshot{
				Processes:    []metrics.ProcessInfo{{PID: 11, Name: "ignored.exe"}},
				PortBindings: repeatBindings(11, "ignored.exe", 5),
			},
			Alerts: alerts,
		})
	}
	d.Analyze(&AnalysisContext{
		Now: now.Add(20 * time.Second),
		Cfg: cfg,
		Snapshot: &metrics.SystemSnapshot{
			Processes:    []metrics.ProcessInfo{{PID: 11, Name: "ignored.exe"}},
			PortBindings: repeatBindings(11, "ignored.exe", 200),
		},
		Alerts: alerts,
	})
	if findActiveAlert(alerts, d.Name(), 11) != nil {
		t.Fatal("ignored process should not alert")
	}
}

func TestNetworkAnomalyDetectorGCStalePIDs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 100000
	d := NewNetworkAnomalyDetector()
	now := time.Now()
	alerts := NewAlertStore(8)
	// Add a PID.
	d.Analyze(&AnalysisContext{
		Now: now, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{
			Processes:    []metrics.ProcessInfo{{PID: 22, Name: "x.exe"}},
			PortBindings: repeatBindings(22, "x.exe", 1),
		},
	})
	if _, ok := d.stats[22]; !ok {
		t.Fatal("expected stats entry")
	}
	// Drop the PID — but keep PortBindings non-nil so the detector runs the GC loop.
	d.Analyze(&AnalysisContext{
		Now: now.Add(time.Second), Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{
			PortBindings: []metrics.PortBinding{},
		},
	})
	if _, ok := d.stats[22]; ok {
		t.Fatal("expected stats entry GC'd")
	}
}
