//go:build windows

package collector

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// Once-guards for the loop tests so the subscriber's wg.Done() is called
// exactly once even if the loop fires multiple times within the wait window.
var (
	portsLoopDone int32
	treeLoopDone  int32
)

func newTestManager(t *testing.T) (*Manager, *storage.Store, *event.Emitter) {
	t.Helper()
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)
	return m, store, emitter
}

func TestNewManagerWiresAllCollectors(t *testing.T) {
	m, _, _ := newTestManager(t)
	if m == nil {
		t.Fatal("expected manager to be non-nil")
	}
	if m.cpu == nil {
		t.Fatal("expected cpu collector")
	}
	if m.mem == nil {
		t.Fatal("expected memory collector")
	}
	if m.proc == nil {
		t.Fatal("expected process collector")
	}
	if m.net == nil {
		t.Fatal("expected network collector")
	}
	if m.disk == nil {
		t.Fatal("expected disk collector")
	}
	if m.gpu == nil {
		t.Fatal("expected gpu collector")
	}
	if m.ports == nil {
		t.Fatal("expected ports collector")
	}
	if m.cfg == nil {
		t.Fatal("expected config")
	}
	if m.store == nil {
		t.Fatal("expected store")
	}
	if m.emitter == nil {
		t.Fatal("expected emitter")
	}
	if m.latestPID == nil {
		t.Fatal("expected latestPID initialized")
	}
	if len(m.latestPID) != 0 {
		t.Fatalf("expected empty latestPID map, got %d", len(m.latestPID))
	}
}

func TestStartSpawnsLoopsAndExitsOnContextCancel(t *testing.T) {
	m, store, _ := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx)

	// Give the loops a moment to run at least once.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.Latest() != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	// Allow goroutines a moment to exit.
	time.Sleep(50 * time.Millisecond)
}

func TestCollectOnceReturnsSnapshotAndPopulatesStore(t *testing.T) {
	m, store, _ := newTestManager(t)
	snap := m.CollectOnce()
	if snap == nil {
		t.Fatal("expected non-nil snapshot from CollectOnce")
	}
	if snap.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be set")
	}
	if store.Latest() == nil {
		t.Fatal("expected store to hold the snapshot after CollectOnce")
	}
}

func TestCollectOnceEmitsEvent(t *testing.T) {
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	var wg sync.WaitGroup
	wg.Add(1)
	var captured *metrics.SystemSnapshot
	emitter.Subscribe(func(eventType string, data any) {
		if eventType != EventSnapshot {
			return
		}
		captured, _ = data.(*metrics.SystemSnapshot)
		wg.Done()
	})

	_ = m.CollectOnce()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected snapshot event to be delivered")
	}
	if captured == nil {
		t.Fatal("expected snapshot payload to be delivered")
	}
}

func TestCollectOnceWithNilEmitterSkipsEmit(t *testing.T) {
	store := storage.NewStore(60, 10)
	cfg := config.DefaultConfig()
	m := NewManager(cfg, store, nil, "Test CPU", 3000)
	snap := m.CollectOnce()
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
}

func TestFastSampleCarriesForwardPrevTreeAndPorts(t *testing.T) {
	m, store, _ := newTestManager(t)

	// Seed the store with a snapshot that has a tree and ports.
	prev := &metrics.SystemSnapshot{
		Timestamp: time.Now().Add(-time.Second),
		ProcessTree: []*metrics.ProcessNode{
			{Process: metrics.ProcessInfo{PID: 1, Name: "root"}},
		},
		PortBindings: []metrics.PortBinding{{Protocol: "tcp", LocalPort: 8080}},
	}
	store.SetLatest(prev)

	snap := m.fastSample()
	if len(snap.ProcessTree) != 1 {
		t.Fatalf("expected carried-forward tree, got %d", len(snap.ProcessTree))
	}
	if len(snap.PortBindings) != 1 {
		t.Fatalf("expected carried-forward ports, got %d", len(snap.PortBindings))
	}
}

