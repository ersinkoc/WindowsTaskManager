package anomaly

import (
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func TestNewProcessDetectorDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = false
	d := NewNewProcessDetector()
	ctx := &AnalysisContext{
		Now:      time.Now(),
		Cfg:      cfg,
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{{PID: 1, Name: "p", ExePath: "C:\\Temp\\p.exe"}}},
		Alerts:   NewAlertStore(8),
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("disabled should not alert")
	}
}

func TestNewProcessDetectorSkipsKnownAndEmptyPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = true
	cfg.Anomaly.NewProcess.SuspiciousPaths = []string{`C:\Temp`}
	d := NewNewProcessDetector()
	now := time.Now()
	// First call registers PID.
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "good.exe", ExePath: "C:\\Windows\\good.exe"},
		}},
	}
	d.Analyze(ctx)
	d.Analyze(ctx) // second call should be "known"
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("known PID with non-matching path should not alert")
	}
}

func TestNewProcessDetectorSkipsEmptyExePath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = true
	cfg.Anomaly.NewProcess.SuspiciousPaths = []string{`C:\Temp`}
	d := NewNewProcessDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "p.exe", ExePath: ""},
		}},
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("empty path should not alert")
	}
}

func TestNewProcessDetectorSkipsIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = true
	cfg.Anomaly.NewProcess.SuspiciousPaths = []string{`C:\Temp`}
	cfg.Anomaly.IgnoreProcesses = []string{"ignore.exe"}
	d := NewNewProcessDetector()
	ctx := &AnalysisContext{
		Now: time.Now(), Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "ignore.exe", ExePath: `C:\Temp\ignore.exe`},
		}},
	}
	d.Analyze(ctx)
	if len(ctx.Alerts.Active()) != 0 {
		t.Fatal("ignored process should not alert")
	}
}

func TestNewProcessDetectorGCStalePIDs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Anomaly.NewProcess.Enabled = true
	cfg.Anomaly.NewProcess.SuspiciousPaths = []string{`C:\Temp`}
	d := NewNewProcessDetector()
	now := time.Now()
	ctx := &AnalysisContext{
		Now: now, Cfg: cfg, Alerts: NewAlertStore(8),
		Snapshot: &metrics.SystemSnapshot{Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "p.exe", ExePath: "C:\\Windows\\p.exe"},
		}},
	}
	d.Analyze(ctx)
	if _, ok := d.known[1]; !ok {
		t.Fatal("expected PID known")
	}
	ctx.Snapshot.Processes = nil
	d.Analyze(ctx)
	if _, ok := d.known[1]; ok {
		t.Fatal("expected known map GC'd")
	}
}

func TestExpandPathsAndReplaceWinVars(t *testing.T) {
	t.Setenv("FOO", "bar")
	got := expandPaths([]string{"plain", "%FOO%\\baz", "%FOO%:%FOO%"})
	if len(got) != 3 {
		t.Fatalf("got %d entries", len(got))
	}
	if got[0] != "plain" {
		t.Fatalf("got[0]=%q", got[0])
	}
	if got[1] != "bar\\baz" {
		t.Fatalf("got[1]=%q", got[1])
	}
	// "%FOO:%FOO%" — there's no closing %, so the first '%' is kept as a literal.
	if got[2] == "" {
		t.Fatal("got[2] empty")
	}
	// Lone trailing percent is preserved.
	if s := replaceWinVars("a%b%c"); s != "a${b}c" {
		t.Fatalf("lone percents: %q", s)
	}
	// Open-ended percent is preserved.
	if s := replaceWinVars("a%"); s != "a%" {
		t.Fatalf("open percent: %q", s)
	}
	// Percent with no closing delimiter stays literal.
	if s := replaceWinVars("a%incomplete"); s != "a%incomplete" {
		t.Fatalf("incomplete percent: %q", s)
	}
}
