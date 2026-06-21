package storage

import (
	"sync"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

func makeSnapshot(t time.Time, pids ...uint32) *metrics.SystemSnapshot {
	procs := make([]metrics.ProcessInfo, 0, len(pids))
	for _, pid := range pids {
		procs = append(procs, metrics.ProcessInfo{PID: pid, Name: "p.exe", CPUPercent: 1.0, WorkingSet: 1024})
	}
	return &metrics.SystemSnapshot{
		Timestamp: t,
		CPU:       metrics.CPUMetrics{TotalPercent: 10},
		Memory:    metrics.MemoryMetrics{UsedPercent: 50},
		GPU:       metrics.GPUMetrics{Utilization: 5},
		Network:   metrics.NetworkMetrics{TotalUpBPS: 100},
		Disk:      metrics.DiskMetrics{Drives: []metrics.DriveInfo{{Letter: "C:"}}},
		Processes: procs,
	}
}

func TestNewStoreEnforcesMinimumCapacity(t *testing.T) {
	// Both inputs below the minimums; constructor should clamp to 60 / 10.
	s := NewStore(0, 0)
	// Fill 70 system snapshots: ring capacity should be 60, not 70 or 0.
	base := time.Unix(1720000000, 0)
	for i := 0; i < 70; i++ {
		s.SetLatest(makeSnapshot(base.Add(time.Duration(i) * time.Second)))
	}
	if got := len(s.SystemHistory()); got != 60 {
		t.Fatalf("system history len=%d want 60 (clamped capacity)", got)
	}

	// Fill 15 samples for a single PID; proc capacity should be 10, not 15 or 0.
	store := NewStore(60, 0)
	for i := 0; i < 15; i++ {
		store.SetLatest(makeSnapshot(base.Add(time.Duration(i)*time.Second), 101))
	}
	if got := len(store.ProcessHistory(101)); got != 10 {
		t.Fatalf("process history len=%d want 10 (clamped capacity)", got)
	}
}

func TestUpdateLatestOnEmptyStore(t *testing.T) {
	s := NewStore(60, 10)
	if ok := s.UpdateLatest(func(_ *metrics.SystemSnapshot) {}); ok {
		t.Fatal("UpdateLatest on empty store returned true, want false")
	}
	// Latest should still be nil.
	if got := s.Latest(); got != nil {
		t.Fatalf("Latest()=%v want nil", got)
	}
}

func TestUpdateLatestDoesNotAppendHistory(t *testing.T) {
	store := NewStore(60, 10)
	ts := time.Unix(1710000000, 0)
	snap := &metrics.SystemSnapshot{
		Timestamp: ts,
		CPU:       metrics.CPUMetrics{TotalPercent: 10},
		Processes: []metrics.ProcessInfo{
			{PID: 42, Name: "demo.exe", CPUPercent: 2.5, WorkingSet: 1024},
		},
	}

	store.SetLatest(snap)
	store.UpdateLatest(func(latest *metrics.SystemSnapshot) {
		latest.ProcessTree = []*metrics.ProcessNode{{Process: latest.Processes[0]}}
		latest.PortBindings = []metrics.PortBinding{{PID: 42, Process: "demo.exe", LocalPort: 8080}}
	})

	if got := len(store.SystemHistory()); got != 1 {
		t.Fatalf("system history len=%d want 1", got)
	}
	if got := len(store.ProcessHistory(42)); got != 1 {
		t.Fatalf("process history len=%d want 1", got)
	}
	latest := store.Latest()
	if latest == nil {
		t.Fatal("latest snapshot is nil")
	}
	if len(latest.ProcessTree) != 1 {
		t.Fatalf("process tree len=%d want 1", len(latest.ProcessTree))
	}
	if len(latest.PortBindings) != 1 {
		t.Fatalf("port bindings len=%d want 1", len(latest.PortBindings))
	}
}

func TestLatestReturnsDetachedClone(t *testing.T) {
	store := NewStore(60, 10)
	ts := time.Unix(1710001000, 0)
	original := &metrics.SystemSnapshot{
		Timestamp: ts,
		CPU: metrics.CPUMetrics{
			TotalPercent: 20,
			PerCore:      []float64{10, 30},
		},
		Disk: metrics.DiskMetrics{
			Drives: []metrics.DriveInfo{{Letter: "C:", UsedPct: 42}},
		},
		Network: metrics.NetworkMetrics{
			Interfaces: []metrics.InterfaceInfo{{Name: "Ethernet", InBPS: 100}},
		},
		Processes: []metrics.ProcessInfo{
			{PID: 7, Name: "agent.exe", CPUPercent: 8.5},
		},
		ProcessTree: []*metrics.ProcessNode{{
			Process: metrics.ProcessInfo{PID: 7, Name: "agent.exe"},
			Children: []*metrics.ProcessNode{{
				Process: metrics.ProcessInfo{PID: 8, Name: "worker.exe"},
			}},
		}},
		PortBindings: []metrics.PortBinding{{PID: 7, Process: "agent.exe", LocalPort: 9000}},
	}

	store.SetLatest(original)

	original.Processes[0].Name = "mutated.exe"
	original.ProcessTree[0].Children[0].Process.Name = "mutated-child.exe"
	original.PortBindings[0].Process = "mutated.exe"

	latestA := store.Latest()
	if latestA == nil {
		t.Fatal("latest snapshot is nil")
	}
	if latestA.Processes[0].Name != "agent.exe" {
		t.Fatalf("stored process name=%q want agent.exe", latestA.Processes[0].Name)
	}
	if latestA.ProcessTree[0].Children[0].Process.Name != "worker.exe" {
		t.Fatalf("stored child name=%q want worker.exe", latestA.ProcessTree[0].Children[0].Process.Name)
	}
	if latestA.PortBindings[0].Process != "agent.exe" {
		t.Fatalf("stored binding process=%q want agent.exe", latestA.PortBindings[0].Process)
	}

	latestA.Processes[0].Name = "client-side-mutation.exe"
	latestA.CPU.PerCore[0] = 999
	latestA.ProcessTree[0].Process.Name = "changed-tree.exe"

	latestB := store.Latest()
	if latestB.Processes[0].Name != "agent.exe" {
		t.Fatalf("latest process name=%q want agent.exe", latestB.Processes[0].Name)
	}
	if latestB.CPU.PerCore[0] != 10 {
		t.Fatalf("latest per-core[0]=%v want 10", latestB.CPU.PerCore[0])
	}
	if latestB.ProcessTree[0].Process.Name != "agent.exe" {
		t.Fatalf("latest tree root=%q want agent.exe", latestB.ProcessTree[0].Process.Name)
	}
}

func TestSystemHistorySinceFilters(t *testing.T) {
	s := NewStore(60, 10)
	base := time.Unix(1730000000, 0)
	for i := 0; i < 5; i++ {
		s.SetLatest(makeSnapshot(base.Add(time.Duration(i) * time.Second)))
	}

	// Cutoff is exclusive: anything strictly after the cutoff is returned.
	cutoff := base.Add(2 * time.Second)
	rows := s.SystemHistorySince(cutoff)
	if got := len(rows); got != 2 {
		t.Fatalf("rows=%d want 2 (t+3s, t+4s)", got)
	}
	if !rows[0].Time.After(cutoff) || !rows[1].Time.After(cutoff) {
		t.Fatalf("returned rows are not all after cutoff: %+v", rows)
	}
	if !rows[0].Time.Equal(base.Add(3 * time.Second)) {
		t.Fatalf("first row time=%v want %v", rows[0].Time, base.Add(3*time.Second))
	}
	if !rows[1].Time.Equal(base.Add(4 * time.Second)) {
		t.Fatalf("second row time=%v want %v", rows[1].Time, base.Add(4*time.Second))
	}

	// Cutoff after all entries → empty result.
	if got := len(s.SystemHistorySince(base.Add(1 * time.Hour))); got != 0 {
		t.Fatalf("future cutoff rows=%d want 0", got)
	}
	// Cutoff before all entries → all entries returned.
	if got := len(s.SystemHistorySince(base.Add(-1 * time.Hour))); got != 5 {
		t.Fatalf("past cutoff rows=%d want 5", got)
	}
}

func TestProcessHistoryMissingPIDReturnsNil(t *testing.T) {
	s := NewStore(60, 10)
	if got := s.ProcessHistory(9999); got != nil {
		t.Fatalf("ProcessHistory(9999)=%v want nil", got)
	}
}

func TestPruneStaleProcesses(t *testing.T) {
	s := NewStore(60, 10)
	base := time.Unix(1740000000, 0)
	s.SetLatest(makeSnapshot(base, 1, 2, 3))
	s.SetLatest(makeSnapshot(base.Add(5*time.Second), 1, 2, 3))
	s.SetLatest(makeSnapshot(base.Add(10*time.Second), 1, 2, 3))

	if got := s.TrackedProcessCount(); got != 3 {
		t.Fatalf("tracked=%d want 3", got)
	}

	// PruneStaleProcesses drops a PID when its last-seen time is strictly
	// before the cutoff (last.Before(cutoff)). Equal timestamps are kept.
	cutoffKeep := base.Add(7 * time.Second)
	if removed := s.PruneStaleProcesses(cutoffKeep); removed != 0 {
		t.Fatalf("removed=%d want 0 (cutoff before last-seen)", removed)
	}
	if got := s.TrackedProcessCount(); got != 3 {
		t.Fatalf("tracked=%d want 3 after no-op prune", got)
	}

	// Cutoff = base+11s is strictly after every last-seen → prune all three.
	cutoffPruneAll := base.Add(11 * time.Second)
	if removed := s.PruneStaleProcesses(cutoffPruneAll); removed != 3 {
		t.Fatalf("removed=%d want 3", removed)
	}
	if got := s.TrackedProcessCount(); got != 0 {
		t.Fatalf("tracked=%d want 0 after pruning all", got)
	}
	if got := s.ProcessHistory(1); got != nil {
		t.Fatalf("ProcessHistory(1)=%v want nil after prune", got)
	}

	// Re-add a single PID and prune with a future cutoff → still pruned.
	s.SetLatest(makeSnapshot(base.Add(20*time.Second), 5))
	if removed := s.PruneStaleProcesses(base.Add(1 * time.Hour)); removed != 1 {
		t.Fatalf("removed=%d want 1", removed)
	}
	if got := s.TrackedProcessCount(); got != 0 {
		t.Fatalf("tracked=%d want 0", got)
	}
}

func TestTrackedProcessCount(t *testing.T) {
	s := NewStore(60, 10)
	if got := s.TrackedProcessCount(); got != 0 {
		t.Fatalf("tracked=%d want 0 on empty store", got)
	}
	base := time.Unix(1750000000, 0)
	s.SetLatest(makeSnapshot(base, 10, 20))
	if got := s.TrackedProcessCount(); got != 2 {
		t.Fatalf("tracked=%d want 2", got)
	}
	// Same PIDs again — count must not change.
	s.SetLatest(makeSnapshot(base.Add(time.Second), 10, 20))
	if got := s.TrackedProcessCount(); got != 2 {
		t.Fatalf("tracked=%d want 2 (no new PIDs)", got)
	}
	s.SetLatest(makeSnapshot(base.Add(2*time.Second), 10, 20, 30))
	if got := s.TrackedProcessCount(); got != 3 {
		t.Fatalf("tracked=%d want 3", got)
	}
}

func TestCloneSnapshotNil(t *testing.T) {
	if got := cloneSnapshot(nil); got != nil {
		t.Fatalf("cloneSnapshot(nil)=%v want nil", got)
	}
}

func TestCloneProcessTreeNilNodes(t *testing.T) {
	// A tree containing a nil entry must be preserved as a nil entry.
	nodes := []*metrics.ProcessNode{
		{Process: metrics.ProcessInfo{PID: 1, Name: "a"}},
		nil,
		{Process: metrics.ProcessInfo{PID: 2, Name: "b"}},
	}
	cloned := cloneProcessTree(nodes)
	if len(cloned) != 3 {
		t.Fatalf("len=%d want 3", len(cloned))
	}
	if cloned[0] == nil || cloned[0].Process.PID != 1 {
		t.Fatalf("cloned[0]=%v want non-nil PID=1", cloned[0])
	}
	if cloned[1] != nil {
		t.Fatalf("cloned[1]=%v want nil", cloned[1])
	}
	if cloned[2] == nil || cloned[2].Process.PID != 2 {
		t.Fatalf("cloned[2]=%v want non-nil PID=2", cloned[2])
	}

	// Mutating the source tree must not affect the clone.
	nodes[0].Process.Name = "mutated"
	if cloned[0].Process.Name != "a" {
		t.Fatalf("cloned[0].Name=%q want %q", cloned[0].Process.Name, "a")
	}

	// Empty input → nil output.
	if got := cloneProcessTree(nil); got != nil {
		t.Fatalf("cloneProcessTree(nil)=%v want nil", got)
	}
	if got := cloneProcessTree([]*metrics.ProcessNode{}); got != nil {
		t.Fatalf("cloneProcessTree([])=%v want nil", got)
	}
}

func TestRecordSnapshotAccumulatesPerProcess(t *testing.T) {
	// Drive the recordSnapshotLocked path directly to cover the
	// "process already has a buffer" branch (ok==true).
	// Capacity must be above the constructor's minimum (10) so the ring
	// actually caps at the requested value.
	s := NewStore(60, 20)
	base := time.Unix(1760000000, 0)
	s.SetLatest(makeSnapshot(base, 7))
	// Second snapshot with same PID — should reuse the existing buffer.
	s.SetLatest(makeSnapshot(base.Add(time.Second), 7))
	// Insert 18 more samples (20 total); buffer capacity is 20 so all are kept.
	for i := 2; i < 20; i++ {
		s.SetLatest(makeSnapshot(base.Add(time.Duration(i)*time.Second), 7))
	}
	hist := s.ProcessHistory(7)
	if got := len(hist); got != 20 {
		t.Fatalf("process history len=%d want 20", got)
	}
	// First inserted sample should be the oldest one still in the ring.
	if !hist[0].Time.Equal(base) {
		t.Fatalf("oldest sample time=%v want %v", hist[0].Time, base)
	}
	if !hist[len(hist)-1].Time.Equal(base.Add(19 * time.Second)) {
		t.Fatalf("newest sample time=%v want %v", hist[len(hist)-1].Time, base.Add(19*time.Second))
	}
}

func TestSystemHistoryOnEmptyStore(t *testing.T) {
	s := NewStore(60, 10)
	// RingBuffer.Slice returns an empty (non-nil) slice for an empty ring;
	// the only contract is that it has length 0 and is safe to range over.
	if got := s.SystemHistory(); len(got) != 0 {
		t.Fatalf("empty SystemHistory() len=%d want 0", len(got))
	}
	if got := s.Latest(); got != nil {
		t.Fatalf("empty Latest()=%v want nil", got)
	}
}

func TestStoreConcurrentReadDuringSet(t *testing.T) {
	// Sanity check that locks are correct: concurrent SetLatest + Latest reads
	// must not race. Run with `go test -race` to detect data races.
	s := NewStore(60, 10)
	base := time.Unix(1770000000, 0)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				s.SetLatest(makeSnapshot(base.Add(time.Duration(i)*time.Millisecond), uint32(i%16)))
			}
		}
	}()

	for i := 0; i < 200; i++ {
		_ = s.Latest()
		_ = s.SystemHistory()
		_ = s.TrackedProcessCount()
	}
	close(stop)
	wg.Wait()
}
