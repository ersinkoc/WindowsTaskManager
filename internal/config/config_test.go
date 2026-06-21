package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// overwrite writes raw bytes to a path, replacing any existing file.
func overwrite(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// writeDirAsFile creates a directory at path, used to force ReadFile to fail
// with something other than ENOENT (so Load's "file missing" branch is not taken).
func writeDirAsFile(path string) error {
	return os.MkdirAll(path, 0o700)
}

func TestDefaultTelegramNotificationTypes(t *testing.T) {
	got := DefaultTelegramNotificationTypes()
	want := []string{
		"runaway_cpu",
		"memory_leak",
		"port_conflict",
		"new_process",
		"network_anomaly",
		"network_anomaly_system",
		"rule:*",
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestDefaultConfig_IsValidAndComplete(t *testing.T) {
	c := DefaultConfig()

	if c.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion=%d want %d", c.SchemaVersion, CurrentSchemaVersion)
	}
	if c.Server.Host == "" || c.Server.Port == 0 {
		t.Errorf("Server zero-value: %+v", c.Server)
	}
	if c.Monitoring.Interval == 0 || c.Monitoring.MaxProcesses == 0 {
		t.Errorf("Monitoring zero-value: %+v", c.Monitoring)
	}
	if len(c.Anomaly.IgnoreProcesses) == 0 {
		t.Error("expected default IgnoreProcesses to be populated")
	}
	if len(c.Telegram.NotificationTypes) == 0 {
		t.Error("expected default NotificationTypes to be populated")
	}
	if len(c.Rules) != 0 {
		t.Errorf("expected default Rules to be empty, got %d", len(c.Rules))
	}
	if len(c.AI.AutoAction.AllowedActions) == 0 {
		t.Error("expected default AllowedActions to be populated")
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate(default) failed: %v", err)
	}
}

func TestValidate_PortOutOfRange(t *testing.T) {
	c := DefaultConfig()
	c.Server.Port = 0
	if err := c.Validate(); err == nil {
		t.Error("expected error for port=0")
	}
	c.Server.Port = 70000
	if err := c.Validate(); err == nil {
		t.Error("expected error for port=70000")
	}
	// boundary: 1 and 65535 should pass
	c.Server.Port = 1
	if err := c.Validate(); err != nil {
		t.Errorf("port=1 should be valid: %v", err)
	}
	c.Server.Port = 65535
	if err := c.Validate(); err != nil {
		t.Errorf("port=65535 should be valid: %v", err)
	}
}

func TestValidate_MonitoringIntervalTooSmall(t *testing.T) {
	c := DefaultConfig()
	c.Monitoring.Interval = 50 * time.Millisecond
	if err := c.Validate(); err == nil {
		t.Error("expected error for monitoring.interval < 100ms")
	}
}

func TestValidate_MonitoringClampsAndAITweaks(t *testing.T) {
	c := DefaultConfig()
	c.Monitoring.MaxProcesses = 10
	c.UI.SparklinePoints = 0
	c.AI.MaxTokens = 10
	c.AI.MaxRequestsPerMinute = 0
	c.AI.Scheduler.MinInterval = time.Second
	c.AI.Scheduler.MaxCyclesPerHour = 0
	c.AI.Scheduler.MaxReservedTokensPerDay = 1 // < MaxTokens
	c.AI.Scheduler.CooldownAfterError = time.Second
	c.AI.Scheduler.HistoryLimit = 0
	c.AI.AutoAction.AllowedActions = nil
	c.AI.AutoAction.RequireRepeatCycles = 0
	c.AI.AutoAction.CooldownPerPID = -time.Second

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate returned err: %v", err)
	}
	if c.Monitoring.MaxProcesses != 100 {
		t.Errorf("MaxProcesses=%d want 100", c.Monitoring.MaxProcesses)
	}
	if c.UI.SparklinePoints != 10 {
		t.Errorf("SparklinePoints=%d want 10", c.UI.SparklinePoints)
	}
	if c.AI.MaxTokens != 64 {
		t.Errorf("MaxTokens=%d want 64", c.AI.MaxTokens)
	}
	if c.AI.MaxRequestsPerMinute != 1 {
		t.Errorf("MaxRequestsPerMinute=%d want 1", c.AI.MaxRequestsPerMinute)
	}
	if c.AI.Scheduler.MinInterval != 15*time.Second {
		t.Errorf("MinInterval=%v want 15s", c.AI.Scheduler.MinInterval)
	}
	if c.AI.Scheduler.MaxCyclesPerHour != 1 {
		t.Errorf("MaxCyclesPerHour=%d want 1", c.AI.Scheduler.MaxCyclesPerHour)
	}
	if c.AI.Scheduler.MaxReservedTokensPerDay != c.AI.MaxTokens {
		t.Errorf("MaxReservedTokensPerDay=%d want %d", c.AI.Scheduler.MaxReservedTokensPerDay, c.AI.MaxTokens)
	}
	if c.AI.Scheduler.CooldownAfterError != 30*time.Second {
		t.Errorf("CooldownAfterError=%v want 30s", c.AI.Scheduler.CooldownAfterError)
	}
	if c.AI.Scheduler.HistoryLimit != 1 {
		t.Errorf("HistoryLimit=%d want 1", c.AI.Scheduler.HistoryLimit)
	}
	if len(c.AI.AutoAction.AllowedActions) != 3 {
		t.Errorf("AllowedActions len=%d want 3", len(c.AI.AutoAction.AllowedActions))
	}
	if c.AI.AutoAction.RequireRepeatCycles != 1 {
		t.Errorf("RequireRepeatCycles=%d want 1", c.AI.AutoAction.RequireRepeatCycles)
	}
	if c.AI.AutoAction.CooldownPerPID != 0 {
		t.Errorf("CooldownPerPID=%v want 0", c.AI.AutoAction.CooldownPerPID)
	}
}

func TestValidate_TelegramBranches(t *testing.T) {
	cases := []struct {
		name        string
		mode        string
		wantMode    string
		wantDefault bool // whether validator should rewrite to default
	}{
		{"empty", "", "high_value", true},
		{"high_value", "high_value", "high_value", false},
		{"all_critical", "all_critical", "all_critical", false},
		{"garbage", "weird", "high_value", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultConfig()
			c.Telegram.NotificationMode = tc.mode
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if c.Telegram.NotificationMode != tc.wantMode {
				t.Errorf("mode=%q want %q", c.Telegram.NotificationMode, tc.wantMode)
			}
		})
	}
}

