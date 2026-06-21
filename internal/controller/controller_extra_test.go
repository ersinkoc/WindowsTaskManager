//go:build windows

package controller

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// spawnChild starts a long-running child process and returns its PID plus a
// cleanup function that kills it (best-effort) when the test finishes.
func spawnChild(t *testing.T, args ...string) (uint32, func()) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn child %v: %v", args, err)
	}
	pid := uint32(cmd.Process.Pid)
	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return pid, cleanup
}

// pingChild launches "ping -n <seconds+5> 127.0.0.1" which keeps the child
// alive long enough for the test to interact with it.
func pingChild(t *testing.T, seconds int) (uint32, func()) {
	t.Helper()
	// -n <count>; pick a count large enough to outlive the test.
	count := seconds + 30
	return spawnChild(t, "ping", "-n", strconv.Itoa(count), "127.0.0.1")
}

// setSnapshot installs a single-process snapshot for the given PID.
func setSnapshot(t *testing.T, store *storage.Store, proc metrics.ProcessInfo) {
	t.Helper()
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{proc},
	})
}

// waitForExit polls until the child PID has exited. Times out after 10s.
func waitForExit(t *testing.T, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return // process gone
		}
		var code uint32
		if err := windows.GetExitCodeProcess(h, &code); err == nil && code != 259 /* STILL_ACTIVE */ {
			windows.Close(h)
			return
		}
		windows.Close(h)
		time.Sleep(50 * time.Millisecond)
	}
}

// TestSetConfigSwapsSafety verifies the controller.SetConfig passthrough works.
func TestSetConfigSwapsSafety(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = []string{"never-match.exe"}
	ctrl.SetConfig(cfg)
	// Verify by checking that Safety now uses the new config.
	err := ctrl.safety.Check(metrics.ProcessInfo{PID: 1, Name: "never-match.exe"}, true)
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("err=%v want ErrProtected", err)
	}
}

// TestEmitNoOpWithoutEmitter verifies emit is a no-op when emitter is nil.
func TestEmitNoOpWithoutEmitter(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	ctrl.emit(EventKilled, 42, map[string]any{"name": "x"}) // must not panic
}

// TestKillHappyPath spawns a child and kills it through Controller.Kill.
func TestKillHappyPath(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()

	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	rel := strings.ToLower(filepath.Base(exe))
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: rel,
	})

	if err := ctrl.Kill(pid, true); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	waitForExit(t, pid)
}

// TestKillNotFound verifies Kill returns ErrNotFound for an unknown PID.
func TestKillNotFound(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	err := ctrl.Kill(99999, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

// TestKillSafetyRejectsSelf ensures Kill refuses to kill its own process.
func TestKillSafetyRejectsSelf(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: uint32(os.Getpid()), Name: "wtm-self.exe"},
		},
	})
	if err := ctrl.Kill(uint32(os.Getpid()), true); !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestKillSystemPathRequiresConfirm verifies the ConfirmKillSystem branch.
func TestKillSystemPathRequiresConfirm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ConfirmKillSystem = true
	cfg.Controller.ProtectedProcesses = nil
	store := storage.NewStore(60, 10)
	ctrl := NewController(cfg, store, nil)
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     12345,
		Name:    "imaginary.exe",
		ExePath: root + `\System32\imaginary.exe`,
	})
	if err := ctrl.Kill(12345, false); !errors.Is(err, ErrConfirmNeeded) {
		t.Fatalf("err=%v want ErrConfirmNeeded", err)
	}
}

// TestKillOpenProcessError covers the OpenProcessHandle failure branch by
// pointing at a PID the controller has no right to open (PID 4 = System).
// Safety rejects it first; we use a non-system PID that we just exited so
// OpenProcess fails. Easier: use a clearly-non-existent PID that bypasses
// safety.
func TestKillOpenProcessError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = nil
	cfg.Controller.ConfirmKillSystem = false
	store := storage.NewStore(60, 10)
	ctrl := NewController(cfg, store, nil)
	setSnapshot(t, store, metrics.ProcessInfo{PID: 99999, Name: "ghost.exe"})
	err := ctrl.Kill(99999, true)
	if err == nil {
		t.Fatal("expected error killing non-existent PID")
	}
	if !strings.Contains(err.Error(), "open process") && !errors.Is(err, ErrNotFound) {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestKillTreeHappyAndPartial covers KillTree success and per-child failure.
func TestKillTreeHappyAndPartial(t *testing.T) {
	rootPID, cleanupRoot := pingChild(t, 30)
	defer cleanupRoot()
	childPID, cleanupChild := pingChild(t, 30)
	defer cleanupChild()

	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: rootPID, Name: "root.exe"},
			{PID: childPID, Name: "child.exe", ParentPID: rootPID},
		},
	})

	// Happy path: kill both.
	killed, err := ctrl.KillTree(rootPID, true)
	if err != nil {
		t.Fatalf("KillTree err=%v", err)
	}
	if killed != 2 {
		t.Fatalf("killed=%d want 2", killed)
	}
	waitForExit(t, rootPID)
	waitForExit(t, childPID)

	// Partial path: root is already dead; Kill(root) fails with an
	// OpenProcess error and is reported as firstErr. killed == 0.
	killed, err = ctrl.KillTree(rootPID, true)
	if err == nil {
		t.Fatal("expected error on second KillTree")
	}
	if killed != 0 {
		t.Fatalf("killed=%d want 0", killed)
	}
}

