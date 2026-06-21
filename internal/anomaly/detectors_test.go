package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func testAnalysisContext(cfg *config.Config, snap *metrics.SystemSnapshot, now time.Time) *AnalysisContext {
	return &AnalysisContext{
		Now:      now,
		Snapshot: snap,
		Store:    storage.NewStore(60, 10),
		Cfg:      cfg,
		Alerts:   NewAlertStore(64),
	}
}

func findActiveAlert(alerts *AlertStore, alertType string, pid uint32) *Alert {
	for _, a := range alerts.Active() {
		if a.Type == alertType && a.PID == pid {
			cp := a
			return &cp
		}
	}
	return nil
}

func TestSpawnStormDetectorRaisesForNonWhitelistedParent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 3

	d := NewSpawnStormDetector()
	now := time.Now()

	baseSnap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 1, Name: "badfork.exe"},
		},
	}
	ctx := testAnalysisContext(cfg, baseSnap, now)
	d.Analyze(ctx)

	nextSnap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 1, Name: "badfork.exe"},
			{PID: 11, ParentPID: 10, Name: "child1.exe"},
			{PID: 12, ParentPID: 10, Name: "child2.exe"},
			{PID: 13, ParentPID: 10, Name: "child3.exe"},
		},
	}
	ctx = testAnalysisContext(cfg, nextSnap, now.Add(10*time.Second))
	d.Analyze(ctx)

	alert := findActiveAlert(ctx.Alerts, d.Name(), 10)
	if alert == nil {
		t.Fatal("expected spawn storm alert")
	}
	if alert.Severity != SeverityCritical {
		t.Fatalf("severity=%s want %s", alert.Severity, SeverityCritical)
	}
}

func TestRunawayCPUDetectorEscalatesAfterDuration(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.RunawayCPU.Enabled = true
	cfg.Anomaly.RunawayCPU.CPUThreshold = 80
	cfg.Anomaly.RunawayCPU.DurationThreshold = 2 * time.Second
	cfg.Anomaly.RunawayCPU.CriticalDuration = 4 * time.Second

	d := NewRunawayCPUDetector()
	alerts := NewAlertStore(64)
	now := time.Now()
	snap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{
			{PID: 20, Name: "heater.exe", CPUPercent: 95},
		},
	}

	ctx := &AnalysisContext{Now: now, Snapshot: snap, Cfg: cfg, Alerts: alerts}
	d.Analyze(ctx)
	if got := len(alerts.Active()); got != 0 {
		t.Fatalf("active alerts=%d want 0 on first sample", got)
	}

	ctx.Now = now.Add(3 * time.Second)
	d.Analyze(ctx)
	alert := findActiveAlert(alerts, d.Name(), 20)
	if alert == nil || alert.Severity != SeverityWarning {
		t.Fatalf("expected warning alert after duration threshold, got %+v", alert)
	}

	ctx.Now = now.Add(5 * time.Second)
	d.Analyze(ctx)
	alert = findActiveAlert(alerts, d.Name(), 20)
	if alert == nil || alert.Severity != SeverityCritical {
		t.Fatalf("expected critical alert after critical duration, got %+v", alert)
	}
}

func TestNetworkAnomalyDetectorRaisesSystemAndBurstAlerts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NetworkAnomaly.Enabled = true
	cfg.Anomaly.NetworkAnomaly.ConnectionSigma = 2
	cfg.Anomaly.NetworkAnomaly.MaxSystemConnections = 40

	d := NewNetworkAnomalyDetector()
	alerts := NewAlertStore(64)
	now := time.Now()

	for i := 0; i < 10; i++ {
		snap := &metrics.SystemSnapshot{
			Processes:    []metrics.ProcessInfo{{PID: 30, Name: "socketbomb.exe"}},
			PortBindings: repeatBindings(30, "socketbomb.exe", 5),
		}
		ctx := &AnalysisContext{Now: now.Add(time.Duration(i) * time.Second), Snapshot: snap, Cfg: cfg, Alerts: alerts}
		d.Analyze(ctx)
	}

	burstSnap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{{PID: 30, Name: "socketbomb.exe"}},
		PortBindings: append(
			repeatBindings(30, "socketbomb.exe", 35),
			repeatBindings(31, "other.exe", 10)...,
		),
	}
	ctx := &AnalysisContext{Now: now.Add(11 * time.Second), Snapshot: burstSnap, Cfg: cfg, Alerts: alerts}
	d.Analyze(ctx)

	if findActiveAlert(alerts, d.Name(), 30) == nil {
		t.Fatal("expected per-process network burst alert")
	}
	if findActiveAlert(alerts, d.Name()+"_system", 0) == nil {
		t.Fatal("expected system network cap alert")
	}
}

func TestNewProcessDetectorFlagsSuspiciousPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = true
	cfg.Anomaly.NewProcess.SuspiciousPaths = []string{`C:\Temp`}

	d := NewNewProcessDetector()
	alerts := NewAlertStore(64)
	now := time.Now()
	snap := &metrics.SystemSnapshot{
		Processes: []metrics.ProcessInfo{
			{PID: 40, Name: "evil.exe", ExePath: `C:\Temp\evil.exe`},
		},
	}
	ctx := &AnalysisContext{Now: now, Snapshot: snap, Cfg: cfg, Alerts: alerts}
	d.Analyze(ctx)

	alert := findActiveAlert(alerts, d.Name(), 40)
	if alert == nil {
		t.Fatal("expected suspicious path alert")
	}
	if alert.Severity != SeverityInfo {
		t.Fatalf("severity=%s want %s", alert.Severity, SeverityInfo)
	}
}

func repeatBindings(pid uint32, proc string, n int) []metrics.PortBinding {
	out := make([]metrics.PortBinding, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, metrics.PortBinding{
			PID:       pid,
			Process:   proc,
			Protocol:  "tcp",
			LocalPort: uint16(10000 + i),
		})
	}
	return out
}

// TestMemoryLeakDetectorLowRSquaredSkip exercises the
// `if r2 < cfg.MinRSquared { continue }` branch in memory_leak.go. To force
// low R² we plant 8 samples whose working set alternates between two values
// (100MB / 800MB) instead of trending linearly — the resulting regression
// line is poor, so r2 < MinRSquared regardless of growth rate.
func TestMemoryLeakDetectorLowRSquaredSkip(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 10 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.9 // require strong linearity
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "1MB"
	st := storage.NewStore(120, 64)
	now := time.Now()
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Minute)
		// Alternate between two very different values so the line fit is poor.
		var ws uint64
		if i%2 == 0 {
			ws = 100 << 20
		} else {
			ws = 800 << 20
		}
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{PID: 9, Name: "noisy.exe", WorkingSet: ws}},
		})
	}
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 9, Name: "noisy.exe", WorkingSet: 500 << 20}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 9); a != nil {
		t.Fatal("low R² data should not raise an alert (r2 < MinRSquared branch)")
	}
}