func TestValidate_TelegramFillsDefaults(t *testing.T) {
	c := DefaultConfig()
	c.Telegram.APIBaseURL = ""
	c.Telegram.PollTimeout = 0
	c.Telegram.NotificationTypes = nil
	c.Telegram.ConfirmTTL = 0

	if err := c.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if c.Telegram.APIBaseURL != "https://api.telegram.org" {
		t.Errorf("APIBaseURL=%q", c.Telegram.APIBaseURL)
	}
	if c.Telegram.PollTimeout != 5*time.Second {
		t.Errorf("PollTimeout=%v", c.Telegram.PollTimeout)
	}
	if len(c.Telegram.NotificationTypes) == 0 {
		t.Error("NotificationTypes should be filled with defaults")
	}
	if c.Telegram.ConfirmTTL != 15*time.Second {
		t.Errorf("ConfirmTTL=%v", c.Telegram.ConfirmTTL)
	}
}

func TestMergeUniqueFold(t *testing.T) {
	got := mergeUniqueFold(
		[]string{"a", "B", "  c  ", "", "b"},
		[]string{"B", "d", "A", "", "  e  "},
	)
	want := []string{"a", "B", "  c  ", "d", "  e  "}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]=%q want %q", i, got[i], want[i])
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", 0},
		{"   ", 0},
		{"/MIN", 0}, // strip /MIN -> "" -> 0
		{"10MB/min", 10 << 20},
		{"2GB", 2 << 30},
		{"512KB", 512 << 10},
		{"1TB", 1 << 40},
		{"100B", 100},
		{"2.5MB", uint64(2.5 * float64(1<<20))},
		{"garbage", 0},
		{"1234", 1234},
		{"  3GB  ", 3 << 30},
		{"4gb", 4 << 30}, // case-insensitive
		{"MB", 0},        // suffix-only with no number -> ParseFloat fails -> 0
		{"abcMB", 0},     // non-numeric before MB -> ParseFloat fails -> 0
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := ParseSize(tc.in); got != tc.want {
				t.Errorf("ParseSize(%q)=%d want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoad_CreateWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "wtm.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil cfg")
	}
	if cfg.Server.Port == 0 {
		t.Error("expected non-zero port from default config")
	}
}

func TestLoad_ReadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	// Create a directory where a file is expected so ReadFile fails with non-ENOENT error.
	if err := writeDirAsFile(path); err != nil {
		t.Skipf("cannot create dir-as-file on this platform: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error reading dir-as-file")
	}
}

func TestLoad_BadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	if err := overwrite(path, []byte("not: valid: yaml: :::\n  - [\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected YAML parse error")
	}
}

