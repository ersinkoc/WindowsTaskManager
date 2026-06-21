//go:build windows

package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/platform"
)

// allDetectorTypes is the complete set of alert types clearDisabledDetectorAlerts
// knows how to purge. Order matters for the "all disabled" sweep below.
var allDetectorTypes = []string{
	"hung_process",
	"orphan",
	"port_conflict",
	"network_anomaly",
	"network_anomaly_system",
	"spawn_storm",
	"memory_leak",
	"runaway_cpu",
	"new_process",
}

// seedActive raises one alert for every type in allDetectorTypes (using a
// distinct PID so they land in separate keys) and asserts they are present.
func seedActive(t *testing.T, alerts *anomaly.AlertStore) {
	t.Helper()
	for i, typ := range allDetectorTypes {
		_, added := alerts.Raise(anomaly.Alert{
			Type:        typ,
			PID:         uint32(1000 + i),
			Severity:    anomaly.SeverityWarning,
			Title:       "seed",
			Description: "seed",
		})
		if !added {
			t.Fatalf("Raise(%s) was not added to active set", typ)
		}
	}
}

func TestClearDisabledDetectorAlerts_AllEnabled(t *testing.T) {
	// When every detector is enabled, nothing should be purged.
	cfg := config.DefaultConfig()
	a := cfg.Anomaly
	a.HungProcess.Enabled = true
	a.Orphan.Enabled = true
	a.PortConflict.Enabled = true
	a.NetworkAnomaly.Enabled = true
	a.SpawnStorm.Enabled = true
	a.MemoryLeak.Enabled = true
	a.RunawayCPU.Enabled = true
	a.NewProcess.Enabled = true
	cfg.Anomaly = a

	alerts := anomaly.NewAlertStore(256)
	seedActive(t, alerts)

	clearDisabledDetectorAlerts(alerts, cfg)

	if got := len(alerts.Active()); got != len(allDetectorTypes) {
		t.Fatalf("expected %d active alerts after clear, got %d", len(allDetectorTypes), got)
	}
}

func TestClearDisabledDetectorAlerts_AllDisabled(t *testing.T) {
	// When every detector is disabled, every seeded alert must be cleared.
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = false
	cfg.Anomaly.Orphan.Enabled = false
	cfg.Anomaly.PortConflict.Enabled = false
	cfg.Anomaly.NetworkAnomaly.Enabled = false
	cfg.Anomaly.SpawnStorm.Enabled = false
	cfg.Anomaly.MemoryLeak.Enabled = false
	cfg.Anomaly.RunawayCPU.Enabled = false
	cfg.Anomaly.NewProcess.Enabled = false

	alerts := anomaly.NewAlertStore(256)
	seedActive(t, alerts)

	clearDisabledDetectorAlerts(alerts, cfg)

	if got := len(alerts.Active()); got != 0 {
		t.Fatalf("expected 0 active alerts after clearing all detectors, got %d", got)
	}
}

// allButOneDisabled returns a config where only `keepType`'s detector is
// enabled; every other detector is turned off. It also returns the list of
// alert types that should survive the clear (the ones owned by the enabled
// detector — network owns two).
func allButOneDisabled(keepType string) (*config.Config, []string) {
	cfg := config.DefaultConfig()
	a := cfg.Anomaly
	a.HungProcess.Enabled = false
	a.Orphan.Enabled = false
	a.PortConflict.Enabled = false
	a.NetworkAnomaly.Enabled = false
	a.SpawnStorm.Enabled = false
	a.MemoryLeak.Enabled = false
	a.RunawayCPU.Enabled = false
	a.NewProcess.Enabled = false

	var survivors []string
	switch keepType {
	case "hung_process":
		a.HungProcess.Enabled = true
		survivors = []string{"hung_process"}
	case "orphan":
		a.Orphan.Enabled = true
		survivors = []string{"orphan"}
	case "port_conflict":
		a.PortConflict.Enabled = true
		survivors = []string{"port_conflict"}
	case "network_anomaly":
		a.NetworkAnomaly.Enabled = true
		survivors = []string{"network_anomaly", "network_anomaly_system"}
	case "spawn_storm":
		a.SpawnStorm.Enabled = true
		survivors = []string{"spawn_storm"}
	case "memory_leak":
		a.MemoryLeak.Enabled = true
		survivors = []string{"memory_leak"}
	case "runaway_cpu":
		a.RunawayCPU.Enabled = true
		survivors = []string{"runaway_cpu"}
	case "new_process":
		a.NewProcess.Enabled = true
		survivors = []string{"new_process"}
	}
	cfg.Anomaly = a
	return cfg, survivors
}

