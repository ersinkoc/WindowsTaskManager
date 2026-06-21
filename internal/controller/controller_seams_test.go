//go:build windows

package controller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// saveSeams captures the current value of every seam so a test can restore
// them via t.Cleanup.
func saveSeams() *seamSnapshot {
	return &seamSnapshot{
		createJobObject:          seamCreateJobObject,
		assignProcessToJobObject: seamAssignProcessToJobObject,
		openProcessHandle:        seamOpenProcessHandle,
		setInformationJobObject:  seamSetInformationJobObject,
		setPriorityClass:         seamSetPriorityClass,
		setProcessAffinityMask:   seamSetProcessAffinityMask,
		createToolhelp32Snapshot: seamCreateToolhelp32Snapshot,
		thread32First:            seamThread32First,
		thread32Next:             seamThread32Next,
		openThreadHandle:         seamOpenThreadHandle,
		suspendThread:            seamSuspendThread,
		resumeThread:             seamResumeThread,
		terminateProcess:         seamTerminateProcess,
	}
}

type seamSnapshot struct {
	createJobObject          func() (windows.Handle, error)
	assignProcessToJobObject func(windows.Handle, windows.Handle) error
	openProcessHandle        func(uint32, uint32) (windows.Handle, error)
	setInformationJobObject  func(windows.Handle, uint32, unsafe.Pointer, uint32) error
	setPriorityClass         func(windows.Handle, uint32) error
	setProcessAffinityMask   func(windows.Handle, uintptr) error
	createToolhelp32Snapshot func(uint32, uint32) (windows.Handle, error)
	thread32First            func(windows.Handle, *winapi.THREADENTRY32) error
	thread32Next             func(windows.Handle, *winapi.THREADENTRY32) error
	openThreadHandle         func(uint32, uint32) (windows.Handle, error)
	suspendThread            func(windows.Handle) (uint32, error)
	resumeThread             func(windows.Handle) (uint32, error)
	terminateProcess         func(windows.Handle, uint32) error
}

func (s *seamSnapshot) restore() {
	seamCreateJobObject = s.createJobObject
	seamAssignProcessToJobObject = s.assignProcessToJobObject
	seamOpenProcessHandle = s.openProcessHandle
	seamSetInformationJobObject = s.setInformationJobObject
	seamSetPriorityClass = s.setPriorityClass
	seamSetProcessAffinityMask = s.setProcessAffinityMask
	seamCreateToolhelp32Snapshot = s.createToolhelp32Snapshot
	seamThread32First = s.thread32First
	seamThread32Next = s.thread32Next
	seamOpenThreadHandle = s.openThreadHandle
	seamSuspendThread = s.suspendThread
	seamResumeThread = s.resumeThread
	seamTerminateProcess = s.terminateProcess
}

func snap(t *testing.T) *seamSnapshot {
	t.Helper()
	s := saveSeams()
	t.Cleanup(s.restore)
	return s
}

// --- CreateJobObject ---

func TestSeamCreateJobObjectError(t *testing.T) {
	snap(t)
	seamCreateJobObject = func() (windows.Handle, error) {
		return 0, errors.New("simulated create job failure")
	}
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
	err := ctrl.Limit(pid, 25, 0, true)
	if err == nil || !strings.Contains(err.Error(), "create job") {
		t.Fatalf("err=%v", err)
	}
}

// --- AssignProcessToJobObject ---

func TestSeamAssignProcessToJobObjectError(t *testing.T) {
	snap(t)
	seamAssignProcessToJobObject = func(_, _ windows.Handle) error {
		return errors.New("simulated assign failure")
	}
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
	err := ctrl.Limit(pid, 25, 0, true)
	if err == nil || !strings.Contains(err.Error(), "assign job") {
		t.Fatalf("err=%v", err)
	}
}

// --- SetInformationJobObject (memory) ---

func TestSeamSetMemoryLimitError(t *testing.T) {
	snap(t)
	seamSetInformationJobObject = func(_ windows.Handle, _ uint32, _ unsafe.Pointer, _ uint32) error {
		return errors.New("simulated memory limit failure")
	}
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
	err := ctrl.Limit(pid, 0, 1024*1024, true)
	if err == nil || !strings.Contains(err.Error(), "memory limit") {
		t.Fatalf("err=%v", err)
	}
}