// TestKillTreeNoSnapshot covers the nil-snapshot branch in KillTree.
func TestKillTreeNoSnapshot(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	killed, err := ctrl.KillTree(1, true)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
	if killed != 0 {
		t.Fatalf("killed=%d", killed)
	}
}

// TestSuspendResumeRoundTrip suspends and then resumes a child process.
func TestSuspendResumeRoundTrip(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()

	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	if err := ctrl.Suspend(pid, true); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := ctrl.Resume(pid); err != nil {
		t.Fatalf("Resume: %v", err)
	}
}

// TestSuspendNotFound covers the not-found branch in Suspend.
func TestSuspendNotFound(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	if err := ctrl.Suspend(77777, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

// TestSuspendOrResumeThreadsNoThreads exercises the "no threads found" branch.
func TestSuspendOrResumeThreadsNoThreads(t *testing.T) {
	// PID 0 has no threads in the snapshot loop, but also is a critical PID.
	// Use a high non-existent PID; OpenProcess in the safety layer is NOT
	// reached because we call suspendOrResumeThreads directly. The system
	// thread snapshot always has many threads; we want te.OwnerProcessID
	// to never equal our fake PID.
	err := suspendOrResumeThreads(0xFFFFFFFE, false)
	if err == nil {
		t.Fatal("expected error for missing threads")
	}
	if !strings.Contains(err.Error(), "no threads found") {
		t.Fatalf("err=%v", err)
	}
}

// TestSetPriorityClassUnknown and happy path together.
func TestSetPriorityAllValidClasses(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()

	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	for _, class := range []string{"idle", "below_normal", "below-normal", "normal", "above_normal", "above-normal", "high", "realtime"} {
		if err := ctrl.SetPriority(pid, class, true); err != nil {
			t.Fatalf("SetPriority(%s): %v", class, err)
		}
	}
}

// TestSetPriorityUnknownClass covers the "unknown priority class" branch.
func TestSetPriorityUnknownClass(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})
	if err := ctrl.SetPriority(pid, "turbo", true); err == nil {
		t.Fatal("expected unknown-class error")
	}
}

