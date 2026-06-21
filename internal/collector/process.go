//go:build windows

package collector

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// procPrev tracks the prior CPU time slice for a single PID so the next
// sample can compute a percentage.
type procPrev struct {
	kernelTicks  uint64
	userTicks    uint64
	ioReadBytes  uint64
	ioWriteBytes uint64
	ioReadOps    uint64
	ioWriteOps   uint64
	sampleTime   time.Time
}

// ProcessCollector enumerates running processes and computes per-process CPU%.
type ProcessCollector struct {
	mu         sync.Mutex
	prev       map[uint32]procPrev
	maxResults int
	numLogical int
}

func NewProcessCollector(maxResults int) *ProcessCollector {
	if maxResults <= 0 {
		maxResults = 2000
	}
	return &ProcessCollector{
		prev:       make(map[uint32]procPrev),
		maxResults: maxResults,
		numLogical: runtime.NumCPU(),
	}
}

// Function variables wrapping the Win32 calls so tests can stub the
// failure paths without invoking real OS APIs.
var (
	createToolhelp32Snapshot  = winapi.CreateToolhelp32Snapshot
	process32First            = winapi.Process32First
	process32Next             = winapi.Process32Next
	closeHandleSafe           = winapi.CloseHandleSafe
	openProcessHandle         = winapi.OpenProcessHandle
	getProcessMemoryInfo      = winapi.GetProcessMemoryInfo
	getProcessIoCounters      = winapi.GetProcessIoCounters
	getProcessTimes           = winapi.GetProcessTimes
	queryFullProcessImageName = winapi.QueryFullProcessImageName
	isProcessCritical         = winapi.IsProcessCritical
	getPriorityClass          = winapi.GetPriorityClass
)

// Collect returns the current process list. It limits results to maxResults
// after sorting by working set descending so we always retain the heaviest
// processes when there are too many.
func (pc *ProcessCollector) Collect() []metrics.ProcessInfo {
	snap, err := createToolhelp32Snapshot(winapi.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer closeHandleSafe(snap)

	var entry winapi.PROCESSENTRY32W
	if err := process32First(snap, &entry); err != nil {
		return nil
	}

	now := time.Now()
	results := make([]metrics.ProcessInfo, 0, 256)
	live := make(map[uint32]procPrev, 256)

	for {
		info := pc.collectOne(entry, now, live)
		results = append(results, info)
		if err := process32Next(snap, &entry); err != nil {
			break
		}
	}

	pc.mu.Lock()
	pc.prev = live
	pc.mu.Unlock()

	if len(results) > pc.maxResults {
		// Trim least-interesting (smallest working set) entries.
		results = trimByWorkingSet(results, pc.maxResults)
	}

	return results
}

const accessLimited = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_VM_READ

func (pc *ProcessCollector) collectOne(entry winapi.PROCESSENTRY32W, now time.Time, live map[uint32]procPrev) metrics.ProcessInfo {
	name := windows.UTF16ToString(entry.ExeFile[:])
	info := metrics.ProcessInfo{
		PID:         entry.ProcessID,
		ParentPID:   entry.ParentProcessID,
		Name:        name,
		ThreadCount: entry.Threads,
		Status:      "running",
	}

	if entry.ProcessID == 0 || entry.ProcessID == 4 {
		info.IsCritical = true
		live[entry.ProcessID] = procPrev{sampleTime: now}
		return info
	}

	h, err := openProcessHandle(accessLimited, entry.ProcessID)
	if err != nil {
		// Many system processes deny query — return what we have.
		live[entry.ProcessID] = procPrev{sampleTime: now}
		return info
	}
	defer closeHandleSafe(h)

	if mem, err := getProcessMemoryInfo(h); err == nil && mem != nil {
		info.WorkingSet = uint64(mem.WorkingSetSize)
		info.PrivateBytes = uint64(mem.PrivateUsage)
		info.PageFaults = mem.PageFaultCount
	}

	var currReadBytes, currWriteBytes, currReadOps, currWriteOps uint64
	if io, err := getProcessIoCounters(h); err == nil && io != nil {
		currReadBytes = io.ReadTransferCount
		currWriteBytes = io.WriteTransferCount
		currReadOps = io.ReadOperationCount
		currWriteOps = io.WriteOperationCount
		pc.mu.Lock()
		prev, hasPrev := pc.prev[entry.ProcessID]
		pc.mu.Unlock()
		if hasPrev {
			info.IOReadBytes = saturatingSub(currReadBytes, prev.ioReadBytes)
			info.IOWriteBytes = saturatingSub(currWriteBytes, prev.ioWriteBytes)
			info.IOReadOps = saturatingSub(currReadOps, prev.ioReadOps)
			info.IOWriteOps = saturatingSub(currWriteOps, prev.ioWriteOps)
		}
	}

	create, _, kernel, user, terr := getProcessTimes(h)
	if terr == nil {
		info.CreateTime = winapi.FileTimeToUnix(create)
		kt := kernel.Ticks()
		ut := user.Ticks()

		pc.mu.Lock()
		prev, hasPrev := pc.prev[entry.ProcessID]
		pc.mu.Unlock()

		if hasPrev {
			elapsed := now.Sub(prev.sampleTime).Seconds()
			if elapsed > 0 {
				deltaTicks := float64((kt - prev.kernelTicks) + (ut - prev.userTicks))
				cpuSeconds := deltaTicks / 1e7 // 100ns -> seconds
				cpuPercent := (cpuSeconds / elapsed) * 100.0 / float64(pc.numLogical)
				// Clamp to [0, 100]. The lower bound is defensive; with
				// uint64 ticks negative deltas wrap to a huge positive
				// value, so the > 100 branch catches them. The < 0 branch
				// is preserved as a safety net.
				if cpuPercent < 0 {
					cpuPercent = 0
				}
				if cpuPercent > 100 {
					cpuPercent = 100
				}
				info.CPUPercent = cpuPercent
			}
		}
		live[entry.ProcessID] = procPrev{
			kernelTicks:  kt,
			userTicks:    ut,
			ioReadBytes:  currReadBytes,
			ioWriteBytes: currWriteBytes,
			ioReadOps:    currReadOps,
			ioWriteOps:   currWriteOps,
			sampleTime:   now,
		}
	} else {
		live[entry.ProcessID] = procPrev{sampleTime: now}
	}

	if path, err := queryFullProcessImageName(h); err == nil {
		info.ExePath = path
		// Prefer the basename from path if PROCESSENTRY32 had a truncated name.
		if base := filepath.Base(path); base != "" && !strings.EqualFold(base, info.Name) {
			info.Name = base
		}
	}

	if crit, err := isProcessCritical(h); err == nil && crit {
		info.IsCritical = true
	}

	if pri, err := getPriorityClass(h); err == nil {
		info.PriorityClass = pri
	}

	return info
}

func trimByWorkingSet(list []metrics.ProcessInfo, n int) []metrics.ProcessInfo {
	if len(list) <= n {
		return list
	}
	// Selection-based trim: sort descending by WorkingSet then truncate.
	sortByWorkingSetDesc(list)
	return list[:n]
}

func sortByWorkingSetDesc(a []metrics.ProcessInfo) {
	sort.Slice(a, func(i, j int) bool { return a[i].WorkingSet > a[j].WorkingSet })
}
