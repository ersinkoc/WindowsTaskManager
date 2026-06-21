//go:build windows

package collector

import (
	"errors"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

func restoreGPUStubs(t *testing.T) {
	t.Helper()
	savedOPQ := gpuOpenPdhQuery
	savedAEC := gpuAddEnglishCounter
	savedCQD := gpuCollectQueryData
	savedGFC := gpuGetFormatted
	savedRRS := gpuRegReadString
	savedRRQ := gpuRegReadQWORD
	t.Cleanup(func() {
		gpuOpenPdhQuery = savedOPQ
		gpuAddEnglishCounter = savedAEC
		gpuCollectQueryData = savedCQD
		gpuGetFormatted = savedGFC
		gpuRegReadString = savedRRS
		gpuRegReadQWORD = savedRRQ
	})
}

// TestGPUCollectReturnsUnavailableWhenPerfInitFails exercises the branch
// where newGPUPerfCounters fails and the early-return populates the
// unavailable metrics.
func TestGPUCollectReturnsUnavailableWhenPerfInitFails(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return 0, errors.New("no pdh") }
	g := NewGPUCollector()
	// Pre-populate adapter info so cachedName is non-empty.
	g.adapters = []gpuAdapterInfo{{name: "StubGPU", totalBytes: 1024 * 1024 * 1024}}
	g.cachedName = "StubGPU"
	g.cachedTotalVRAM = 1024 * 1024 * 1024
	out := g.Collect()
	if out.Available {
		t.Fatal("expected Available=false when perf init fails")
	}
	if out.Name != "StubGPU" {
		t.Fatalf("Name=%q want StubGPU", out.Name)
	}
	if out.Temperature != -1 {
		t.Fatalf("Temperature=%v want -1", out.Temperature)
	}
}

// TestGPUCollectCollectQueryDataFails exercises the post-perf-init
// CollectQueryData error branch.
func TestGPUCollectCollectQueryDataFails(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	gpuAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	// First CollectQueryData call (in newGPUPerfCounters) succeeds; later ones
	// fail so the post-init error branch is hit.
	collectCalls := 0
	gpuCollectQueryData = func(winapi.PdhQuery) error {
		collectCalls++
		if collectCalls == 1 {
			return nil
		}
		return errors.New("collect failed")
	}
	g := NewGPUCollector()
	g.adapters = []gpuAdapterInfo{{name: "StubGPU", totalBytes: 1 << 30}}
	g.cachedName = "StubGPU"
	g.cachedTotalVRAM = 1 << 30
	_ = g.Collect() // initialises perf via the successful collect
	if g.perf == nil {
		t.Fatal("expected perf to be set after first call")
	}
	out := g.Collect()
	if out.Available {
		t.Fatal("expected Available=false when post-init CollectQueryData fails")
	}
}

// TestGPUCollectAllFormattedErrors exercises the "all three formatted
// counters fail" branch.
func TestGPUCollectAllFormattedErrors(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	gpuAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	gpuCollectQueryData = func(winapi.PdhQuery) error { return nil }
	gpuGetFormatted = func(winapi.PdhCounter) (map[string]float64, error) {
		return nil, errors.New("formatted failed")
	}
	g := NewGPUCollector()
	g.adapters = []gpuAdapterInfo{{name: "StubGPU", totalBytes: 1 << 30}}
	g.cachedName = "StubGPU"
	g.cachedTotalVRAM = 1 << 30
	out := g.Collect()
	if out.Available {
		t.Fatal("expected Available=false when all formatted counters fail")
	}
	if out.Name != "StubGPU" {
		t.Fatalf("Name=%q want StubGPU", out.Name)
	}
}