func TestClearDisabledDetectorAlerts_PerDetector(t *testing.T) {
	// For each detector: disable everything else, enable only it, and verify
	// exactly that detector's alert types survive while the rest are purged.
	cases := []string{
		"hung_process",
		"orphan",
		"port_conflict",
		"network_anomaly",
		"spawn_storm",
		"memory_leak",
		"runaway_cpu",
		"new_process",
	}
	for _, keep := range cases {
		t.Run(keep, func(t *testing.T) {
			cfg, survivors := allButOneDisabled(keep)
			alerts := anomaly.NewAlertStore(256)
			seedActive(t, alerts)

			clearDisabledDetectorAlerts(alerts, cfg)

			active := alerts.Active()
			if len(active) != len(survivors) {
				t.Fatalf("expected %d survivors, got %d (%v)", len(survivors), len(active), typeNames(active))
			}
			got := make(map[string]bool, len(active))
			for _, al := range active {
				got[al.Type] = true
			}
			for _, want := range survivors {
				if !got[want] {
					t.Fatalf("expected survivor %q to remain, active set = %v", want, typeNames(active))
				}
			}
		})
	}
}

func typeNames(as []anomaly.Alert) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Type)
	}
	return out
}

func TestClearDisabledDetectorAlerts_EmptyStore(t *testing.T) {
	// Clearing an empty store with all detectors disabled must not panic and
	// must leave the store empty.
	cfg := config.DefaultConfig()
	cfg.Anomaly.HungProcess.Enabled = false
	cfg.Anomaly.Orphan.Enabled = false
	cfg.Anomaly.PortConflict.Enabled = false
	cfg.Anomaly.NetworkAnomaly.Enabled = false
	cfg.Anomaly.SpawnStorm.Enabled = false
	cfg.Anomaly.MemoryLeak.Enabled = false
	cfg.Anomaly.RunawayCPU.Enabled = false
	cfg.Anomaly.NewProcess.Enabled = false

	alerts := anomaly.NewAlertStore(256)
	clearDisabledDetectorAlerts(alerts, cfg)
	if got := len(alerts.Active()); got != 0 {
		t.Fatalf("expected 0 active alerts, got %d", got)
	}
}

func TestResolveConfigPath_FlagWins(t *testing.T) {
	// An explicit flag value must always be returned verbatim, overriding the
	// exe-local and APPDATA fallbacks.
	want := "C:/custom/path/to/config.yaml"
	if got := resolveConfigPath(want); got != want {
		t.Fatalf("flag value not returned verbatim: got %q want %q", got, want)
	}
}

func TestResolveConfigPath_LocalConfigNextToExe(t *testing.T) {
	// Drop a config.yaml into the test binary's directory and verify it is
	// preferred over the APPDATA fallback. With a non-empty flag this path is
	// never reached, so pass "".
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	exeDir := filepath.Dir(exe)
	local := filepath.Join(exeDir, "config.yaml")
	if err := os.WriteFile(local, []byte("schema_version: 2\n"), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(local) })

	got := resolveConfigPath("")
	want := local
	if !strings.EqualFold(got, want) {
		t.Fatalf("local config next to exe not resolved: got %q want %q", got, want)
	}
}

func TestResolveConfigPath_AppDataFallback(t *testing.T) {
	// With no flag and no exe-local config, resolveConfigPath must fall back to
	// %APPDATA%\WindowsTaskManager\config.yaml. Use a controlled APPDATA dir so
	// the test does not depend on the real user profile.
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)

	// Ensure there is no config.yaml next to the test binary, otherwise that
	// branch would win over APPDATA.
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	local := filepath.Join(filepath.Dir(exe), "config.yaml")
	if _, err := os.Stat(local); err == nil {
		t.Skipf("exe-local config.yaml present (%s); cannot deterministically test APPDATA fallback", local)
	}

	got := resolveConfigPath("")
	want := filepath.Join(tmp, "WindowsTaskManager", "config.yaml")
	if !strings.EqualFold(got, want) {
		t.Fatalf("APPDATA fallback wrong: got %q want %q", got, want)
	}
}

func TestResolveConfigPath_EmptyAppData(t *testing.T) {
	// When APPDATA is unset, resolveConfigPath falls back to a relative path
	// rooted at ".". Ensure the local-exe branch does not win first.
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	local := filepath.Join(filepath.Dir(exe), "config.yaml")
	if _, err := os.Stat(local); err == nil {
		t.Skipf("exe-local config.yaml present (%s); cannot deterministically test empty APPDATA fallback", local)
	}

	t.Setenv("APPDATA", "")
	got := resolveConfigPath("")
	want := filepath.Join(".", "WindowsTaskManager", "config.yaml")
	if !strings.EqualFold(got, want) {
		t.Fatalf("empty APPDATA fallback wrong: got %q want %q", got, want)
	}
}

