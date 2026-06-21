//go:build windows

package collector

import (
	"errors"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// restoreDiskStubs swaps the package-level disk-stub functions back to their
// real values. Each test that mutates them should call this in t.Cleanup.
func restoreDiskStubs(t *testing.T) {
	t.Helper()
	savedGLS := diskGetLogicalDriveStrings
	savedGDT := diskGetDriveType
	savedGDF := diskGetDiskFreeSpaceEx
	savedGVI := diskGetVolumeInformation
	savedOPQ := diskOpenPdhQuery
	savedAEC := diskAddEnglishCounter
	savedCQD := diskCollectQueryData
	savedGFC := diskGetFormattedCounter
	t.Cleanup(func() {
		diskGetLogicalDriveStrings = savedGLS
		diskGetDriveType = savedGDT
		diskGetDiskFreeSpaceEx = savedGDF
		diskGetVolumeInformation = savedGVI
		diskOpenPdhQuery = savedOPQ
		diskAddEnglishCounter = savedAEC
		diskCollectQueryData = savedCQD
		diskGetFormattedCounter = savedGFC
	})
}

func TestDiskCollectGetLogicalDriveStringsError(t *testing.T) {
	restoreDiskStubs(t)
	diskGetLogicalDriveStrings = func() ([]string, error) {
		return nil, errors.New("drive strings failed")
	}
	d := NewDiskCollector()
	out := d.Collect()
	if len(out.Drives) != 0 {
		t.Fatalf("expected empty drives on error, got %d", len(out.Drives))
	}
}

func TestDiskCollectSkipsNonFixedDrives(t *testing.T) {
	restoreDiskStubs(t)
	diskGetLogicalDriveStrings = func() ([]string, error) {
		return []string{`C:\`, `D:\`}, nil
	}
	// First call returns DRIVE_CDROM (0) — should be skipped.
	// Second call returns DRIVE_FIXED — should be included.
	diskGetDriveType = func(root string) uint32 {
		if root == `C:\` {
			return 0 // not in {FIXED,REMOVABLE,REMOTE}
		}
		return winapi.DRIVE_FIXED
	}
	diskGetDiskFreeSpaceEx = func(path string) (uint64, uint64, uint64, error) {
		return 50, 100, 50, nil
	}
	diskGetVolumeInformation = func(root string) (string, string) {
		return "Label", "NTFS"
	}
	// Force perf init to fail so we exercise the early-return in samplePerfCounters.
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) {
		return 0, errors.New("no pdh")
	}
	d := NewDiskCollector()
	out := d.Collect()
	if len(out.Drives) != 1 {
		t.Fatalf("expected 1 drive, got %d (%+v)", len(out.Drives), out.Drives)
	}
	if out.Drives[0].Letter != "D:" {
		t.Fatalf("expected D:, got %q", out.Drives[0].Letter)
	}
}

func TestDiskCollectSkipsFreeSpaceErrorAndZeroTotal(t *testing.T) {
	restoreDiskStubs(t)
	diskGetLogicalDriveStrings = func() ([]string, error) {
		return []string{`E:\`, `F:\`, `G:\`}, nil
	}
	diskGetDriveType = func(root string) uint32 { return winapi.DRIVE_FIXED }
	calls := 0
	diskGetDiskFreeSpaceEx = func(path string) (uint64, uint64, uint64, error) {
		calls++
		switch path {
		case `E:\`:
			return 0, 0, 0, errors.New("free space failed")
		case `F:\`:
			// total == 0
			return 1, 0, 0, nil
		default:
			return 50, 100, 50, nil
		}
	}
	diskGetVolumeInformation = func(root string) (string, string) {
		return "Label", "NTFS"
	}
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return 0, errors.New("no pdh") }
	d := NewDiskCollector()
	out := d.Collect()
	if calls != 3 {
		t.Fatalf("expected 3 free-space calls, got %d", calls)
	}
	if len(out.Drives) != 1 {
		t.Fatalf("expected 1 drive after skips, got %d", len(out.Drives))
	}
	if out.Drives[0].Letter != "G:" {
		t.Fatalf("expected G:, got %q", out.Drives[0].Letter)
	}
}

func TestDiskSamplePerfCountersWhenInitFails(t *testing.T) {
	restoreDiskStubs(t)
	// Make every PDH call fail so newDiskPerfCounters returns an error.
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) {
		return 0, errors.New("no pdh")
	}
	diskAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return 0, errors.New("counter failed")
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return errors.New("collect failed") }
	diskGetFormattedCounter = func(winapi.PdhCounter) (map[string]float64, error) {
		return nil, errors.New("formatted failed")
	}
	// Provide a drive so the loop runs.
	diskGetLogicalDriveStrings = func() ([]string, error) { return []string{`C:\`}, nil }
	diskGetDriveType = func(root string) uint32 { return winapi.DRIVE_FIXED }
	diskGetDiskFreeSpaceEx = func(path string) (uint64, uint64, uint64, error) {
		return 50, 100, 50, nil
	}
	diskGetVolumeInformation = func(root string) (string, string) { return "Label", "NTFS" }

	d := NewDiskCollector()
	// First call: d.perf starts nil, init fails, returns empty maps.
	out := d.Collect()
	if len(out.Drives) != 1 {
		t.Fatalf("expected 1 drive, got %d", len(out.Drives))
	}
	if d.perf != nil {
		t.Fatal("expected perf to remain nil after init failure")
	}
}

func TestDiskSamplePerfCountersCollectQueryDataFails(t *testing.T) {
	restoreDiskStubs(t)
	// Initialize perf successfully...
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	diskAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	// The first call (inside newDiskPerfCounters) must succeed so perf is
	// initialised; the second call (inside samplePerfCounters) fails to
	// exercise the post-init error branch.
	collectCalls := 0
	diskCollectQueryData = func(winapi.PdhQuery) error {
		collectCalls++
		if collectCalls == 1 {
			return nil
		}
		return errors.New("collect failed")
	}
	diskGetFormattedCounter = func(winapi.PdhCounter) (map[string]float64, error) {
		return map[string]float64{"c:": 100}, nil
	}
	diskGetLogicalDriveStrings = func() ([]string, error) { return []string{`C:\`}, nil }
	diskGetDriveType = func(root string) uint32 { return winapi.DRIVE_FIXED }
	diskGetDiskFreeSpaceEx = func(path string) (uint64, uint64, uint64, error) {
		return 50, 100, 50, nil
	}
	diskGetVolumeInformation = func(root string) (string, string) { return "Label", "NTFS" }

	d := NewDiskCollector()
	_ = d.Collect()
	if d.perf == nil {
		t.Fatal("expected perf to be initialized on first call")
	}
	// Now the second call should hit the diskCollectQueryData error branch.
	out := d.Collect()
	if len(out.Drives) != 1 {
		t.Fatalf("expected 1 drive, got %d", len(out.Drives))
	}
	// Read/Write counters should be zero since the collect errored.
	if out.Drives[0].ReadBPS != 0 {
		t.Fatalf("expected ReadBPS=0 on collect error, got %d", out.Drives[0].ReadBPS)
	}
}

func TestDiskSamplePerfCountersFormattedError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	diskAddEnglishCounter = func(winapi.PdhQuery, string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return nil }
	calls := 0
	diskGetFormattedCounter = func(winapi.PdhCounter) (map[string]float64, error) {
		calls++
		// First call (initial sample) succeeds; later ones fail to exercise the
		// "any formatted error" branch.
		if calls == 1 {
			return map[string]float64{}, nil
		}
		return nil, errors.New("formatted failed")
	}
	diskGetLogicalDriveStrings = func() ([]string, error) { return []string{`C:\`}, nil }
	diskGetDriveType = func(root string) uint32 { return winapi.DRIVE_FIXED }
	diskGetDiskFreeSpaceEx = func(path string) (uint64, uint64, uint64, error) {
		return 50, 100, 50, nil
	}
	diskGetVolumeInformation = func(root string) (string, string) { return "Label", "NTFS" }

	d := NewDiskCollector()
	_ = d.Collect() // initial sample succeeds
	out := d.Collect()
	if len(out.Drives) != 1 {
		t.Fatalf("expected 1 drive, got %d", len(out.Drives))
	}
	if out.Drives[0].ReadBPS != 0 || out.Drives[0].WriteBPS != 0 {
		t.Fatalf("expected zero counters on formatted error, got read=%d write=%d",
			out.Drives[0].ReadBPS, out.Drives[0].WriteBPS)
	}
}

func TestNewDiskPerfCountersOpenPdhQueryError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) {
		return 0, errors.New("open failed")
	}
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when OpenPdhQuery fails")
	}
}

func TestNewDiskPerfCountersReadBPSAddError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	calls := 0
	diskAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("read counter failed")
		}
		return winapi.PdhCounter(1), nil
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return nil }
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when read counter add fails")
	}
}

func TestNewDiskPerfCountersWriteBPSAddError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	calls := 0
	diskAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		calls++
		if calls == 2 {
			return 0, errors.New("write counter failed")
		}
		return winapi.PdhCounter(1), nil
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return nil }
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when write counter add fails")
	}
}

func TestNewDiskPerfCountersReadIOPSAddError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	calls := 0
	diskAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		calls++
		if calls == 3 {
			return 0, errors.New("read-iops counter failed")
		}
		return winapi.PdhCounter(1), nil
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return nil }
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when read-iops counter add fails")
	}
}

func TestNewDiskPerfCountersWriteIOPSAddError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	calls := 0
	diskAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		calls++
		if calls == 4 {
			return 0, errors.New("write-iops counter failed")
		}
		return winapi.PdhCounter(1), nil
	}
	diskCollectQueryData = func(winapi.PdhQuery) error { return nil }
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when write-iops counter add fails")
	}
}

func TestNewDiskPerfCountersFinalCollectError(t *testing.T) {
	restoreDiskStubs(t)
	diskOpenPdhQuery = func() (winapi.PdhQuery, error) { return winapi.PdhQuery(1), nil }
	diskAddEnglishCounter = func(_ winapi.PdhQuery, path string) (winapi.PdhCounter, error) {
		return winapi.PdhCounter(1), nil
	}
	// The final CollectQueryData call in newDiskPerfCounters fails.
	diskCollectQueryData = func(winapi.PdhQuery) error {
		return errors.New("final collect failed")
	}
	_, err := newDiskPerfCounters()
	if err == nil {
		t.Fatal("expected error when final collect query fails")
	}
}

func TestNewDiskCollectorIsNotNil(t *testing.T) {
	d := NewDiskCollector()
	if d == nil {
		t.Fatal("expected non-nil disk collector")
	}
	if d.perf != nil {
		t.Fatal("perf should start nil until first Collect")
	}
}

func TestDiskCollectorCollectSecondSampleUsesPerf(t *testing.T) {
	d := NewDiskCollector()
	_ = d.Collect()
	// After the first call, perf should be initialized (or nil if PDH unavailable).
	if d.perf != nil {
		// Second call exercises the cached-perf path.
		out := d.Collect()
		_ = out
	}
}

func TestNormalizeCounterMapEmpty(t *testing.T) {
	if got := normalizeCounterMap(map[string]float64{}); len(got) != 0 {
		t.Fatalf("normalizeCounterMap empty len=%d", len(got))
	}
	if got := normalizeCounterMap(nil); len(got) != 0 {
		t.Fatalf("normalizeCounterMap nil len=%d", len(got))
	}
}

func TestNormalizeCounterMapSkipsBlankKeys(t *testing.T) {
	got := normalizeCounterMap(map[string]float64{
		"":    1.0,
		"   ": 2.0,
		"c:":  3.0,
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got["c:"] != 3.0 {
		t.Fatalf("c:=%v", got["c:"])
	}
}

func TestCounterUint64Zero(t *testing.T) {
	if got := counterUint64(0); got != 0 {
		t.Fatalf("counterUint64(0)=%d", got)
	}
}