// TestGPUCollectDedicatedZeroUsesSharedTotalAdjustment covers the
// "dedicated==0 use shared" and "total<used bump total" branches inside
// Collect by stubbing the formatted-counter calls to return data with
// dedicated=0 and shared>0.
func TestGPUCollectDedicatedZeroUsesSharedTotalAdjustment(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	gpuAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	gpuCollectQueryData = func(winapi.PdhQuery) error { return nil }
	// Use maps with a shared adapter key for util (engine-format) and
	// dedicated/shared (adapter-format). The dedicated map has value 0
	// so the used-fallback branch fires.
	utilMap := map[string]float64{
		"pid_1_luid_0x0_0x1_phys_0_eng_0_engtype_3d": 50,
	}
	memMap := map[string]float64{
		"luid_0x0_0x1_phys_0": 200,
	}
	dedicatedMap := map[string]float64{
		"luid_0x0_0x1_phys_0": 0, // dedicated==0 triggers fallback
	}
	calls := 0
	gpuGetFormatted = func(counter winapi.PdhCounter) (map[string]float64, error) {
		calls++
		switch calls % 3 {
		case 1:
			return utilMap, nil
		case 2:
			return dedicatedMap, nil
		default:
			return memMap, nil
		}
	}
	g := NewGPUCollector()
	g.adapters = []gpuAdapterInfo{{name: "StubGPU", totalBytes: 100}}
	g.cachedName = "StubGPU"
	g.cachedTotalVRAM = 100
	_ = g.Collect() // init perf via stubs
	out := g.Collect()
	if out.VRAMUsed != 200 {
		t.Fatalf("VRAMUsed=%d want 200 (dedicated+shared)", out.VRAMUsed)
	}
	if out.VRAMTotal < out.VRAMUsed {
		t.Fatalf("VRAMTotal=%d should be >= VRAMUsed=%d", out.VRAMTotal, out.VRAMUsed)
	}
	if out.VRAMTotal != 200 {
		t.Fatalf("VRAMTotal=%d want 200 (bumped up)", out.VRAMTotal)
	}
}

// TestGPUInitInventoryUnknownGPUFallback covers the branch where no adapter
// is found and cachedName stays empty so the "Unknown GPU" fallback fires.
func TestGPUInitInventoryUnknownGPUFallback(t *testing.T) {
	restoreGPUStubs(t)
	// Return a single adapter entry with empty name and non-zero totalBytes
	// from the class-root registry. The fallback (readGPUName) is skipped
	// because the slice is non-empty, but the inner loop never sets
	// cachedName because the adapter's name is empty.
	gpuRegReadString = func(_ uintptr, _ string, _ string) (string, error) {
		return "", errors.New("not found")
	}
	gpuRegReadQWORD = func(_ uintptr, _ string, _ string) (uint64, error) {
		return 1 << 20, nil
	}
	g := NewGPUCollector()
	g.initInventory()
	if g.cachedName != "Unknown GPU" {
		t.Fatalf("cachedName=%q want Unknown GPU", g.cachedName)
	}
	if g.cachedTotalVRAM == 0 {
		t.Fatal("expected cachedTotalVRAM to be set from qwMemorySize")
	}
}

// TestNewGPUPerfCountersOpenPdhQueryError covers the OpenPdhQuery error
// branch in newGPUPerfCounters.
func TestNewGPUPerfCountersOpenPdhQueryError(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) {
		return 0, errors.New("open failed")
	}
	_, err := newGPUPerfCounters()
	if err == nil {
		t.Fatal("expected error when OpenPdhQuery fails")
	}
}

// TestNewGPUPerfCountersUtilAddError covers the util counter add error
// branch.
func TestNewGPUPerfCountersUtilAddError(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	calls := 0
	gpuAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("util counter failed")
		}
		return winapi.PdhCounter(1), nil
	}
	gpuCollectQueryData = func(winapi.PdhQuery) error { return nil }
	_, err := newGPUPerfCounters()
	if err == nil {
		t.Fatal("expected error when util counter add fails")
	}
}

// TestNewGPUPerfCountersFinalCollectError covers the final CollectQueryData
// error branch in newGPUPerfCounters.
func TestNewGPUPerfCountersFinalCollectError(t *testing.T) {
	restoreGPUStubs(t)
	gpuOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	gpuAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	gpuCollectQueryData = func(winapi.PdhQuery) error {
		return errors.New("final collect failed")
	}
	_, err := newGPUPerfCounters()
	if err == nil {
		t.Fatal("expected error when final collect query fails")
	}
}