// TestMain_VersionFlag drives the real main() through the --version path.
// main() uses the global flag.CommandLine and flag.Parse(), and exits the
// version branch with a plain `return`, so we save/restore both os.Args and
// flag.CommandLine around the call and capture stdout via a temp file.
func TestMain_VersionFlag(t *testing.T) {
	savedArgs := os.Args
	savedCommandLine := flag.CommandLine
	t.Cleanup(func() {
		os.Args = savedArgs
		flag.CommandLine = savedCommandLine
	})

	// A fresh FlagSet is required: flag.Parse() short-circuits once it has
	// parsed, so re-parsing with different args against the same CommandLine
	// (now already-parsed from a prior run) would silently no-op.
	flag.CommandLine = flag.NewFlagSet("wtm-test", flag.ContinueOnError)
	os.Args = []string{"wtm", "--version"}

	// main() prints to os.Stdout; redirect it to a pipe we can read back.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	savedStdout := os.Stdout
	os.Stdout = w

	// Run main() in a goroutine: it writes the version line to the pipe and
	// returns immediately (the --version branch never touches the network,
	// single-instance guard, or desktop window).
	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
		_ = w.Close() // signal EOF to the reader
	}()

	// Drain the pipe from the main goroutine; Read returns EOF once w is closed.
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	<-done
	os.Stdout = savedStdout

	out := string(buf)
	if !strings.Contains(out, "Windows Task Manager") {
		t.Fatalf("--version output missing app name: %q", out)
	}
	if !strings.Contains(out, version) {
		t.Fatalf("--version output missing version %q: %q", version, out)
	}
}

// TestMain_AlreadyRunning drives the real main() through the single-instance
// guard rejection branch. We reserve the named mutex from the test itself so
// that the AcquireSingleInstance call inside main() returns ErrAlreadyRunning.
// With --no-browser we also avoid spawning the default browser via
// ShellExecute, which keeps the test hermetic. --no-tray is harmless and
// matches the rest of the headless path. main() should return normally (no
// HTTP listener, no panic) and log the "already running" message to stderr.
func TestMain_AlreadyRunning(t *testing.T) {
	// Reserve the singleton mutex first so the next AcquireSingleInstance
	// call (inside main()) sees it as already held.
	hold, err := platform.AcquireSingleInstance(`Local\WindowsTaskManager.Singleton`)
	if err != nil {
		// If the user's machine already has wtm running, we cannot reserve
		// the mutex. Skip rather than fail — the production "another
		// instance is already running" branch is exactly what we wanted to
		// exercise, and it is being exercised by that real running process.
		t.Skipf("singleton already held on this host (real wtm running?): %v", err)
	}
	t.Cleanup(hold)

	savedArgs := os.Args
	savedCommandLine := flag.CommandLine
	savedStderr := os.Stderr
	savedStdout := os.Stdout
	t.Cleanup(func() {
		os.Args = savedArgs
		flag.CommandLine = savedCommandLine
		os.Stderr = savedStderr
		os.Stdout = savedStdout
	})

	// Fresh FlagSet so flag.Parse() actually runs against our new args
	// instead of no-op'ing because the previous test already parsed.
	flag.CommandLine = flag.NewFlagSet("wtm-test", flag.ContinueOnError)
	flag.CommandLine.SetOutput(os.Stderr)

	// Point --config at a temp path so config.Load succeeds (it auto-creates
	// a default config when the file does not exist). A temp dir also keeps
	// this test from touching the user's real APPDATA config.
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	os.Args = []string{"wtm", "--config", cfgPath, "--no-browser", "--no-tray"}

	// Capture stderr so we can assert the log output and silence it from
	// the test runner's view; stdout stays clean because the already-running
	// branch writes only via log.Printf (which goes to stderr).
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	os.Stdout = w

	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
		_ = w.Close()
	}()

	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	<-done

	out := string(buf)
	if !strings.Contains(out, "already running") {
		t.Fatalf("expected 'already running' log line, got: %q", out)
	}
	if !strings.Contains(out, "loading config from "+cfgPath) {
		t.Fatalf("expected config-load log line for %s, got: %q", cfgPath, out)
	}
	// The --version branch must NOT have fired: it writes "Windows Task Manager"
	// to stdout. Already-running path returns without printing the banner.
	if strings.Contains(out, "Windows Task Manager") {
		t.Fatalf("--version banner leaked into already-running path: %q", out)
	}
}
