//go:build windows

package collector

import (
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

func TestNewProcessCollectorDefaultsZeroMax(t *testing.T) {
	pc := NewProcessCollector(0)
	if pc.maxResults != 2000 {
		t.Fatalf("maxResults=%d want 2000", pc.maxResults)
	}
	if pc.prev == nil {
		t.Fatal("expected prev map initialized")
	}
	if pc.numLogical <= 0 {
		t.Fatalf("numLogical=%d", pc.numLogical)
	}
}

func TestNewProcessCollectorDefaultsNegativeMax(t *testing.T) {
	pc := NewProcessCollector(-5)
	if pc.maxResults != 2000 {
		t.Fatalf("maxResults=%d want 2000", pc.maxResults)
	}
}

func TestNewProcessCollectorHonorsMax(t *testing.T) {
	pc := NewProcessCollector(50)
	if pc.maxResults != 50 {
		t.Fatalf("maxResults=%d want 50", pc.maxResults)
	}
}

func TestProcessCollectorCollectSmoke(t *testing.T) {
	pc := NewProcessCollector(2000)
	procs := pc.Collect()
	// We don't assume there's a process list, but the call must succeed.
	_ = procs
}

func TestProcessCollectorCollectSecondSampleComputesDeltas(t *testing.T) {
	pc := NewProcessCollector(2000)
	_ = pc.Collect()
	procs := pc.Collect()
	// Some processes may have deltas (cpu, io). The function shouldn't panic.
	_ = procs
}

func TestSortByWorkingSetDesc(t *testing.T) {
	in := []metrics.ProcessInfo{
		{PID: 1, WorkingSet: 100},
		{PID: 2, WorkingSet: 500},
		{PID: 3, WorkingSet: 300},
	}
	sortByWorkingSetDesc(in)
	if in[0].PID != 2 || in[1].PID != 3 || in[2].PID != 1 {
		t.Fatalf("sort order wrong: %+v", in)
	}
}

func TestSortByWorkingSetDescEmptyAndSingle(t *testing.T) {
	sortByWorkingSetDesc(nil)
	sortByWorkingSetDesc([]metrics.ProcessInfo{{PID: 1, WorkingSet: 10}})
}

func TestTrimByWorkingSetReturnsInputWhenSmall(t *testing.T) {
	in := []metrics.ProcessInfo{
		{PID: 1, WorkingSet: 100},
		{PID: 2, WorkingSet: 200},
	}
	got := trimByWorkingSet(in, 10)
	if len(got) != 2 {
		t.Fatalf("expected length 2, got %d", len(got))
	}
}

func TestTrimByWorkingSetTrimsToN(t *testing.T) {
	in := []metrics.ProcessInfo{
		{PID: 1, WorkingSet: 100},
		{PID: 2, WorkingSet: 200},
		{PID: 3, WorkingSet: 50},
		{PID: 4, WorkingSet: 300},
	}
	got := trimByWorkingSet(in, 2)
	if len(got) != 2 {
		t.Fatalf("expected len 2, got %d", len(got))
	}
	// Should be the two largest by working set.
	if got[0].WorkingSet != 300 || got[1].WorkingSet != 200 {
		t.Fatalf("trimmed set wrong: %+v", got)
	}
}

func TestTrimByWorkingSetEqualToLen(t *testing.T) {
	in := []metrics.ProcessInfo{
		{PID: 1, WorkingSet: 100},
		{PID: 2, WorkingSet: 200},
	}
	got := trimByWorkingSet(in, 2)
	if len(got) != 2 {
		t.Fatalf("expected len 2, got %d", len(got))
	}
}

// Verify sort.Slice is exported and can be called the same way as our
// sortByWorkingSetDesc (this is just a smoke test for the local sort import).
var _ = sort.SliceStable

func TestProcessCollectorTrimPathLargeList(t *testing.T) {
	pc := NewProcessCollector(5)
	// Construct a fake prev state so internal caching doesn't crash.
	pc.prev[1] = procPrev{sampleTime: pc.prev[1].sampleTime}
	// We can't easily fabricate a long process list without syscalls, so just
	// exercise the public Collect path and trust the trim path is taken if
	// there are >5 procs.
	procs := pc.Collect()
	if len(procs) > 5 {
		t.Fatalf("expected <= 5 procs after trim, got %d", len(procs))
	}
}

// TestCollectOneClampsNegativeAndOver100CPUPercent hits the cpuPercent<0
// and cpuPercent>100 defensive clamp branches in collectOne by seeding
// prev ticks higher than current, then huge deltas.
func TestCollectOneClampsNegativeAndOver100CPUPercent(t *testing.T) {
	pc := NewProcessCollector(10)
	// Run once to populate prev with real ticks for current process (self).
	_ = pc.Collect()
	// Now bias prev values so the next Collect computes > 100 or < 0.
	pc.mu.Lock()
	for pid, prev := range pc.prev {
		prev.kernelTicks += 1_000_000_000_000 // huge future tick count
		prev.userTicks += 1_000_000_000_000
		pc.prev[pid] = prev
	}
	pc.mu.Unlock()
	// Sleep so elapsed > 0.
	time.Sleep(10 * time.Millisecond)
	procs := pc.Collect()
	// Clamping should keep CPUPercent in [0, 100] for any process.
	for _, p := range procs {
		if p.CPUPercent < 0 || p.CPUPercent > 100 {
			t.Fatalf("PID %d CPUPercent=%v out of range", p.PID, p.CPUPercent)
		}
	}
}

// TestCollectOneClampsNegativeCPUPercent exercises the defensive
// `cpuPercent < 0` branch in collectOne by setting numLogical to a
// negative value, which flips the sign of the final division. With
// normal float64 semantics the cpuPercent expression is always
// non-negative, so this branch is reachable only via direct field
// manipulation.
func TestCollectOneClampsNegativeCPUPercent(t *testing.T) {
	pc := NewProcessCollector(10)
	// Stub getProcessTimes to return a fixed kernel-tick delta of 1ms.
	savedTimes := getProcessTimes
	savedOpen := openProcessHandle
	savedCH := closeHandleSafe
	t.Cleanup(func() {
		getProcessTimes = savedTimes
		openProcessHandle = savedOpen
		closeHandleSafe = savedCH
	})
	openProcessHandle = func(access uint32, pid uint32) (windows.Handle, error) {
		return windows.Handle(0xfeed), nil
	}
	closeHandleSafe = func(h windows.Handle) {}
	// Return a fixed (but non-zero) kernel-tick delta.
	getProcessTimes = func(h windows.Handle) (winapi.FILETIME, winapi.FILETIME, winapi.FILETIME, winapi.FILETIME, error) {
		// 1ms = 10_000 ticks of 100ns.
		ft := winapi.FILETIME{LowDateTime: 10_000}
		return ft, ft, ft, ft, nil
	}
	// Seed prev with zero ticks at a past sample time.
	pc.mu.Lock()
	pc.prev[1234] = procPrev{
		kernelTicks: 0,
		userTicks:   0,
		sampleTime:  time.Now().Add(-1 * time.Second),
	}
	// Flip numLogical to negative so the final division produces a
	// negative cpuPercent — without this the defensive < 0 branch is
	// unreachable through normal execution.
	pc.numLogical = -1
	pc.mu.Unlock()

	entry := winapi.PROCESSENTRY32W{
		ProcessID:       1234,
		ParentProcessID: 1,
		Threads:         1,
	}
	live := make(map[uint32]procPrev)
	info := pc.collectOne(entry, time.Now(), live)
	// We expect the clamp to fire (cpuPercent would otherwise be < 0)
	// and CPUPercent to settle at 0.
	if info.CPUPercent < 0 {
		t.Fatalf("CPUPercent=%v should be clamped to >= 0", info.CPUPercent)
	}
}

// withStubs swaps out the package-level Win32 wrappers and restores them
// at the end of the test. Each test that needs stubs uses its own set.
type procStubs struct {
	saved map[string]any
}

// TestCollectCreateToolhelpError covers the early-return when
// CreateToolhelp32Snapshot fails.
func TestCollectCreateToolhelpError(t *testing.T) {
	saved := createToolhelp32Snapshot
	t.Cleanup(func() { createToolhelp32Snapshot = saved })
	createToolhelp32Snapshot = func(flags, pid uint32) (windows.Handle, error) {
		return 0, errors.New("snapshot failed")
	}
	pc := NewProcessCollector(10)
	if procs := pc.Collect(); procs != nil {
		t.Fatalf("expected nil when CreateToolhelp fails, got %d procs", len(procs))
	}
}

// TestCollectProcess32FirstError covers the early-return when Process32First fails.
func TestCollectProcess32FirstError(t *testing.T) {
	savedSnap := createToolhelp32Snapshot
	savedFirst := process32First
	t.Cleanup(func() {
		createToolhelp32Snapshot = savedSnap
		process32First = savedFirst
	})
	createToolhelp32Snapshot = func(flags, pid uint32) (windows.Handle, error) {
		return windows.Handle(0xdead), nil
	}
	closeHandleSafeBackup := closeHandleSafe
	t.Cleanup(func() { closeHandleSafe = closeHandleSafeBackup })
	closeHandleSafe = func(h windows.Handle) {}
	process32First = func(snap windows.Handle, entry *winapi.PROCESSENTRY32W) error {
		return errors.New("Process32First failed")
	}
	pc := NewProcessCollector(10)
	if procs := pc.Collect(); procs != nil {
		t.Fatalf("expected nil when Process32First fails, got %d procs", len(procs))
	}
}

// TestCollectOneGetProcessTimesError covers the else branch in collectOne where
// GetProcessTimes fails (live[pid] = procPrev{sampleTime: now} but no CPU info).
func TestCollectOneGetProcessTimesError(t *testing.T) {
	saved := getProcessTimes
	t.Cleanup(func() { getProcessTimes = saved })
	getProcessTimes = func(h windows.Handle) (winapi.FILETIME, winapi.FILETIME, winapi.FILETIME, winapi.FILETIME, error) {
		return winapi.FILETIME{}, winapi.FILETIME{}, winapi.FILETIME{}, winapi.FILETIME{}, errors.New("times failed")
	}
	pc := NewProcessCollector(10)
	// We don't seed prev so the times branch isn't entered; the else branch
	// in collectOne runs whenever terr != nil for any non-PID-0/4 process.
	// Since the natural Collect always tries the times call, the else branch
	// is hit only when our stub returns an error. The real call would
	// succeed, so we synthesize via direct invocation.
	entry := winapi.PROCESSENTRY32W{ProcessID: 1234, ParentProcessID: 1, Threads: 1}
	live := make(map[uint32]procPrev)
	info := pc.collectOne(entry, time.Now(), live)
	if info.CPUPercent != 0 {
		t.Fatalf("CPUPercent should be 0 when GetProcessTimes fails, got %v", info.CPUPercent)
	}
	if info.CreateTime != 0 {
		t.Fatalf("CreateTime should be 0 when GetProcessTimes fails, got %d", info.CreateTime)
	}
	if _, ok := live[1234]; !ok {
		t.Fatal("expected live entry to be populated even on GetProcessTimes error")
	}
}

// TestCollectOneBasenameDiffers covers the QueryFullProcessImageName path
// where the path's basename differs from the entry's ExeFile name.
func TestCollectOneBasenameDiffers(t *testing.T) {
	savedOpen := openProcessHandle
	savedQFPI := queryFullProcessImageName
	savedCH := closeHandleSafe
	t.Cleanup(func() {
		openProcessHandle = savedOpen
		queryFullProcessImageName = savedQFPI
		closeHandleSafe = savedCH
	})
	openProcessHandle = func(access uint32, pid uint32) (windows.Handle, error) {
		return windows.Handle(0xfeed), nil
	}
	closeHandleSafe = func(h windows.Handle) {}
	queryFullProcessImageName = func(h windows.Handle) (string, error) {
		return `C:\Path\To\Different\Name\foo.exe`, nil
	}
	pc := NewProcessCollector(10)
	entry := winapi.PROCESSENTRY32W{
		ProcessID:       5678,
		ParentProcessID: 1,
		Threads:         1,
	}
	// Set ExeFile to a different name from the basename we'll return.
	copy(entry.ExeFile[:], windows.StringToUTF16("oldname.exe"))
	live := make(map[uint32]procPrev)
	info := pc.collectOne(entry, time.Now(), live)
	if info.Name != "foo.exe" {
		t.Fatalf("expected Name=foo.exe (from path basename), got %q", info.Name)
	}
	if info.ExePath != `C:\Path\To\Different\Name\foo.exe` {
		t.Fatalf("ExePath=%q", info.ExePath)
	}
}

// TestCollectOneIsProcessCriticalTrue covers the path where IsProcessCritical
// returns true and IsCritical is set on the info.
func TestCollectOneIsProcessCriticalTrue(t *testing.T) {
	savedOpen := openProcessHandle
	savedIPC := isProcessCritical
	savedCH := closeHandleSafe
	t.Cleanup(func() {
		openProcessHandle = savedOpen
		isProcessCritical = savedIPC
		closeHandleSafe = savedCH
	})
	openProcessHandle = func(access uint32, pid uint32) (windows.Handle, error) {
		return windows.Handle(0xfeed), nil
	}
	closeHandleSafe = func(h windows.Handle) {}
	isProcessCritical = func(h windows.Handle) (bool, error) {
		return true, nil
	}
	pc := NewProcessCollector(10)
	entry := winapi.PROCESSENTRY32W{
		ProcessID:       9999,
		ParentProcessID: 1,
		Threads:         1,
	}
	live := make(map[uint32]procPrev)
	info := pc.collectOne(entry, time.Now(), live)
	if !info.IsCritical {
		t.Fatal("expected IsCritical to be true")
	}
}

// TestCollectOneOpenProcessError covers the branch where OpenProcessHandle
// fails (typically for protected PIDs).
func TestCollectOneOpenProcessError(t *testing.T) {
	saved := openProcessHandle
	t.Cleanup(func() { openProcessHandle = saved })
	openProcessHandle = func(access uint32, pid uint32) (windows.Handle, error) {
		return 0, errors.New("access denied")
	}
	pc := NewProcessCollector(10)
	entry := winapi.PROCESSENTRY32W{
		ProcessID:       12345,
		ParentProcessID: 1,
		Threads:         1,
	}
	copy(entry.ExeFile[:], windows.StringToUTF16("test.exe"))
	live := make(map[uint32]procPrev)
	info := pc.collectOne(entry, time.Now(), live)
	if info.Name != "test.exe" {
		t.Fatalf("expected name=test.exe on OpenProcess error, got %q", info.Name)
	}
	if _, ok := live[12345]; !ok {
		t.Fatal("expected live entry to be populated on OpenProcess error")
	}
}

// Save a guard so multiple tests don't step on each other.
var procStubMu sync.Mutex
