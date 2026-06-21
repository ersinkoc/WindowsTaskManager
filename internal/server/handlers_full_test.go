package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// ===== system endpoints =====

func TestHandleSystemNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleSystem(rr, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleSystemReturnsSnapshot(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now()})
	s, _ := fullTestServer(t, "", nil, store)
	rr := httptest.NewRecorder()
	s.handleSystem(rr, httptest.NewRequest(http.MethodGet, "/api/v1/system", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleSystemCPUAndFriends(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		CPU:       metrics.CPUMetrics{NumLogical: 4},
		Memory:    metrics.MemoryMetrics{UsedPercent: 50},
		GPU:       metrics.GPUMetrics{Name: "fake"},
		Disk:      metrics.DiskMetrics{},
		Network:   metrics.NetworkMetrics{},
	})
	s, _ := fullTestServer(t, "", nil, store)
	for _, tc := range []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"cpu", s.handleCPU},
		{"memory", s.handleMemory},
		{"gpu", s.handleGPU},
		{"disk", s.handleDisk},
		{"network", s.handleNetwork},
	} {
		rr := httptest.NewRecorder()
		tc.fn(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status=%d body=%s", tc.name, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleSystemSubsystemsNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	for _, tc := range []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"cpu", s.handleCPU},
		{"memory", s.handleMemory},
		{"gpu", s.handleGPU},
		{"disk", s.handleDisk},
		{"network", s.handleNetwork},
	} {
		rr := httptest.NewRecorder()
		tc.fn(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status=%d", tc.name, rr.Code)
		}
	}
}

