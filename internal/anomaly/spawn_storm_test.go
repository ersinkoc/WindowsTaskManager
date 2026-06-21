package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestSpawnStormDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = false
	d := NewSpawnStormDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, ParentPID: 1, Name: "p"}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled should not alert")
	}
}

func TestSpawnStormDetectorSkipsParentPIDZero(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 1
	d := NewSpawnStormDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 0, Name: "orphan.exe"}, // no parent -> not counted
		}},
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("orphan parent should not count")
	}
}

func TestSpawnStormDetectorSkipsKnownAndIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 1
	cfg.Anomaly.IgnoreProcesses = []string{"ignore.exe"}
	d := NewSpawnStormDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 100, Name: "ignore.exe"},
		}},
	}
	d.Analyze(ctx)
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("ignored parent should not alert")
	}
}

func TestSpawnStormDetectorParentNotFoundInSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 1
	d := NewSpawnStormDetector()
	now := time.Now()
	// First sample — known PID recorded.
	d.Analyze(&AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 1, Name: "c.exe"},
		}},
	})
	// Second sample adds another child; parent 1 is NOT in processes.
	d.Analyze(&AnalysisContext{
		Now: now.Add(time.Second), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 1, Name: "c.exe"},
			{PID: 11, ParentPID: 1, Name: "c2.exe"},
		}},
	})
	// Parent isn't in the snapshot, so findProcess returns nil -> no alert.
	if len(d.parentEvents) == 0 {
		t.Fatal("expected parentEvents to have entries")
	}
}

func TestSpawnStormDetectorSkipsWhitelistedParent(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 1
	d := NewSpawnStormDetector()
	now := time.Now()
	alerts := NewAlertStore(8)
	// Children of chrome.exe -> parent is whitelisted, so no alert.
	d.Analyze(&AnalysisContext{
		Now: now, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 100, ParentPID: 0, Name: "chrome.exe"},
			{PID: 1, ParentPID: 100, Name: "child.exe"},
		}},
	})
	d.Analyze(&AnalysisContext{
		Now: now.Add(time.Second), Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 100, ParentPID: 0, Name: "chrome.exe"},
			{PID: 1, ParentPID: 100, Name: "child.exe"},
			{PID: 2, ParentPID: 100, Name: "child2.exe"},
		}},
	})
	// Events are recorded (whitelist only suppresses the alert, not tracking).
	if len(d.parentEvents[100]) == 0 {
		t.Fatal("expected events recorded for whitelisted parent")
	}
	if findActiveAlert(alerts, d.Name(), 100) != nil {
		t.Fatal("whitelisted parent should not raise alert")
	}
}

func TestIsSpawnStormWhitelistedAllBranches(t *testing.T) {
	cases := []string{"chrome.exe", "CHROME.EXE", "notlisted.exe"}
	if !isSpawnStormWhitelisted(cases[0]) {
		t.Fatal("chrome should be whitelisted")
	}
	if !isSpawnStormWhitelisted(cases[1]) {
		t.Fatal("case insensitive")
	}
	if isSpawnStormWhitelisted(cases[2]) {
		t.Fatal("notlisted should not be whitelisted")
	}
}

func TestFindProcess(t *testing.T) {
	list := []metrics.ProcessInfo{
		{PID: 1, Name: "a"},
		{PID: 2, Name: "b"},
	}
	if p := findProcess(list, 1); p == nil || p.Name != "a" {
		t.Fatalf("findProcess(1) = %+v", p)
	}
	if p := findProcess(list, 999); p != nil {
		t.Fatalf("findProcess(999) should return nil")
	}
}

func TestSpawnStormDetectorGCStalePIDs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	d := NewSpawnStormDetector()
	now := time.Now()
	d.Analyze(&AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 0, Name: "p.exe"},
		}},
	})
	if _, ok := d.knownPIDs[1]; !ok {
		t.Fatal("expected PID known")
	}
	d.Analyze(&AnalysisContext{
		Now: now.Add(time.Second), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{},
	})
	if _, ok := d.knownPIDs[1]; ok {
		t.Fatal("expected PID GC'd")
	}
}

func TestSpawnStormDetectorIgnoresParentAtThreshold(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 2
	cfg.Anomaly.IgnoreProcesses = []string{"badparent.exe"}
	d := NewSpawnStormDetector()
	now := time.Now()
	alerts := NewAlertStore(8)
	// Spawn 2 children from an ignored parent — exceeds threshold but parent is ignored.
	d.Analyze(&AnalysisContext{
		Now: now, Cfg: cfg, Alerts: alerts,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 50, ParentPID: 0, Name: "badparent.exe"},
			{PID: 1, ParentPID: 50, Name: "c1.exe"},
			{PID: 2, ParentPID: 50, Name: "c2.exe"},
		}},
	})
	if findActiveAlert(alerts, d.Name(), 50) != nil {
		t.Fatal("ignored parent at threshold should not alert")
	}
}

func TestSpawnStormDetectorDropsStaleEvents(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.SpawnStorm.Enabled = true
	cfg.Anomaly.SpawnStorm.MaxChildrenPerMinute = 100
	d := NewSpawnStormDetector()
	t0 := time.Now()
	// First: record 3 children.
	d.Analyze(&AnalysisContext{
		Now: t0, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 0, Name: "p.exe"},
			{PID: 1, ParentPID: 10, Name: "c.exe"},
			{PID: 2, ParentPID: 10, Name: "c.exe"},
			{PID: 3, ParentPID: 10, Name: "c.exe"},
		}},
	})
	if got := len(d.parentEvents[10]); got != 3 {
		t.Fatalf("events=%d want 3", got)
	}
	// Move past 1 minute -> all events trimmed.
	d.Analyze(&AnalysisContext{
		Now: t0.Add(2 * time.Minute), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 10, ParentPID: 0, Name: "p.exe"},
		}},
	})
	if _, ok := d.parentEvents[10]; ok {
		t.Fatal("expected stale parentEvents to be dropped")
	}
}