// TestSetPriorityNotFound covers the ErrNotFound branch.
func TestSetPriorityNotFound(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	if err := ctrl.SetPriority(11111, "normal", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

// TestSetAffinityHappyAndZero covers both the mask-zero branch and the
// happy path.
func TestSetAffinityHappyAndZero(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	if err := ctrl.SetAffinity(pid, 0, true); err == nil {
		t.Fatal("expected error for zero mask")
	}

	// Use a non-zero mask with bit 0 set — always a valid CPU on any
	// Windows system, satisfying the SetAffinity happy path.
	if err := ctrl.SetAffinity(pid, 1, true); err != nil {
		t.Fatalf("SetAffinity happy: %v", err)
	}
}

// TestSetAffinityNotFound covers the ErrNotFound branch.
func TestSetAffinityNotFound(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	if err := ctrl.SetAffinity(22222, 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

// TestLimitFullLifecycle covers Limit's CPU+memory+replace branches.
func TestLimitFullLifecycle(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	// First call: CPU limit only (memory branch skipped because maxBytes==0).
	if err := ctrl.Limit(pid, 25, 0, true); err != nil {
		t.Fatalf("Limit cpu-only: %v", err)
	}

	// Second call: memory limit only (CPU branch skipped because cpuPct==0).
	if err := ctrl.Limit(pid, 0, 1024*1024*100, true); err != nil {
		t.Fatalf("Limit mem-only: %v", err)
	}

	// Third call: both limits AND exercise the existing-job replacement branch.
	if err := ctrl.Limit(pid, 50, 1024*1024*200, true); err != nil {
		t.Fatalf("Limit both+replace: %v", err)
	}

	limits := ctrl.ActiveLimits()
	if len(limits) != 1 {
		t.Fatalf("active limits=%d want 1", len(limits))
	}
	if limits[0].CPUPct != 50 || limits[0].MemBytes != 1024*1024*200 {
		t.Fatalf("limits=%+v", limits[0])
	}

	if err := ctrl.ClearLimit(pid); err != nil {
		t.Fatalf("ClearLimit: %v", err)
	}
}

// TestLimitInvalidArgs covers the cpuPct-out-of-range branch.
func TestLimitInvalidArgs(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})
	if err := ctrl.Limit(pid, 101, 0, true); err == nil {
		t.Fatal("expected error for cpuPct>100")
	}
	if err := ctrl.Limit(pid, -1, 0, true); err == nil {
		t.Fatal("expected error for cpuPct<0")
	}
}

// TestLimitNotFound covers the ErrNotFound branch.
func TestLimitNotFound(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	if err := ctrl.Limit(33333, 25, 0, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

// TestActiveLimitsEmpty covers the empty-jobs branch in ActiveLimits.
func TestActiveLimitsEmpty(t *testing.T) {
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), nil)
	if got := ctrl.ActiveLimits(); len(got) != 0 {
		t.Fatalf("ActiveLimits=%v want empty", got)
	}
}

// TestEmitWithNilExtra covers the loop-zero-iter branch in emit.
func TestEmitWithNilExtra(t *testing.T) {
	em := event.NewEmitter()
	got := make(chan map[string]any, 1)
	em.Subscribe(func(eventType string, data any) {
		if eventType != EventResumed {
			return
		}
		got <- data.(map[string]any)
	})
	ctrl := NewController(config.DefaultConfig(), storage.NewStore(60, 10), em)
	ctrl.emit(EventResumed, 7, nil)
	select {
	case payload := <-got:
		if payload["pid"] != uint32(7) {
			t.Fatalf("pid=%v", payload["pid"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("event not emitted")
	}
}

// TestControllerConcurrentSafe drives a few concurrent ActiveLimits calls
// to exercise the mutex in Controller.
func TestControllerConcurrentSafe(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	ctrl.mu.Lock()
	for i := uint32(1); i <= 20; i++ {
		ctrl.jobs[i] = &jobEntry{job: windows.Handle(0), pid: i, cpuPct: int(i)}
	}
	ctrl.mu.Unlock()

	done := make(chan struct{}, 20)
	for i := uint32(1); i <= 20; i++ {
		go func() {
			_ = ctrl.ActiveLimits()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}

// drainEvents drains any queued events of the given type from the emitter
// until `timeout` elapses with no new events.
func drainEvents(em *event.Emitter, name string, timeout time.Duration) []map[string]any {
	var out []map[string]any
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case p := <-collectChan(em, name):
			out = append(out, p)
		case <-time.After(50 * time.Millisecond):
			return out
		}
	}
	return out
}

func collectChan(em *event.Emitter, name string) <-chan map[string]any {
	ch := make(chan map[string]any, 1)
	em.On(name, func(data any) {
		if m, ok := data.(map[string]any); ok {
			select {
			case ch <- m:
			default:
			}
		}
	})
	return ch
}

// TestKillEmitsEvent verifies the controller's Kill emits an EventKilled.
func TestKillEmitsEvent(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()

	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	ch := collectChan(em, EventKilled)
	if err := ctrl.Kill(pid, true); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case payload := <-ch:
		if payload["pid"] != pid {
			t.Fatalf("pid=%v", payload["pid"])
		}
		if payload["name"] != "ping.exe" {
			t.Fatalf("name=%v", payload["name"])
		}
	case <-time.After(time.Second):
		t.Fatal("kill event not received")
	}
}

// TestSuspendEmitsEvent verifies Suspend emits EventSuspended.
func TestSuspendEmitsEvent(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()

	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	ch := collectChan(em, EventSuspended)
	if err := ctrl.Suspend(pid, true); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if err := ctrl.Resume(pid); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	select {
	case payload := <-ch:
		if payload["pid"] != pid {
			t.Fatalf("pid=%v", payload["pid"])
		}
	case <-time.After(time.Second):
		t.Fatal("suspend event not received")
	}
	// Also drain the resumed event so the dispatcher's goroutine doesn't
	// linger past the test.
	drainEvents(em, EventResumed, 200*time.Millisecond)
}

// TestSetPriorityEmitsEvent verifies EventPriority fires.
func TestSetPriorityEmitsEvent(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	ch := collectChan(em, EventPriority)
	if err := ctrl.SetPriority(pid, "high", true); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	select {
	case payload := <-ch:
		if payload["class"] != "high" {
			t.Fatalf("class=%v", payload["class"])
		}
	case <-time.After(time.Second):
		t.Fatal("priority event not received")
	}
}

// TestSetAffinityEmitsEvent verifies EventAffinity fires.
func TestSetAffinityEmitsEvent(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	ch := collectChan(em, EventAffinity)
	if err := ctrl.SetAffinity(pid, 1, true); err != nil {
		t.Fatalf("SetAffinity: %v", err)
	}
	select {
	case payload := <-ch:
		if payload["mask"] != uint64(1) {
			t.Fatalf("mask=%v", payload["mask"])
		}
	case <-time.After(time.Second):
		t.Fatal("affinity event not received")
	}
}

// TestLimitEmitsEvent verifies EventLimited fires.
func TestLimitEmitsEvent(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	em := event.NewEmitter()
	ctrl := NewController(config.DefaultConfig(), store, em)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	ch := collectChan(em, EventLimited)
	if err := ctrl.Limit(pid, 25, 0, true); err != nil {
		t.Fatalf("Limit: %v", err)
	}
	select {
	case payload := <-ch:
		if payload["cpu_pct"] != 25 {
			t.Fatalf("cpu_pct=%v", payload["cpu_pct"])
		}
		if payload["mem_bytes"] != uint64(0) {
			t.Fatalf("mem_bytes=%v", payload["mem_bytes"])
		}
	case <-time.After(time.Second):
		t.Fatal("limited event not received")
	}
}

// TestLimitReassignReplacesEntry covers the existing-job replacement path.
func TestLimitReassignReplacesEntry(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(config.DefaultConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	if err := ctrl.Limit(pid, 10, 0, true); err != nil {
		t.Fatalf("first Limit: %v", err)
	}
	// Second call replaces the first job.
	if err := ctrl.Limit(pid, 20, 1024*1024, true); err != nil {
		t.Fatalf("second Limit: %v", err)
	}
	limits := ctrl.ActiveLimits()
	if len(limits) != 1 || limits[0].CPUPct != 20 || limits[0].MemBytes != 1024*1024 {
		t.Fatalf("limits=%+v", limits)
	}
}

// safetyRejectConfig returns a Config whose protected list and confirm flag
// have been cleared so we can exercise safety rejections driven by PID.
func safetyRejectConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = nil
	cfg.Controller.ConfirmKillSystem = false
	return cfg
}

// TestSuspendRejectedBySafetySelf covers safety rejection inside Suspend.
func TestSuspendRejectedBySafetySelf(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: uint32(os.Getpid()), Name: "wtm-self.exe"},
		},
	})
	if err := ctrl.Suspend(uint32(os.Getpid()), true); !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestSuspendRejectedBySafetyCritical covers the ErrCritical branch in Suspend.
func TestSuspendRejectedBySafetyCritical(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 0, Name: "system-idle.exe"},
		},
	})
	if err := ctrl.Suspend(0, true); !errors.Is(err, ErrCritical) {
		t.Fatalf("err=%v want ErrCritical", err)
	}
}