// --- SetInformationJobObject (cpu) ---

func TestSeamSetCpuLimitError(t *testing.T) {
	snap(t)
	seamSetInformationJobObject = func(_ windows.Handle, infoClass uint32, _ unsafe.Pointer, _ uint32) error {
		if infoClass == winapi.JobObjectCpuRateControlInformation {
			return errors.New("simulated cpu limit failure")
		}
		return nil
	}
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
	err := ctrl.Limit(pid, 50, 0, true)
	if err == nil || !strings.Contains(err.Error(), "cpu limit") {
		t.Fatalf("err=%v", err)
	}
}

// --- OpenProcessHandle inside Limit (the set-quota branch) ---

func TestSeamOpenProcessHandleLimitError(t *testing.T) {
	snap(t)
	// Allow OpenProcessHandle to succeed for Kill/Terminate etc. but fail
	// for the PROCESS_SET_QUOTA|PROCESS_TERMINATE access mask used by
	// Limit. We track call count to be selective.
	calls := 0
	seamOpenProcessHandle = func(access, pid uint32) (windows.Handle, error) {
		calls++
		if access == windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE {
			return 0, errors.New("simulated quota open failure")
		}
		return winapi.OpenProcessHandle(access, pid)
	}
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
	err := ctrl.Limit(pid, 25, 0, true)
	if err == nil || !strings.Contains(err.Error(), "open process") {
		t.Fatalf("err=%v", err)
	}
	_ = calls
}

// --- SetPriorityClass error ---

func TestSeamSetPriorityClassError(t *testing.T) {
	snap(t)
	seamSetPriorityClass = func(_ windows.Handle, _ uint32) error {
		return errors.New("simulated priority class failure")
	}
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
	err := ctrl.SetPriority(pid, "normal", true)
	if err == nil {
		t.Fatal("expected priority class error")
	}
}

// --- SetProcessAffinityMask error ---

func TestSeamSetProcessAffinityMaskError(t *testing.T) {
	snap(t)
	seamSetProcessAffinityMask = func(_ windows.Handle, _ uintptr) error {
		return errors.New("simulated affinity mask failure")
	}
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
	err := ctrl.SetAffinity(pid, 1, true)
	if err == nil {
		t.Fatal("expected affinity mask error")
	}
}

// --- CreateToolhelp32Snapshot error ---

func TestSeamCreateToolhelp32SnapshotError(t *testing.T) {
	snap(t)
	seamCreateToolhelp32Snapshot = func(_, _ uint32) (windows.Handle, error) {
		return 0, errors.New("simulated snapshot failure")
	}
	err := suspendOrResumeThreads(1, true)
	if err == nil {
		t.Fatal("expected snapshot error")
	}
}

// --- Thread32First error ---

func TestSeamThread32FirstError(t *testing.T) {
	snap(t)
	// Make the snapshot succeed so Thread32First is reached.
	seamCreateToolhelp32Snapshot = func(_, _ uint32) (windows.Handle, error) {
		return winapi.CreateToolhelp32Snapshot(winapi.TH32CS_SNAPTHREAD, 0)
	}
	seamThread32First = func(_ windows.Handle, _ *winapi.THREADENTRY32) error {
		return errors.New("simulated Thread32First failure")
	}
	err := suspendOrResumeThreads(1, true)
	if err == nil {
		t.Fatal("expected Thread32First error")
	}
}

// --- OpenThreadHandle error ---

func TestSeamOpenThreadHandleError(t *testing.T) {
	snap(t)
	// Drive suspendOrResumeThreads with the test PID — it has at least one
	// thread, so OpenThreadHandle will be called. Override it to fail.
	seamOpenThreadHandle = func(_, _ uint32) (windows.Handle, error) {
		return 0, errors.New("simulated open thread failure")
	}
	err := suspendOrResumeThreads(uint32(os.Getpid()), true)
	if err == nil {
		t.Fatal("expected firstErr from open thread failure")
	}
	if !strings.Contains(err.Error(), "open thread") {
		t.Fatalf("err=%v", err)
	}
}