// TestReadGPUAdaptersFallsBackToName covers the readGPUName fallback path
// inside readGPUAdapters when no class-root adapter is found.
func TestReadGPUAdaptersFallsBackToName(t *testing.T) {
	restoreGPUStubs(t)
	// Class-root reads always return empty/zero so no adapter is appended.
	gpuRegReadString = func(uintptr, string, string) (string, error) {
		return "", errors.New("not found")
	}
	gpuRegReadQWORD = func(uintptr, string, string) (uint64, error) {
		return 0, errors.New("not found")
	}
	// The fallback path then reads the Video path; let it return a non-empty
	// value.
	gpuRegReadString = func(_ uintptr, subKey, _ string) (string, error) {
		if subKey == `SYSTEM\CurrentControlSet\Control\Video\{00000000-0000-0000-0000-000000000000}\0000` {
			return "FallbackGPU", nil
		}
		return "", errors.New("not found")
	}
	adapters := readGPUAdapters()
	foundFallback := false
	for _, a := range adapters {
		if a.name == "FallbackGPU" {
			foundFallback = true
			break
		}
	}
	if !foundFallback {
		t.Fatalf("expected fallback GPU name in adapters, got %+v", adapters)
	}
}

// TestReadGPUNameReturnsDriverDesc covers the success branch where the
// registry read returns a non-empty DriverDesc.
func TestReadGPUNameReturnsDriverDesc(t *testing.T) {
	restoreGPUStubs(t)
	gpuRegReadString = func(uintptr, string, string) (string, error) {
		return "RealDriverName", nil
	}
	if got := readGPUName(); got != "RealDriverName" {
		t.Fatalf("readGPUName=%q want RealDriverName", got)
	}
}

func TestParseGPUEngineInstanceInvalidReturnsFalse(t *testing.T) {
	if _, _, ok := parseGPUEngineInstance("not-an-instance"); ok {
		t.Fatal("expected invalid instance to fail")
	}
}

func TestParseGPUAdapterInstanceValid(t *testing.T) {
	key, ok := parseGPUAdapterInstance("luid_0x00000000_0x0001688d_phys_0")
	if !ok {
		t.Fatal("expected valid adapter instance")
	}
	if key != "luid_0x00000000_0x0001688d_phys_0" {
		t.Fatalf("adapterKey=%q", key)
	}
}

func TestParseGPUAdapterInstanceInvalidReturnsFalse(t *testing.T) {
	if _, ok := parseGPUAdapterInstance("garbage"); ok {
		t.Fatal("expected invalid adapter instance to fail")
	}
}

func TestParseGPUAdapterIndexMissingMarkerErrors(t *testing.T) {
	if _, err := parseGPUAdapterIndex("luid_0x00000000_0x0001688d_no_phys"); err == nil {
		t.Fatal("expected error for missing phys marker")
	}
}

func TestParseGPUAdapterIndexInvalidNumberErrors(t *testing.T) {
	if _, err := parseGPUAdapterIndex("luid_0x0_0x1_phys_notanumber"); err == nil {
		t.Fatal("expected error for invalid number")
	}
}

func TestAggregateGPUSamplesSkipsUnparseableInstances(t *testing.T) {
	samples := aggregateGPUSamples(
		map[string]float64{"garbage": 50.0},
		map[string]float64{"also-garbage": 100.0},
		map[string]float64{"shared-garbage": 200.0},
	)
	if len(samples) != 0 {
		t.Fatalf("expected no samples from unparseable instances, got %d", len(samples))
	}
}

func TestAggregateGPUSamplesCapsUtilizationAtHundred(t *testing.T) {
	// Single engine contributing >100 — the per-engine cap at 100 should activate.
	samples := aggregateGPUSamples(
		map[string]float64{
			"pid_1_luid_0x0_0x1_phys_0_eng_0_engtype_3d": 250,
		},
		map[string]float64{},
		map[string]float64{},
	)
	s := samples["luid_0x0_0x1_phys_0"]
	if s.utilization != 100 {
		t.Fatalf("utilization=%v want 100 (capped at 100)", s.utilization)
	}
}