func TestHandleHealth(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"ok"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestHandleHistoryBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now()})
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now().Add(-time.Minute)})
	s, _ := fullTestServer(t, "", nil, store)

	rr := httptest.NewRecorder()
	s.handleHistory(rr, httptest.NewRequest(http.MethodGet, "/api/v1/history", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	s.handleHistory(rr, httptest.NewRequest(http.MethodGet, "/api/v1/history?since=9999999999", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	s.handleHistory(rr, httptest.NewRequest(http.MethodGet, "/api/v1/history?since=bad", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

// ===== process endpoints =====

func TestHandleProcessesBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store,
		metrics.ProcessInfo{PID: 1, Name: "alpha.exe", CPUPercent: 5, WorkingSet: 100, PrivateBytes: 50, ThreadCount: 4},
		metrics.ProcessInfo{PID: 2, Name: "bravo.exe", CPUPercent: 50, WorkingSet: 200, PrivateBytes: 80, ThreadCount: 8},
		metrics.ProcessInfo{PID: 3, Name: "charlie.exe", CPUPercent: 90, WorkingSet: 50, PrivateBytes: 10, ThreadCount: 1},
	)
	s, _ := fullTestServer(t, "", nil, store)

	cases := []struct {
		name   string
		query  string
		expect int
	}{
		{"default cpu desc", "", 3},
		{"filter by name", "?name=alpha", 1},
		{"sort memory asc", "?sort=memory&order=asc", 3},
		{"sort private desc", "?sort=private&order=desc", 3},
		{"sort name asc", "?sort=name&order=asc", 3},
		{"sort pid", "?sort=pid&order=desc", 3},
		{"sort threads asc", "?sort=threads&order=asc", 3},
		{"limit trimmed", "?limit=1", 1},
		{"limit too large", "?limit=99999", 3},
		{"limit invalid", "?limit=abc", 3},
		{"unknown sort", "?sort=bogus", 3},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		s.handleProcesses(rr, httptest.NewRequest(http.MethodGet, "/api/v1/processes"+c.query, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status=%d", c.name, rr.Code)
			continue
		}
		var got []metrics.ProcessInfo
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Errorf("%s: decode: %v", c.name, err)
			continue
		}
		if len(got) != c.expect {
			t.Errorf("%s: count=%d want %d", c.name, len(got), c.expect)
		}
	}
}

func TestHandleProcessesNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleProcesses(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleProcessTreeBranches(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleProcessTree(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp:   time.Now(),
		ProcessTree: []*metrics.ProcessNode{{Process: metrics.ProcessInfo{PID: 1, Name: "root"}}},
	})
	s, _ = fullTestServer(t, "", nil, store)
	rr = httptest.NewRecorder()
	s.handleProcessTree(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleProcessByIDBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: 42, Name: "alpha"})
	s, _ := fullTestServer(t, "", nil, store)

	// missing pid
	rr := httptest.NewRecorder()
	s.handleProcessByID(rr, httptest.NewRequest(http.MethodGet, "/api/v1/processes/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing pid status=%d", rr.Code)
	}

	// not found
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/9999", nil)
	s.handleProcessByID(rr, paramCtx(req, map[string]string{"pid": "9999"}))
	if rr.Code != http.StatusNotFound {
		t.Errorf("not found status=%d", rr.Code)
	}

	// ok
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/processes/42", nil)
	s.handleProcessByID(rr, paramCtx(req, map[string]string{"pid": "42"}))
	if rr.Code != http.StatusOK {
		t.Errorf("ok status=%d", rr.Code)
	}

	// no snapshot
	s2, _ := fullTestServer(t, "", nil, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/processes/1", nil)
	s2.handleProcessByID(rr, paramCtx(req, map[string]string{"pid": "1"}))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("no snap status=%d", rr.Code)
	}
}

func TestHandleProcessHistoryOK(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), Processes: []metrics.ProcessInfo{{PID: 5}}})
	s, _ := fullTestServer(t, "", nil, store)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/5/history", nil)
	s.handleProcessHistory(rr, paramCtx(req, map[string]string{"pid": "5"}))
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleProcessHistoryMissingPID(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleProcessHistory(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleProcessChildrenBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store,
		metrics.ProcessInfo{PID: 1, ParentPID: 0},
		metrics.ProcessInfo{PID: 2, ParentPID: 1},
		metrics.ProcessInfo{PID: 3, ParentPID: 1},
		metrics.ProcessInfo{PID: 4, ParentPID: 2},
	)
	s, _ := fullTestServer(t, "", nil, store)

	// bad pid
	rr := httptest.NewRecorder()
	s.handleProcessChildren(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad pid status=%d", rr.Code)
	}

	// no snapshot
	s2, _ := fullTestServer(t, "", nil, nil)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s2.handleProcessChildren(rr, paramCtx(req, map[string]string{"pid": "1"}))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("no snap status=%d", rr.Code)
	}

	// happy path: children of pid 1 are {2, 3}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	s.handleProcessChildren(rr, paramCtx(req, map[string]string{"pid": "1"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []metrics.ProcessInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("children=%v", got)
	}
}

func TestHandleProcessConnectionsBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		PortBindings: []metrics.PortBinding{
			{PID: 10, LocalPort: 443},
			{PID: 20, LocalPort: 80},
		},
	})
	s, _ := fullTestServer(t, "", nil, store)

	// missing pid
	rr := httptest.NewRecorder()
	s.handleProcessConnections(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing pid status=%d", rr.Code)
	}

	// no snapshot
	s2, _ := fullTestServer(t, "", nil, nil)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s2.handleProcessConnections(rr, paramCtx(req, map[string]string{"pid": "10"}))
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("no snap status=%d", rr.Code)
	}

	// happy path filtered to pid 10
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	s.handleProcessConnections(rr, paramCtx(req, map[string]string{"pid": "10"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got []metrics.PortBinding
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].PID != 10 {
		t.Fatalf("got=%+v", got)
	}

	// happy path no rows for pid 99
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	s.handleProcessConnections(rr, paramCtx(req, map[string]string{"pid": "99"}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleKillHappy(t *testing.T) {
	pid, cleanup := pingSpawn(t, 5)
	defer cleanup()
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: pid, Name: "ping.exe"})
	s, _ := fullTestServer(t, "", nil, store)

	// Bad pid
	rr := httptest.NewRecorder()
	s.handleKill(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad pid=%d", rr.Code)
	}

	// Happy
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleKill(rr, paramCtx(req, map[string]string{"pid": strconv.FormatUint(uint64(pid), 10)}))
	if rr.Code != http.StatusOK {
		t.Fatalf("kill status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKillNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleKill(rr, paramCtx(req, map[string]string{"pid": "1"}))
	// No snapshot → controller.findProcess returns ErrNotFound → 404.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKillTreeBadPidAndNoSnap(t *testing.T) {
	// Bad pid
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleKillTree(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad pid status=%d", rr.Code)
	}

	// No snapshot
	s, _ = fullTestServer(t, "", nil, nil)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleKillTree(rr, paramCtx(req, map[string]string{"pid": "1"}))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKillTreeHappy(t *testing.T) {
	pid, cleanup := pingSpawn(t, 5)
	defer cleanup()
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: pid, Name: "ping.exe", ParentPID: 0})
	s, _ := fullTestServer(t, "", nil, store)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleKillTree(rr, paramCtx(req, map[string]string{"pid": strconv.FormatUint(uint64(pid), 10)}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleKillTreePartialError(t *testing.T) {
	// Spawn two children: parent kills successfully, child is protected →
	// KillTree returns (1, err) where err != nil and killed > 0 → 200 with
	// partial_error.
	parent, cleanupParent := pingSpawn(t, 5)
	defer cleanupParent()
	child, cleanupChild := pingSpawn(t, 5)
	defer cleanupChild()

	cfg := defaultCfg()
	cfg.Controller.ProtectedProcesses = []string{"protected.exe"}
	dir := t.TempDir()
	cfgPath := dir + "/wtm.yaml"
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: parent, Name: "ping.exe", ParentPID: 0},
			{PID: child, Name: "protected.exe", ParentPID: parent},
		},
	})
	s, _ := fullTestServer(t, cfgPath, cfg, store)
	s.store = store

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleKillTree(rr, paramCtx(req, map[string]string{"pid": strconv.FormatUint(uint64(parent), 10)}))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "partial_error") {
		t.Errorf("body missing partial_error: %s", rr.Body.String())
	}
}

func TestHandleSuspendResumePriorityAffinity(t *testing.T) {
	pid, cleanup := pingSpawn(t, 8)
	defer cleanup()
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: pid, Name: "ping.exe"})
	s, _ := fullTestServer(t, "", nil, store)
	pidStr := strconv.FormatUint(uint64(pid), 10)

	// Suspend bad pid
	rr := httptest.NewRecorder()
	s.handleSuspend(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("suspend bad pid status=%d", rr.Code)
	}

	// Suspend happy (with confirm)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleSuspend(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("suspend status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Resume happy
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleResume(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Priority bad pid
	rr = httptest.NewRecorder()
	s.handlePriority(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("priority bad pid status=%d", rr.Code)
	}

	// Priority bad JSON
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handlePriority(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("priority bad json status=%d", rr.Code)
	}

	// Priority happy
	body := `{"class":"normal"}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handlePriority(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("priority status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Affinity bad pid
	rr = httptest.NewRecorder()
	s.handleAffinity(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("affinity bad pid status=%d", rr.Code)
	}

	// Affinity bad JSON
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleAffinity(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("affinity bad json status=%d", rr.Code)
	}

	// Affinity happy
	body = `{"mask":3}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleAffinity(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("affinity status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleSuspendNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleSuspend(rr, paramCtx(req, map[string]string{"pid": "1"}))
	// No snapshot → ErrNotFound → 404.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleResumeBadPid(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleResume(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleResumeErrorPath(t *testing.T) {
	// Non-existent PID → seam returns "no threads" error → controllerError
	// → 500 (generic error branch).
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleResume(rr, paramCtx(req, map[string]string{"pid": "999999"}))
	if rr.Code == http.StatusOK {
		t.Fatalf("expected error for non-existent pid, got 200: %s", rr.Body.String())
	}
}

func TestHandlePriorityNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	body := `{"class":"normal"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handlePriority(rr, paramCtx(req, map[string]string{"pid": "1"}))
	// No snapshot → ErrNotFound → 404.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleAffinityNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	body := `{"mask":3}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleAffinity(rr, paramCtx(req, map[string]string{"pid": "1"}))
	// No snapshot → ErrNotFound → 404.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleLimitAndClear(t *testing.T) {
	pid, cleanup := pingSpawn(t, 8)
	defer cleanup()
	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: pid, Name: "ping.exe"})
	s, _ := fullTestServer(t, "", nil, store)
	pidStr := strconv.FormatUint(uint64(pid), 10)

	// Limit bad pid
	rr := httptest.NewRecorder()
	s.handleLimit(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad pid status=%d", rr.Code)
	}

	// Limit bad json
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("nope"))
	req.Header.Set("Content-Type", "application/json")
	s.handleLimit(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d", rr.Code)
	}

	// Limit happy
	body := `{"cpu_pct":50,"mem_bytes":0}`
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleLimit(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("limit status=%d body=%s", rr.Code, rr.Body.String())
	}

	// ListLimits should now show it
	rr = httptest.NewRecorder()
	s.handleListLimits(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list limits status=%d", rr.Code)
	}

	// ClearLimit happy
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/", nil)
	s.handleClearLimit(rr, paramCtx(req, map[string]string{"pid": pidStr}))
	if rr.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleLimitNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	body := `{"cpu_pct":50}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	s.handleLimit(rr, paramCtx(req, map[string]string{"pid": "1"}))
	// No snapshot → ErrNotFound → 404.
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleClearLimitBadPidAndMissing(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleClearLimit(rr, httptest.NewRequest(http.MethodDelete, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad pid status=%d", rr.Code)
	}

	store := storage.NewStore(60, 10)
	setStoreSnap(t, store, metrics.ProcessInfo{PID: 1, Name: "a"})
	s, _ = fullTestServer(t, "", nil, store)
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	s.handleClearLimit(rr, paramCtx(req, map[string]string{"pid": "1"}))
	if rr.Code == http.StatusOK {
		t.Fatalf("expected error clearing nonexistent limit; got %d", rr.Code)
	}
}

func TestHandleListLimitsEmpty(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleListLimits(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "[") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestControllerErrorAllBranches(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"protected", fmt.Errorf("wrap: %w", controller.ErrProtected), http.StatusForbidden},
		{"critical", fmt.Errorf("wrap: %w", controller.ErrCritical), http.StatusForbidden},
		{"self", fmt.Errorf("wrap: %w", controller.ErrSelf), http.StatusForbidden},
		{"confirm", fmt.Errorf("wrap: %w", controller.ErrConfirmNeeded), http.StatusConflict},
		{"notfound", fmt.Errorf("wrap: %w", controller.ErrNotFound), http.StatusNotFound},
		{"other", errors.New("other error"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rr := httptest.NewRecorder()
		s.controllerError(rr, c.err)
		if rr.Code != c.want {
			t.Errorf("%s: status=%d want %d", c.name, rr.Code, c.want)
		}
	}
}

// ===== ports / alerts =====

func TestHandlePortsNoSnapshot(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handlePorts(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandlePortsOK(t *testing.T) {
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), PortBindings: []metrics.PortBinding{{PID: 1}}})
	s, _ := fullTestServer(t, "", nil, store)
	rr := httptest.NewRecorder()
	s.handlePorts(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleConnectionsAllBranches(t *testing.T) {
	// No snapshot
	s, _ := fullTestServer(t, "", nil, nil)
	rr := httptest.NewRecorder()
	s.handleConnections(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	// Happy + filter
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp:    time.Now(),
		PortBindings: []metrics.PortBinding{{PID: 100}, {PID: 200}},
	})
	s, _ = fullTestServer(t, "", nil, store)
	rr = httptest.NewRecorder()
	s.handleConnections(rr, httptest.NewRequest(http.MethodGet, "/?pid=100", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	var got []metrics.PortBinding
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].PID != 100 {
		t.Fatalf("got=%+v", got)
	}

	// Bad pid
	rr = httptest.NewRecorder()
	s.handleConnections(rr, httptest.NewRequest(http.MethodGet, "/?pid=bad", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// No filter
	rr = httptest.NewRecorder()
	s.handleConnections(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleAlertsAndHistoryAndClear(t *testing.T) {
	store := storage.NewStore(60, 10)
	alerts := anomaly.NewAlertStore(64)
	alerts.Raise(anomaly.Alert{Type: "test", Severity: anomaly.SeverityInfo, PID: 1})
	alerts.Raise(anomaly.Alert{Type: "test", Severity: anomaly.SeverityInfo, PID: 2})
	s := buildServerWith(t, defaultCfg(), store, alerts, event.NewEmitter())

	rr := httptest.NewRecorder()
	s.handleAlerts(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "test") {
		t.Fatalf("body=%s", rr.Body.String())
	}

	rr = httptest.NewRecorder()
	s.handleAlertHistory(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	s.handleAlertsClear(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"removed":2`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestHandleAlertDismissBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	alerts := anomaly.NewAlertStore(64)
	alerts.Raise(anomaly.Alert{Type: "x", Severity: anomaly.SeverityInfo, PID: 5})
	s := buildServerWith(t, defaultCfg(), store, alerts, event.NewEmitter())

	// missing type
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertDismiss(rr, paramCtx(req, nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing type status=%d", rr.Code)
	}

	// bad pid
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertDismiss(rr, paramCtx(req, map[string]string{"type": "x", "pid": "abc"}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad pid status=%d", rr.Code)
	}

	// happy
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertDismiss(rr, paramCtx(req, map[string]string{"type": "x", "pid": "5"}))
	if rr.Code != http.StatusOK {
		t.Errorf("happy status=%d", rr.Code)
	}
}

func TestHandleAlertSnoozeBranches(t *testing.T) {
	store := storage.NewStore(60, 10)
	alerts := anomaly.NewAlertStore(64)
	s := buildServerWith(t, defaultCfg(), store, alerts, event.NewEmitter())

	// missing type
	rr := httptest.NewRecorder()
	s.handleAlertSnooze(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing type status=%d", rr.Code)
	}

	// bad pid
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertSnooze(rr, paramCtx(req, map[string]string{"type": "x", "pid": "abc"}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad pid status=%d", rr.Code)
	}

	// bad duration
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts/x/snooze?duration=junk", nil)
	s.handleAlertSnooze(rr, paramCtx(req, map[string]string{"type": "x"}))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad duration status=%d", rr.Code)
	}

	// happy default duration
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertSnooze(rr, paramCtx(req, map[string]string{"type": "x"}))
	if rr.Code != http.StatusOK {
		t.Errorf("happy default status=%d body=%s", rr.Code, rr.Body.String())
	}

	// happy with custom duration
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts/x/snooze?duration=10m", nil)
	s.handleAlertSnooze(rr, paramCtx(req, map[string]string{"type": "x"}))
	if rr.Code != http.StatusOK {
		t.Errorf("happy custom status=%d body=%s", rr.Code, rr.Body.String())
	}

	// happy with pid
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	s.handleAlertSnooze(rr, paramCtx(req, map[string]string{"type": "x", "pid": "7"}))
	if rr.Code != http.StatusOK {
		t.Errorf("happy pid status=%d", rr.Code)
	}
}

func TestAlertKeyFromRequestAllBranches(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	// missing type
	if _, ok := alertKeyFromRequest(rr, paramCtx(req, nil)); ok {
		t.Error("expected failure for missing type")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}

	// bad pid
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	if _, ok := alertKeyFromRequest(rr, paramCtx(req, map[string]string{"type": "x", "pid": "abc"})); ok {
		t.Error("expected failure for bad pid")
	}
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d", rr.Code)
	}

	// happy no pid
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	key, ok := alertKeyFromRequest(rr, paramCtx(req, map[string]string{"type": "x"}))
	if !ok || key != "x" {
		t.Errorf("no pid key=%q ok=%v", key, ok)
	}

	// happy with pid
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	key, ok = alertKeyFromRequest(rr, paramCtx(req, map[string]string{"type": "x", "pid": "5"}))
	if !ok || key != "x/5" {
		t.Errorf("with pid key=%q ok=%v", key, ok)
	}
}