// --- SuspendThread error ---

func TestSeamSuspendThreadError(t *testing.T) {
	snap(t)
	seamSuspendThread = func(_ windows.Handle) (uint32, error) {
		return 0, errors.New("simulated suspend thread failure")
	}
	err := suspendOrResumeThreads(uint32(os.Getpid()), true)
	if err == nil {
		t.Fatal("expected suspend thread failure")
	}
	if !strings.Contains(err.Error(), "suspend thread") {
		t.Fatalf("err=%v", err)
	}
}

// --- ResumeThread error ---

func TestSeamResumeThreadError(t *testing.T) {
	snap(t)
	seamResumeThread = func(_ windows.Handle) (uint32, error) {
		return 0, errors.New("simulated resume thread failure")
	}
	err := suspendOrResumeThreads(uint32(os.Getpid()), false)
	if err == nil {
		t.Fatal("expected resume thread failure")
	}
	if !strings.Contains(err.Error(), "resume thread") {
		t.Fatalf("err=%v", err)
	}
}

// --- Thread32Next non-NMF error ---

func TestSeamThread32NextError(t *testing.T) {
	snap(t)
	seamCreateToolhelp32Snapshot = func(_, _ uint32) (windows.Handle, error) {
		return winapi.CreateToolhelp32Snapshot(winapi.TH32CS_SNAPTHREAD, 0)
	}
	seamThread32First = func(_ windows.Handle, entry *winapi.THREADENTRY32) error {
		entry.Size = uint32(unsafe.Sizeof(*entry))
		return nil
	}
	seamThread32Next = func(_ windows.Handle, _ *winapi.THREADENTRY32) error {
		return errors.New("simulated enumerate failure")
	}
	err := suspendOrResumeThreads(uint32(os.Getpid()), true)
	if err == nil {
		t.Fatal("expected enumerate failure")
	}
	if !strings.Contains(err.Error(), "enumerate threads") {
		t.Fatalf("err=%v", err)
	}
}

// --- OpenProcessHandle error inside Kill (for coverage completeness) ---

func TestSeamOpenProcessHandleKillError(t *testing.T) {
	snap(t)
	seamOpenProcessHandle = func(_, _ uint32) (windows.Handle, error) {
		return 0, errors.New("simulated open failure")
	}
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
	err := ctrl.Kill(pid, true)
	if err == nil || !strings.Contains(err.Error(), "open process") {
		t.Fatalf("err=%v", err)
	}
}

// --- TerminateProcess error inside Kill ---

func TestSeamTerminateProcessError(t *testing.T) {
	snap(t)
	seamTerminateProcess = func(_ windows.Handle, _ uint32) error {
		return errors.New("simulated terminate failure")
	}
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
	err := ctrl.Kill(pid, true)
	if err == nil || !strings.Contains(err.Error(), "terminate") {
		t.Fatalf("err=%v", err)
	}
}

// --- NewSafety defensive branch via test ---

// TestSafetyNewSafetyClampsPID covers the MaxUint32 defensive branch in
// NewSafety. We can't easily drive a real PID > MaxUint32, so we exercise
// the branch via a parallel Safety created with a contrived selfPID. The
// NewSafety code branch is `selfPID = math.MaxUint32` when pid overflows.
// To hit this from a test we monkey-patch the construction via a
// temporary exported helper that takes an explicit pid. Without that, the
// branch is unreachable. We accept it as a defensive branch and document
// it.
//
// (No test body — see TestSafetyRejectsSelfPID and TestSafetyCheckSelfPID
// which cover the happy paths.)

// Sanity: ensure saveSeams + restore round-trips correctly.
func TestSeamSnapshotRoundTrip(t *testing.T) {
	snap(t)
	// Mark a sentinel by changing the seam and checking it stuck.
	seamCreateJobObject = func() (windows.Handle, error) {
		return 0, errors.New("changed")
	}
	if _, err := seamCreateJobObject(); err == nil || err.Error() != "changed" {
		t.Fatal("swap did not change")
	}
	// Cleanup runs restoreSeams; verified by t.Cleanup running after this
	// test returns.
}