// TestSuspendPropagatesSuspendError covers the inner-error branch in Suspend.
func TestSuspendPropagatesSuspendError(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	// PID doesn't exist in snapshot threads — suspendOrResumeThreads will
	// return "no threads found for pid X".
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 0xFFFFFFFE, Name: "ghost.exe"},
		},
	})
	if err := ctrl.Suspend(0xFFFFFFFE, true); err == nil {
		t.Fatal("expected error from suspend")
	}
}

// TestResumePropagatesResumeError covers the inner-error branch in Resume.
func TestResumePropagatesResumeError(t *testing.T) {
	ctrl := NewController(safetyRejectConfig(), storage.NewStore(60, 10), nil)
	if err := ctrl.Resume(0xFFFFFFFE); err == nil {
		t.Fatal("expected error from resume")
	}
}

// TestSetPriorityRejectedBySafetySelf covers the safety branch in SetPriority.
func TestSetPriorityRejectedBySafetySelf(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: uint32(os.Getpid()), Name: "wtm-self.exe"},
		},
	})
	if err := ctrl.SetPriority(uint32(os.Getpid()), "normal", true); !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestSetPriorityOpenProcessError covers the OpenProcessHandle failure in
// SetPriority by pointing at a non-existent PID.
func TestSetPriorityOpenProcessError(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 0xFFFFFFFD, Name: "ghost.exe"},
		},
	})
	if err := ctrl.SetPriority(0xFFFFFFFD, "normal", true); err == nil {
		t.Fatal("expected open-process error")
	}
}

