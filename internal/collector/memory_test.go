//go:build windows

package collector

import (
	"errors"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

func TestNewMemoryCollectorIsNotNil(t *testing.T) {
	m := NewMemoryCollector()
	if m == nil {
		t.Fatal("expected non-nil memory collector")
	}
}

func TestMemoryCollectorCollectReturnsPopulated(t *testing.T) {
	m := NewMemoryCollector()
	out := m.Collect()
	// On any sane test machine, total phys should be nonzero.
	if out.TotalPhys == 0 {
		t.Fatal("expected TotalPhys > 0")
	}
	if out.AvailPhys > out.TotalPhys {
		t.Fatalf("AvailPhys=%d > TotalPhys=%d", out.AvailPhys, out.TotalPhys)
	}
	if out.UsedPhys != out.TotalPhys-out.AvailPhys {
		t.Fatalf("UsedPhys=%d want %d", out.UsedPhys, out.TotalPhys-out.AvailPhys)
	}
	if out.UsedPercent < 0 || out.UsedPercent > 100 {
		t.Fatalf("UsedPercent=%v", out.UsedPercent)
	}
}

func TestMemoryCollectorCollectOnErrorReturnsZero(t *testing.T) {
	saved := globalMemoryStatusEx
	t.Cleanup(func() { globalMemoryStatusEx = saved })
	globalMemoryStatusEx = func() (*winapi.MEMORYSTATUSEX, error) {
		return nil, errors.New("memory status failed")
	}
	m := NewMemoryCollector()
	out := m.Collect()
	if out.TotalPhys != 0 || out.AvailPhys != 0 {
		t.Fatalf("expected zero MemoryMetrics on error, got %+v", out)
	}
}

func TestMemoryCollectorCollectOnNilReturnsZero(t *testing.T) {
	saved := globalMemoryStatusEx
	t.Cleanup(func() { globalMemoryStatusEx = saved })
	globalMemoryStatusEx = func() (*winapi.MEMORYSTATUSEX, error) {
		return nil, nil
	}
	m := NewMemoryCollector()
	out := m.Collect()
	if out.TotalPhys != 0 {
		t.Fatalf("expected zero MemoryMetrics on nil result, got %+v", out)
	}
}