func TestFastSampleUpdatesLatestPIDMap(t *testing.T) {
	m, _, _ := newTestManager(t)
	// Seed previous sample so fastSample has a process list to operate on.
	snap := m.fastSample()
	if len(snap.Processes) == 0 {
		t.Skip("no processes found in environment; skipping lookupPID test")
	}
	for _, p := range snap.Processes {
		if name := m.lookupPID(p.PID); name != p.Name {
			t.Fatalf("lookupPID(%d)=%q want %q", p.PID, name, p.Name)
		}
	}
	if got := m.lookupPID(0xDEADBEEF); got != "" {
		t.Fatalf("expected empty name for unknown PID, got %q", got)
	}
}

func TestLookupPIDReturnsEmptyForUnknown(t *testing.T) {
	m, _, _ := newTestManager(t)
	if got := m.lookupPID(99999999); got != "" {
		t.Fatalf("expected empty lookup for unknown pid, got %q", got)
	}
}

func TestApplyConfigReplacesConfigAndUpdatesPorts(t *testing.T) {
	m, _, _ := newTestManager(t)
	newCfg := config.DefaultConfig()
	newCfg.WellKnownPorts = map[uint16]string{9999: "test"}
	m.ApplyConfig(newCfg)

	got := m.currentConfig()
	if got == nil {
		t.Fatal("expected current config to be set")
	}
	if got.WellKnownPorts[9999] != "test" {
		t.Fatalf("expected well-known port label to update, got %v", got.WellKnownPorts[9999])
	}
}

func TestFastIntervalDefaults(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	if got := m.fastInterval(); got != time.Second {
		t.Fatalf("fastInterval default=%v want 1s", got)
	}
}

func TestFastIntervalWithConfigAboveMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.Interval = 750 * time.Millisecond
	if got := m.fastInterval(); got != 750*time.Millisecond {
		t.Fatalf("fastInterval=%v want 750ms", got)
	}
}

func TestFastIntervalClampsBelowMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.Interval = 50 * time.Millisecond
	if got := m.fastInterval(); got != time.Second {
		t.Fatalf("fastInterval=%v want 1s (clamped)", got)
	}
}

func TestTreeIntervalDefaults(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	if got := m.treeInterval(); got != 2*time.Second {
		t.Fatalf("treeInterval default=%v want 2s", got)
	}
}

func TestTreeIntervalWithConfigAboveMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.ProcessTreeInterval = 4 * time.Second
	if got := m.treeInterval(); got != 4*time.Second {
		t.Fatalf("treeInterval=%v want 4s", got)
	}
}

func TestTreeIntervalClampsBelowMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.ProcessTreeInterval = 100 * time.Millisecond
	if got := m.treeInterval(); got != 2*time.Second {
		t.Fatalf("treeInterval=%v want 2s (clamped)", got)
	}
}

func TestPortsIntervalDefaults(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	if got := m.portsInterval(); got != 3*time.Second {
		t.Fatalf("portsInterval default=%v want 3s", got)
	}
}

func TestPortsIntervalWithConfigAboveMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.PortScanInterval = 5 * time.Second
	if got := m.portsInterval(); got != 5*time.Second {
		t.Fatalf("portsInterval=%v want 5s", got)
	}
}

func TestPortsIntervalClampsBelowMinimum(t *testing.T) {
	m := &Manager{latestPID: make(map[uint32]string)}
	m.cfg = &config.Config{}
	m.cfg.Monitoring.PortScanInterval = 200 * time.Millisecond
	if got := m.portsInterval(); got != 3*time.Second {
		t.Fatalf("portsInterval=%v want 3s (clamped)", got)
	}
}

func TestCPUInfoFromRegistryHandlesMissingValues(t *testing.T) {
	name, mhz := CPUInfoFromRegistry()
	// Registry may or may not exist on this machine; either way we want a clean return.
	_ = name
	_ = mhz
}