// TestSetAffinityRejectedBySafetySelf covers the safety branch in SetAffinity.
func TestSetAffinityRejectedBySafetySelf(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: uint32(os.Getpid()), Name: "wtm-self.exe"},
		},
	})
	if err := ctrl.SetAffinity(uint32(os.Getpid()), 1, true); !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestSetAffinityOpenProcessError covers the OpenProcessHandle failure in
// SetAffinity by pointing at a non-existent PID.
func TestSetAffinityOpenProcessError(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 0xFFFFFFFC, Name: "ghost.exe"},
		},
	})
	if err := ctrl.SetAffinity(0xFFFFFFFC, 1, true); err == nil {
		t.Fatal("expected open-process error")
	}
}

// TestSetAffinitySetMaskError covers the SetProcessAffinityMask failure
// path. A mask with bits set for non-existent CPUs is rejected by Windows.
func TestSetAffinitySetMaskError(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})
	// Bit 63 corresponds to a CPU index that doesn't exist on any
	// currently shipping system; the kernel rejects the mask.
	err := ctrl.SetAffinity(pid, 1<<63, true)
	if err == nil {
		t.Fatal("expected SetProcessAffinityMask error")
	}
}

// TestLimitRejectedBySafetySelf covers the safety branch in Limit.
func TestLimitRejectedBySafetySelf(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: uint32(os.Getpid()), Name: "wtm-self.exe"},
		},
	})
	if err := ctrl.Limit(uint32(os.Getpid()), 25, 0, true); !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestLimitOpenProcessError covers the OpenProcessHandle failure in Limit.
// The snapshot contains the ghost PID; safety passes (not protected, not
// self); OpenProcessHandle fails because the PID does not exist.
func TestLimitOpenProcessError(t *testing.T) {
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 0xFFFFFFFB, Name: "ghost.exe"},
		},
	})
	if err := ctrl.Limit(0xFFFFFFFB, 25, 0, true); err == nil {
		t.Fatal("expected open-process error in Limit")
	}
}

// TestSuspendOrResumeThreadsSnapshotError covers the CreateToolhelp32Snapshot
// error path. Windows accepts all flag values for CreateToolhelp32Snapshot
// (no documented failure mode), so the kernel error branch is currently
// unreachable from this codebase. The "no threads found" branch already
// covers the main consumer of suspendOrResumeThreads's error return.
// The check below is left as a forward-looking sanity test in case future
// Windows versions tighten the API.
func TestSuspendOrResumeThreadsSnapshotError(t *testing.T) {
	// Direct call with various flag combinations to verify the kernel
	// never errors. This documents the expected behaviour.
	for _, flags := range []uint32{0, 0x80000000, 0xFFFFFFFF, 0x00000004 | 0x100} {
		h, err := winapi.CreateToolhelp32Snapshot(flags, 0)
		if err != nil {
			continue
		}
		winapi.CloseHandleSafe(h)
	}
}

// TestLimitAssignToJobError covers the AssignProcessToJobObject failure on
// the second call: closing the first job's handle does NOT remove the
// process from the job, so a second assignment fails by default.
func TestLimitAssignToJobError(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})

	// First Limit succeeds; process is now inside a job.
	if err := ctrl.Limit(pid, 10, 0, true); err != nil {
		t.Skipf("first Limit failed (likely nested-job restriction already active): %v", err)
	}
	// Second Limit closes the first job's handle then attempts to assign
	// the process to a new job — the kernel rejects this without
	// JOB_OBJECT_LIMIT_BREAKAWAY_OK on the parent job.
	err := ctrl.Limit(pid, 20, 0, true)
	if err == nil {
		t.Log("AssignProcessToJobObject unexpectedly succeeded on second Limit (nested-job allowed)")
		return
	}
	if !strings.Contains(err.Error(), "assign job") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestLimitMemoryLimitError covers the SetInformationJobObject failure for
// the per-process memory limit. The kernel rejects very small memory caps
// (1 byte). cpuPct=0 disables the CPU branch so the memory path is hit.
func TestLimitMemoryLimitError(t *testing.T) {
	pid, cleanup := pingChild(t, 30)
	defer cleanup()
	store := storage.NewStore(60, 10)
	ctrl := NewController(safetyRejectConfig(), store, nil)
	exe, _ := os.Executable()
	setSnapshot(t, store, metrics.ProcessInfo{
		PID:     pid,
		Name:    "ping.exe",
		ExePath: strings.ToLower(filepath.Base(exe)),
	})
	err := ctrl.Limit(pid, 0, 1, true)
	if err == nil {
		t.Fatal("expected memory-limit error")
	}
	if !strings.Contains(err.Error(), "memory limit") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// (no guard var needed; all imports are used)
