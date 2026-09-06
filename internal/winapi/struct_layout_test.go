//go:build windows && amd64

package winapi

import (
	"testing"
	"unsafe"
)

// TestStructMirrorLayouts pins every Go struct mirror that is passed to (or
// sliced from) Win32 API buffers to the x64 Windows ABI. The expected values
// were verified with offsetof()/sizeof() compiled against the real headers
// (Windows SDK netioapi.h + mingw-w64 tlhelp32/psapi/pdh/winnt, 2026-09) and
// cross-checked at runtime against NtQuerySystemInformation output.
//
// A failure here means a mirror field was added, removed, or reordered: the
// affected syscall will silently read or write the wrong bytes (see the
// parseIfRow2 offset bug for how that manifests). Reconcile the mirror with
// the documented Windows layout before updating these expectations.
//
// windows/arm64 is deliberately not covered: it needs its own
// compiler-verified expectation table.
func TestStructMirrorLayouts(t *testing.T) {
	type layoutCheck struct {
		name string
		got  uintptr
		want uintptr
	}
	run := func(t *testing.T, checks []layoutCheck) {
		t.Helper()
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s = %d, want %d (x64 ABI drift: struct mirror no longer matches the Windows layout)", c.name, c.got, c.want)
			}
		}
	}

	var ft FILETIME
	t.Run("FILETIME", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(ft), 8},
			{"LowDateTime", unsafe.Offsetof(ft.LowDateTime), 0},
			{"HighDateTime", unsafe.Offsetof(ft.HighDateTime), 4},
		})
	})

	var ms MEMORYSTATUSEX
	t.Run("MEMORYSTATUSEX", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(ms), 64},
			{"Length", unsafe.Offsetof(ms.Length), 0},
			{"MemoryLoad", unsafe.Offsetof(ms.MemoryLoad), 4},
			{"TotalPhys", unsafe.Offsetof(ms.TotalPhys), 8},
			{"AvailPhys", unsafe.Offsetof(ms.AvailPhys), 16},
			{"TotalPageFile", unsafe.Offsetof(ms.TotalPageFile), 24},
			{"AvailPageFile", unsafe.Offsetof(ms.AvailPageFile), 32},
			{"TotalVirtual", unsafe.Offsetof(ms.TotalVirtual), 40},
			{"AvailVirtual", unsafe.Offsetof(ms.AvailVirtual), 48},
			{"AvailExtendedVirtual", unsafe.Offsetof(ms.AvailExtendedVirtual), 56},
		})
	})

	var pe PROCESSENTRY32W
	t.Run("PROCESSENTRY32W", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(pe), 568},
			{"Size", unsafe.Offsetof(pe.Size), 0},
			{"Usage", unsafe.Offsetof(pe.Usage), 4},
			{"ProcessID", unsafe.Offsetof(pe.ProcessID), 8},
			{"DefaultHeapID", unsafe.Offsetof(pe.DefaultHeapID), 16},
			{"ModuleID", unsafe.Offsetof(pe.ModuleID), 24},
			{"Threads", unsafe.Offsetof(pe.Threads), 28},
			{"ParentProcessID", unsafe.Offsetof(pe.ParentProcessID), 32},
			{"PriClassBase", unsafe.Offsetof(pe.PriClassBase), 36},
			{"Flags", unsafe.Offsetof(pe.Flags), 40},
			{"ExeFile", unsafe.Offsetof(pe.ExeFile), 44},
		})
	})

	var te THREADENTRY32
	t.Run("THREADENTRY32", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(te), 28},
			{"Size", unsafe.Offsetof(te.Size), 0},
			{"Usage", unsafe.Offsetof(te.Usage), 4},
			{"ThreadID", unsafe.Offsetof(te.ThreadID), 8},
			{"OwnerProcessID", unsafe.Offsetof(te.OwnerProcessID), 12},
			{"BasePri", unsafe.Offsetof(te.BasePri), 16},
			{"DeltaPri", unsafe.Offsetof(te.DeltaPri), 20},
			{"Flags", unsafe.Offsetof(te.Flags), 24},
		})
	})

	var pmc PROCESS_MEMORY_COUNTERS_EX
	t.Run("PROCESS_MEMORY_COUNTERS_EX", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(pmc), 80},
			{"CB", unsafe.Offsetof(pmc.CB), 0},
			{"PageFaultCount", unsafe.Offsetof(pmc.PageFaultCount), 4},
			{"PeakWorkingSetSize", unsafe.Offsetof(pmc.PeakWorkingSetSize), 8},
			{"WorkingSetSize", unsafe.Offsetof(pmc.WorkingSetSize), 16},
			{"QuotaPeakPagedPoolUsage", unsafe.Offsetof(pmc.QuotaPeakPagedPoolUsage), 24},
			{"QuotaPagedPoolUsage", unsafe.Offsetof(pmc.QuotaPagedPoolUsage), 32},
			{"QuotaPeakNonPagedPoolUsage", unsafe.Offsetof(pmc.QuotaPeakNonPagedPoolUsage), 40},
			{"QuotaNonPagedPoolUsage", unsafe.Offsetof(pmc.QuotaNonPagedPoolUsage), 48},
			{"PagefileUsage", unsafe.Offsetof(pmc.PagefileUsage), 56},
			{"PeakPagefileUsage", unsafe.Offsetof(pmc.PeakPagefileUsage), 64},
			{"PrivateUsage", unsafe.Offsetof(pmc.PrivateUsage), 72},
		})
	})

	var io IO_COUNTERS
	t.Run("IO_COUNTERS", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(io), 48},
			{"ReadOperationCount", unsafe.Offsetof(io.ReadOperationCount), 0},
			{"WriteOperationCount", unsafe.Offsetof(io.WriteOperationCount), 8},
			{"OtherOperationCount", unsafe.Offsetof(io.OtherOperationCount), 16},
			{"ReadTransferCount", unsafe.Offsetof(io.ReadTransferCount), 24},
			{"WriteTransferCount", unsafe.Offsetof(io.WriteTransferCount), 32},
			{"OtherTransferCount", unsafe.Offsetof(io.OtherTransferCount), 40},
		})
	})

	var sppi SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION
	t.Run("SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(sppi), 48},
			{"IdleTime", unsafe.Offsetof(sppi.IdleTime), 0},
			{"KernelTime", unsafe.Offsetof(sppi.KernelTime), 8},
			{"UserTime", unsafe.Offsetof(sppi.UserTime), 16},
			{"Reserved1", unsafe.Offsetof(sppi.Reserved1), 24},
			{"Reserved2", unsafe.Offsetof(sppi.Reserved2), 40},
		})
	})

	var jbl JOBOBJECT_BASIC_LIMIT_INFORMATION
	t.Run("JOBOBJECT_BASIC_LIMIT_INFORMATION", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(jbl), 64},
			{"PerProcessUserTimeLimit", unsafe.Offsetof(jbl.PerProcessUserTimeLimit), 0},
			{"PerJobUserTimeLimit", unsafe.Offsetof(jbl.PerJobUserTimeLimit), 8},
			{"LimitFlags", unsafe.Offsetof(jbl.LimitFlags), 16},
			{"MinimumWorkingSetSize", unsafe.Offsetof(jbl.MinimumWorkingSetSize), 24},
			{"MaximumWorkingSetSize", unsafe.Offsetof(jbl.MaximumWorkingSetSize), 32},
			{"ActiveProcessLimit", unsafe.Offsetof(jbl.ActiveProcessLimit), 40},
			{"Affinity", unsafe.Offsetof(jbl.Affinity), 48},
			{"PriorityClass", unsafe.Offsetof(jbl.PriorityClass), 56},
			{"SchedulingClass", unsafe.Offsetof(jbl.SchedulingClass), 60},
		})
	})

	var jel JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	t.Run("JOBOBJECT_EXTENDED_LIMIT_INFORMATION", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(jel), 144},
			{"BasicLimitInformation", unsafe.Offsetof(jel.BasicLimitInformation), 0},
			{"IoInfo", unsafe.Offsetof(jel.IoInfo), 64},
			{"ProcessMemoryLimit", unsafe.Offsetof(jel.ProcessMemoryLimit), 112},
			{"JobMemoryLimit", unsafe.Offsetof(jel.JobMemoryLimit), 120},
			{"PeakProcessMemoryUsed", unsafe.Offsetof(jel.PeakProcessMemoryUsed), 128},
			{"PeakJobMemoryUsed", unsafe.Offsetof(jel.PeakJobMemoryUsed), 136},
		})
	})

	var jcr JOBOBJECT_CPU_RATE_CONTROL_INFORMATION
	t.Run("JOBOBJECT_CPU_RATE_CONTROL_INFORMATION", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(jcr), 8},
			{"ControlFlags", unsafe.Offsetof(jcr.ControlFlags), 0},
			{"CpuRate", unsafe.Offsetof(jcr.CpuRate), 4},
		})
	})

	var pcv pdhFmtCounterValueDouble
	t.Run("pdhFmtCounterValueDouble/PDH_FMT_COUNTERVALUE", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(pcv), 16},
			{"CStatus", unsafe.Offsetof(pcv.CStatus), 0},
			{"Value", unsafe.Offsetof(pcv.Value), 8},
		})
	})

	var pci pdhFmtCounterValueItemDouble
	t.Run("pdhFmtCounterValueItemDouble/PDH_FMT_COUNTERITEM_DOUBLE", func(t *testing.T) {
		run(t, []layoutCheck{
			{"sizeof", unsafe.Sizeof(pci), 24},
			{"Name", unsafe.Offsetof(pci.Name), 0},
			{"Value", unsafe.Offsetof(pci.Value), 8},
		})
	})
}