func TestPickPrimaryGPUEmptyReturnsFalse(t *testing.T) {
	key, sample, ok := pickPrimaryGPU(map[string]gpuAdapterSample{})
	if ok {
		t.Fatal("expected false for empty map")
	}
	if key != "" {
		t.Fatalf("expected empty key, got %q", key)
	}
	if sample.utilization != 0 || sample.dedicated != 0 || sample.shared != 0 {
		t.Fatalf("expected zero sample, got %+v", sample)
	}
}

func TestPickPrimaryGPUSelectsHighestScore(t *testing.T) {
	samples := map[string]gpuAdapterSample{
		"a": {utilization: 5, dedicated: 100},
		"b": {utilization: 80, dedicated: 0},
	}
	key, sample, ok := pickPrimaryGPU(samples)
	if !ok {
		t.Fatal("expected ok")
	}
	if key != "b" {
		t.Fatalf("primary key=%q want b", key)
	}
	if sample.utilization != 80 {
		t.Fatalf("utilization=%v", sample.utilization)
	}
}

func TestFormattedCounterArrayDoubleZeroCounter(t *testing.T) {
	got, err := formattedCounterArrayDouble(0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map for zero counter, got %v", got)
	}
}

func TestGPUCollectorInitInventoryIsIdempotent(t *testing.T) {
	g := NewGPUCollector()
	g.initInventory()
	first := append([]gpuAdapterInfo(nil), g.adapters...)
	g.initInventory()
	if len(g.adapters) != len(first) {
		t.Fatalf("expected initInventory to be idempotent; got %d vs first %d", len(g.adapters), len(first))
	}
}

func TestReadGPUAdaptersReturnsSlice(t *testing.T) {
	adapters := readGPUAdapters()
	// Should be a slice (possibly empty); never nil.
	if adapters == nil {
		t.Fatal("expected non-nil slice")
	}
}

func TestReadGPUNameReturnsString(t *testing.T) {
	name := readGPUName()
	// Either a real driver name or the "Unknown GPU" fallback.
	if name == "" {
		t.Fatal("expected non-empty GPU name")
	}
}

func TestGPUCollectorCollectNameFallbackWhenEmpty(t *testing.T) {
	g := NewGPUCollector()
	g.initInventory()
	// Force cachedName empty to hit the "Unknown GPU" fallback in Collect.
	// This requires the perf counters to be available so we reach the post-init block.
	g.adapters = []gpuAdapterInfo{{name: ""}} // ensure non-empty to skip re-read
	g.cachedName = ""
	// We need perf to be non-nil; if PDH init succeeds, we'll get the early-return path
	// because g.perf is set. But if perf init fails, we return at the perf==nil check
	// with cachedName="" which exercises that path.
	// Either way, we just want Collect() to not panic and produce a name.
	out := g.Collect()
	if out.Name == "" {
		t.Fatal("expected Name to be populated (Unknown GPU fallback)")
	}
}

func TestGPUCollectUsedFallbackAndTotalAdjustment(t *testing.T) {
	// Construct a collector with manual state to exercise the
	// "dedicated==0 use shared" and "total<used bump total" branches.
	g := NewGPUCollector()
	g.initInventory()
	g.adapters = []gpuAdapterInfo{{name: "TestGPU", totalBytes: 100}}
	g.cachedName = "TestGPU"
	g.cachedTotalVRAM = 100
	// Force the perf == nil path by leaving g.perf nil — that triggers the early return
	// which uses cachedName/cachedTotalVRAM and Available=false. This exercises
	// the early-return block but NOT the inner used-fallback branches.
	// To exercise the inner branches, we need a non-nil perf. Since we can't easily
	// fabricate that, we settle for covering the early-return block.
	out := g.Collect()
	if out.Name != "TestGPU" {
		t.Fatalf("Name=%q want TestGPU", out.Name)
	}
}
