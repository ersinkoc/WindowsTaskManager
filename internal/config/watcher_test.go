//go:build windows

package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWatcherReloadsOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	cfg := DefaultConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	changed := make(chan *Config, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(path, func(next *Config) {
		select {
		case changed <- next:
		default:
		}
	})
	go w.Start(ctx)
	time.Sleep(150 * time.Millisecond)

	cfg.Server.Port = 23456
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-changed:
		if got.Server.Port != 23456 {
			t.Fatalf("port=%d want %d", got.Server.Port, 23456)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not reload config after save")
	}
}

func TestNewWatcher_DebounceDefault(t *testing.T) {
	w := NewWatcher("ignored.yaml", nil)
	if w.path != "ignored.yaml" {
		t.Errorf("path=%q", w.path)
	}
	if w.onChange != nil {
		t.Error("expected onChange to be nil")
	}
	if w.debounce != 300*time.Millisecond {
		t.Errorf("debounce=%v want 300ms", w.debounce)
	}
}

func TestWatcher_currentModTime_MissingFile(t *testing.T) {
	w := NewWatcher(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	if got := w.currentModTime(); !got.IsZero() {
		t.Errorf("expected zero time for missing file, got %v", got)
	}
}

func TestWatcher_currentModTime_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	if got := w.currentModTime(); got.IsZero() {
		t.Error("expected non-zero mod time for existing file")
	}
}

func TestWatcher_reloadIfChanged_StatError(t *testing.T) {
	w := NewWatcher(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	got := w.reloadIfChanged(time.Now())
	if !got.Equal(time.Now()) {
		t.Errorf("expected unchanged lastMod, got %v", got)
	}
}

func TestWatcher_reloadIfChanged_NoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	mod := w.currentModTime()
	// ModTime() resolution on Windows is coarse — wait one tick to be safe.
	time.Sleep(20 * time.Millisecond)
	got := w.reloadIfChanged(mod)
	if !got.Equal(mod) {
		t.Errorf("expected unchanged mod, got %v", got)
	}
}

func TestWatcher_reloadIfChanged_TriggersCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	cfg := DefaultConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var (
		mu     sync.Mutex
		gotCfg *Config
	)
	w := NewWatcher(path, func(c *Config) {
		mu.Lock()
		gotCfg = c
		mu.Unlock()
	})
	mod := w.currentModTime()
	// Wait so file mtime is strictly greater than mod.
	time.Sleep(50 * time.Millisecond)
	cfg.Server.Port = 45678
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	newMod := w.reloadIfChanged(mod)
	if !newMod.After(mod) {
		t.Errorf("expected newMod > mod, got %v vs %v", newMod, mod)
	}
	// Callback runs synchronously inside reloadIfChanged.
	mu.Lock()
	if gotCfg == nil || gotCfg.Server.Port != 45678 {
		mu.Unlock()
		t.Fatalf("callback did not fire with new config: %+v", gotCfg)
	}
	mu.Unlock()
}

func TestWatcher_reloadIfChanged_LoadErrorNoCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	called := false
	w := NewWatcher(path, func(c *Config) { called = true })
	mod := w.currentModTime()
	time.Sleep(50 * time.Millisecond)
	// Corrupt the YAML so Load fails inside reloadIfChanged.
	if err := overwrite(path, []byte("::: not yaml :::\n - [\n")); err != nil {
		t.Fatal(err)
	}
	_ = w.reloadIfChanged(mod)
	if called {
		t.Error("callback should not fire when Load errors")
	}
}

func TestWatcher_pollStart_CancelBeforeTick(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.pollStart(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pollStart did not return after ctx cancel")
	}
}

func TestWatcher_pollStart_ReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	cfg := DefaultConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	var (
		mu   sync.Mutex
		seen []*Config
	)
	w := NewWatcher(path, func(c *Config) {
		mu.Lock()
		seen = append(seen, c)
		mu.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.pollStart(ctx)
		close(done)
	}()

	// Give the ticker time to start.
	time.Sleep(100 * time.Millisecond)
	cfg.Server.Port = 55555
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("pollStart did not detect save within 5s")
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	<-done
	mu.Lock()
	last := seen[len(seen)-1]
	mu.Unlock()
	if last.Server.Port != 55555 {
		t.Errorf("last port=%d want 55555", last.Server.Port)
	}
}

func TestWatcher_Start_FallsBackToPoll(t *testing.T) {
	// Point at a directory that does not exist so FindFirstChangeNotification
	// fails and Start falls back to pollStart.
	missingDir := filepath.Join(t.TempDir(), "does-not-exist")
	w := NewWatcher(filepath.Join(missingDir, "wtm.yaml"), func(c *Config) {})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	// Let the fallback path run briefly, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

// TestWatcher_Start_WaitTimeoutBranch forces Start through the WAIT_TIMEOUT
// arm of the switch by running it with no file changes for longer than the
// 500ms wait interval, then cancelling the context.
func TestWatcher_Start_WaitTimeoutBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	// No file activity for >500ms guarantees WaitForSingleObject returns
	// WAIT_TIMEOUT at least once before we cancel.
	time.Sleep(1200 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

// TestWatcher_Start_HandleInvalidated forces Start through the failure
// branches (WaitForSingleObject err / default) by deleting the watched
// directory while the watcher is mid-wait, which causes the next wait
// to return WAIT_FAILED on Windows.
func TestWatcher_Start_HandleInvalidated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "watched")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	// Give Start time to register the directory-change handle.
	time.Sleep(200 * time.Millisecond)
	// Remove the watched directory and all contents. On Windows this
	// invalidates the change-notification handle so WaitForSingleObject
	// returns WAIT_FAILED on the next call, exercising the fallback.
	if err := os.RemoveAll(dir); err != nil {
		t.Logf("RemoveAll: %v (test may still cover success path)", err)
	}
	// Allow the watcher to detect the invalidation.
	time.Sleep(800 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after ctx cancel")
	}
}

// TestWatcher_Start_WaitErrorBranch exercises the `if err != nil` branch in
// Start by swapping the waitForSingleObject variable with a stub that
// returns a non-nil error on its first call. This path is unreachable in
// normal Windows operation because WaitForSingleObject only returns a Go
// error when the syscall itself fails.
func TestWatcher_Start_WaitErrorBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	orig := waitForSingleObject
	calls := 0
	waitForSingleObject = func(handle windows.Handle, ms uint32) (uint32, error) {
		calls++
		return 0, errors.New("stubbed wait error")
	}
	defer func() { waitForSingleObject = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	// pollStart takes over after the error. Cancel context so pollStart
	// returns and Start exits.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after waitForSingleObject error")
	}
	if calls == 0 {
		t.Error("stubbed waitForSingleObject was never called")
	}
}

// TestWatcher_Start_DefaultBranch exercises the `default:` arm of Start's
// switch by returning an event value that is neither WAIT_OBJECT_0 nor
// WAIT_TIMEOUT. In practice WaitForSingleObject never returns such a value
// for a change-notification handle, so this branch is normally unreachable.
func TestWatcher_Start_DefaultBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wtm.yaml")
	if err := Save(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(path, nil)
	orig := waitForSingleObject
	waitForSingleObject = func(handle windows.Handle, ms uint32) (uint32, error) {
		// 0x80 is WAIT_ABANDONED — valid uint32 return with no error.
		return 0x80, nil
	}
	defer func() { waitForSingleObject = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	// pollStart takes over after the unexpected event. Cancel context so
	// pollStart returns and Start exits.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after unexpected event")
	}
}