func TestLoad_ValidatesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	cfg := DefaultConfig()
	cfg.Server.Port = 0 // invalid — should trip Validate
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoad_SchemaMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	// Persist a "legacy" config (SchemaVersion 0, with a user ignore entry).
	legacy := DefaultConfig()
	legacy.SchemaVersion = 0
	legacy.Anomaly.IgnoreProcesses = []string{"CustomProc.exe"}
	if err := Save(path, legacy); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion=%d want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}
	found := false
	for _, p := range cfg.Anomaly.IgnoreProcesses {
		if p == "CustomProc.exe" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected preserved CustomProc.exe in IgnoreProcesses, got %v", cfg.Anomaly.IgnoreProcesses)
	}
}

// TestLoad_CreateDefaultSaveFailure forces Load's "file missing" branch to
// fail at the Save call by asking it to create a default config inside a path
// whose parent component is a regular file (not a directory), so MkdirAll fails.
func TestLoad_CreateDefaultSaveFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	// path is dir/blocker/wtm.yaml — blocker is a file so MkdirAll("dir/blocker") fails.
	path := filepath.Join(blocker, "wtm.yaml")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error creating default config under file-as-dir")
	}
}

// TestLoad_SchemaMigrationSaveFailure forces Load's schema-migration branch
// to fail at the Save call by holding an exclusive handle on path.tmp so
// Save's WriteFile fails when it tries to create the temp file.
func TestLoad_SchemaMigrationSaveFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	legacy := DefaultConfig()
	legacy.SchemaVersion = 0
	legacy.Anomaly.IgnoreProcesses = []string{"CustomProc.exe"}
	if err := Save(path, legacy); err != nil {
		t.Fatal(err)
	}
	// Hold an exclusive lock on the .tmp path so Save cannot write it.
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Load(path); err == nil {
		t.Fatal("expected error persisting migrated config")
	}
}

func TestSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "wtm.yaml")
	cfg := DefaultConfig()
	cfg.Server.Port = 31337
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Server.Port != 31337 {
		t.Errorf("Port=%d want 31337", loaded.Server.Port)
	}
}

// TestSave_MkdirAllError forces Save's MkdirAll branch to fail by passing
// a path whose parent component is an existing regular file.
func TestSave_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err == nil {
		t.Fatal("expected Save to fail when parent is a file")
	}
}

// TestSave_WriteFileError forces Save's WriteFile branch to fail by replacing
// the temp-path with a directory: WriteFile on a path that is a directory
// fails on Windows with access-denied.
func TestSave_WriteFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	// Pre-create path + ".tmp" as a directory so WriteFile to it fails.
	tmpPath := path + ".tmp"
	if err := os.MkdirAll(tmpPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, DefaultConfig()); err == nil {
		t.Fatal("expected Save to fail when .tmp is a directory")
	}
}

// TestSave_RenameError forces Save's rename branch to fail by holding an
// exclusive handle on the destination path. On Windows, renaming over an
// open file with FILE_SHARE_NONE fails. This also exercises the
// os.Remove(tmpPath) cleanup that runs when Rename fails.
func TestSave_RenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	// Lock the destination so the next Rename fails.
	dst, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err := Save(path, DefaultConfig()); err == nil {
		t.Fatal("expected Save to fail when destination is locked")
	}
	// After failed Rename the .tmp file should be cleaned up.
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("expected .tmp to be removed after failed Rename")
	}
}

// TestSave_NoPreservedIgnore covers the "no preserved ignore list" branch
// inside Load's schema-migration code path. When SchemaVersion<Current and
// IgnoreProcesses is empty, the merge call is skipped and defaults win.
func TestLoad_SchemaMigration_NoPreservedIgnore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	legacy := DefaultConfig()
	legacy.SchemaVersion = 0
	legacy.Anomaly.IgnoreProcesses = nil
	if err := Save(path, legacy); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion=%d want %d", cfg.SchemaVersion, CurrentSchemaVersion)
	}
	if len(cfg.Anomaly.IgnoreProcesses) == 0 {
		t.Error("expected default IgnoreProcesses after migration")
	}
}

// TestSave_MarshalError exercises the yaml.Marshal error branch in Save by
// swapping the package-level yamlMarshal function with a stub that returns
// an error. yaml.Marshal never fails on a well-formed *Config so this is
// only reachable through injection.
func TestSave_MarshalError(t *testing.T) {
	orig := yamlMarshal
	yamlMarshal = func(v any) ([]byte, error) {
		return nil, fmt.Errorf("stubbed marshal failure")
	}
	defer func() { yamlMarshal = orig }()

	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	err := Save(path, DefaultConfig())
	if err == nil {
		t.Fatal("expected Save to fail when yaml.Marshal errors")
	}
	if !strings.Contains(err.Error(), "marshal config") {
		t.Errorf("err=%q, expected 'marshal config' substring", err.Error())
	}
}
