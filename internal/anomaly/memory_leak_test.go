package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestMemoryLeakDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = false
	d := NewMemoryLeakDetector()
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

func TestMemoryLeakDetectorZeroConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.MinGrowthRate = ""
	cfg.Anomaly.MemoryLeak.MemoryThreshold = ""
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p"}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("zero growth/threshold should skip")
	}
}

func TestMemoryLeakDetectorNotEnoughSamples(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	d := NewMemoryLeakDetector()
	st := storage.NewStore(60, 32)
	now := time.Now()
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: now,
		Processes: []metrics.ProcessInfo{{PID: 5, Name: "p", WorkingSet: 3 << 30}},
	})
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 5, Name: "p", WorkingSet: 3 << 30}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("not enough samples should not alert")
	}
}

func TestMemoryLeakDetectorRaises(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 10 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.5
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "100MB"
	d := NewMemoryLeakDetector()
	st := storage.NewStore(120, 64)
	now := time.Now()
	// Plant 8 samples in chronological order (oldest first) showing a clear
	// upward trend so slope is positive and R² is high.
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Minute)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{
				PID: 7, Name: "leaky.exe",
				WorkingSet: uint64((200 + (7-i)*30) << 20), // grows 30MB/min
			}},
		})
	}
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 7, Name: "leaky.exe", WorkingSet: uint64(420 << 20)}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 7); a == nil {
		t.Fatal("expected leak alert")
	}
}

func TestMemoryLeakDetectorIgnoresProcess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 10 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.1
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "1MB"
	cfg.Anomaly.IgnoreProcesses = []string{"leaky.exe"}
	d := NewMemoryLeakDetector()
	st := storage.NewStore(120, 64)
	now := time.Now()
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Minute)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{PID: 7, Name: "leaky.exe", WorkingSet: uint64((100 + (7-i)*30) << 20)}},
		})
	}
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 7, Name: "leaky.exe", WorkingSet: 300 << 20}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if a := findActiveAlert(ctx.Alerts, d.Name(), 7); a != nil {
		t.Fatal("ignored process should not alert")
	}
}

func TestMemoryLeakDetectorSlowGrowthNoAlert(t *testing.T) {
	// Linear data with high R² but growth rate below threshold — exercises the
	// growthPerMin < growthBps skip branch (line 58).
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 10 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "100MB/min" // high bar
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.1
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "1MB"
	st := storage.NewStore(120, 64)
	now := time.Now()
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Minute)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			// Linear growth of only 1MB/min — well below the 100MB/min threshold.
			Processes: []metrics.ProcessInfo{{PID: 4, Name: "slow.exe", WorkingSet: uint64((200 + (7 - i)) << 20)}},
		})
	}
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 4, Name: "slow.exe", WorkingSet: 208 << 20}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, d.Name(), 4) != nil {
		t.Fatal("slow linear growth below threshold should not alert")
	}
}

func TestMemoryLeakDetectorSkipBranches(t *testing.T) {
	// Exercise every skip path that TestMemoryLeakDetectorRaises does not.
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 5 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.95
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "1GB"
	st := storage.NewStore(120, 64)
	now := time.Now()

	// Plant 8 samples in-window that are flat (low R², slow growth).
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Second)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{PID: 1, Name: "flat.exe", WorkingSet: 2 << 30}},
		})
	}
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 1, Name: "flat.exe", WorkingSet: 2 << 30}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, d.Name(), 1) != nil {
		t.Fatal("flat data (low R² / slow growth) should not alert")
	}
}

func TestMemoryLeakDetectorSkipsOldSamples(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 1 * time.Second
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.1
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "1MB"
	st := storage.NewStore(120, 64)
	now := time.Now()
	// Plant samples far outside the 1s window — all filtered out, leaving <5 in-window.
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Hour)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{PID: 2, Name: "old.exe", WorkingSet: uint64((100 + (7-i)*30) << 20)}},
		})
	}
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 2, Name: "old.exe", WorkingSet: 1 << 30}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, d.Name(), 2) != nil {
		t.Fatal("all samples outside window should not alert")
	}
}

func TestMemoryLeakDetectorBelowMemoryThreshold(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.MemoryLeak.Enabled = true
	cfg.Anomaly.MemoryLeak.Window = 10 * time.Minute
	cfg.Anomaly.MemoryLeak.MinGrowthRate = "1MB/min"
	cfg.Anomaly.MemoryLeak.MinRSquared = 0.1
	cfg.Anomaly.MemoryLeak.MemoryThreshold = "10GB" // high threshold
	st := storage.NewStore(120, 64)
	now := time.Now()
	// Growing process but below the 10GB memory threshold.
	for i := 7; i >= 0; i-- {
		t0 := now.Add(time.Duration(-i) * time.Minute)
		st.SetLatest(&metrics.SystemSnapshot{
			Timestamp: t0,
			Processes: []metrics.ProcessInfo{{PID: 3, Name: "small.exe", WorkingSet: uint64((100 + (7-i)*30) << 20)}},
		})
	}
	d := NewMemoryLeakDetector()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Store: st,
		Snapshot: &metrics.SystemSnapshot{
			Timestamp: now,
			Processes: []metrics.ProcessInfo{{PID: 3, Name: "small.exe", WorkingSet: 320 << 20}},
		},
		Alerts: NewAlertStore(8),
	}
	d.Analyze(ctx)
	if findActiveAlert(ctx.Alerts, d.Name(), 3) != nil {
		t.Fatal("below memory threshold should not alert")
	}
}
