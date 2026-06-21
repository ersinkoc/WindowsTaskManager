//go:build windows

package collector

import (
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// MemoryCollector samples physical and virtual memory state.
type MemoryCollector struct{}

// Function variable wrapping the Win32 call so tests can stub the failure path.
var globalMemoryStatusEx = winapi.GlobalMemoryStatusEx

func NewMemoryCollector() *MemoryCollector { return &MemoryCollector{} }

func (m *MemoryCollector) Collect() metrics.MemoryMetrics {
	ms, err := globalMemoryStatusEx()
	if err != nil || ms == nil {
		return metrics.MemoryMetrics{}
	}
	used := ms.TotalPhys - ms.AvailPhys
	var pct float64
	if ms.TotalPhys > 0 {
		pct = float64(used) / float64(ms.TotalPhys) * 100.0
	}
	commit := ms.TotalPageFile - ms.AvailPageFile
	return metrics.MemoryMetrics{
		TotalPhys:     ms.TotalPhys,
		AvailPhys:     ms.AvailPhys,
		UsedPhys:      used,
		UsedPercent:   pct,
		TotalPageFile: ms.TotalPageFile,
		AvailPageFile: ms.AvailPageFile,
		CommitCharge:  commit,
	}
}