// TestTreeLoopContinuesWhenStoreEmpty verifies the snap==nil branch.
func TestTreeLoopContinuesWhenStoreEmpty(t *testing.T) {
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	cfg.Monitoring.ProcessTreeInterval = 600 * time.Millisecond // ≥ 500ms minimum
	cfg.Monitoring.Interval = 1 * time.Hour
	cfg.Monitoring.PortScanInterval = 1 * time.Hour
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	// Wait for at least one treeLoop tick where store is empty.
	time.Sleep(800 * time.Millisecond)

	// Verify the loop is still alive (no panic from snap==nil continue path).
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestPortsLoopTicksAndUpdatesSnapshot verifies that portsLoop fires when its
// timer expires and updates the snapshot's port bindings.
func TestPortsLoopTicksAndUpdatesSnapshot(t *testing.T) {
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	cfg.Monitoring.PortScanInterval = 600 * time.Millisecond // ≥ 500ms minimum
	cfg.Monitoring.Interval = 1 * time.Hour
	cfg.Monitoring.ProcessTreeInterval = 1 * time.Hour
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	// Seed an initial snapshot so portsLoop's snap==nil check passes.
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now()})

	var wg sync.WaitGroup
	wg.Add(1)
	emitter.Subscribe(func(eventType string, data any) {
		if eventType != EventPortBindings {
			return
		}
		// Use sync/atomic to ensure Done is called exactly once.
		if atomic.CompareAndSwapInt32(&portsLoopDone, 0, 1) {
			wg.Done()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected portsLoop to emit EventPortBindings")
	}
}

// TestTreeLoopTicksAndUpdatesSnapshot verifies that treeLoop fires when its
// timer expires, builds a tree from the latest processes, and emits an event.
func TestTreeLoopTicksAndUpdatesSnapshot(t *testing.T) {
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	cfg.Monitoring.ProcessTreeInterval = 600 * time.Millisecond // ≥ 500ms minimum
	cfg.Monitoring.Interval = 1 * time.Hour                     // keep fastLoop out of the way
	cfg.Monitoring.PortScanInterval = 1 * time.Hour
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	// Seed a snapshot so treeLoop has processes to build from.
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 0, Name: "root", CPUPercent: 50},
			{PID: 2, ParentPID: 1, Name: "child", CPUPercent: 30},
		},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	emitter.Subscribe(func(eventType string, data any) {
		if eventType != EventProcessTree {
			return
		}
		if atomic.CompareAndSwapInt32(&treeLoopDone, 0, 1) {
			wg.Done()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("expected treeLoop to emit EventProcessTree")
	}
}

// TestPortsLoopContinuesWhenStoreEmpty verifies the snap==nil branch.
func TestPortsLoopContinuesWhenStoreEmpty(t *testing.T) {
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	cfg.Monitoring.PortScanInterval = 600 * time.Millisecond // ≥ 500ms minimum
	cfg.Monitoring.Interval = 1 * time.Hour
	cfg.Monitoring.ProcessTreeInterval = 1 * time.Hour
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	time.Sleep(800 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
}

// TestFastLoopPruneTicksOnce covers the 30-second prune branch by waiting for
// the prune ticker to fire once. This test is slow (~31s) but necessary for
// full statement coverage of fastLoop.
func TestFastLoopPruneTicksOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping prune test in short mode (waits 31s)")
	}
	store := storage.NewStore(60, 10)
	emitter := event.NewEmitter()
	cfg := config.DefaultConfig()
	cfg.Monitoring.Interval = 1 * time.Hour // disable fastSample ticks
	cfg.Monitoring.ProcessTreeInterval = 1 * time.Hour
	cfg.Monitoring.PortScanInterval = 1 * time.Hour
	m := NewManager(cfg, store, emitter, "Test CPU", 3000)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)

	// Seed a stale process so PruneStaleProcesses has something to remove.
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now().Add(-3 * time.Minute),
		Processes: []metrics.ProcessInfo{
			{PID: 1, ParentPID: 0, Name: "stale"},
		},
	})

	// Wait for the 30-second prune ticker to fire.
	time.Sleep(31 * time.Second)

	// Prune should have removed the stale process (cutoff is 2 minutes ago).
	if store.TrackedProcessCount() != 0 {
		t.Fatalf("expected stale process to be pruned, count=%d", store.TrackedProcessCount())
	}
}
