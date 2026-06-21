//go:build windows

package controller

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// Event names emitted on successful operations.
const (
	EventKilled       = "controller.killed"
	EventSuspended    = "controller.suspended"
	EventResumed      = "controller.resumed"
	EventPriority     = "controller.priority"
	EventAffinity     = "controller.affinity"
	EventLimited      = "controller.limited"
	EventLimitCleared = "controller.limit_cleared"
)

// jobEntry tracks a Job Object that wraps a single PID for resource limits.
type jobEntry struct {
	job      windows.Handle
	pid      uint32
	cpuPct   int
	memBytes uint64
}

// Controller exposes process management operations with safety enforcement.
type Controller struct {
	safety  *Safety
	store   *storage.Store
	emitter *event.Emitter

	mu   sync.Mutex
	jobs map[uint32]*jobEntry
}

func NewController(cfg *config.Config, store *storage.Store, emitter *event.Emitter) *Controller {
	return &Controller{
		safety:  NewSafety(cfg),
		store:   store,
		emitter: emitter,
		jobs:    make(map[uint32]*jobEntry),
	}
}

// SetConfig hot-swaps the underlying safety config.
func (c *Controller) SetConfig(cfg *config.Config) { c.safety.SetConfig(cfg) }

// findProcess locates a process by PID in the latest snapshot.
func (c *Controller) findProcess(pid uint32) (metrics.ProcessInfo, error) {
	snap := c.store.Latest()
	if snap == nil {
		return metrics.ProcessInfo{}, ErrNotFound
	}
	for i := range snap.Processes {
		if snap.Processes[i].PID == pid {
			return snap.Processes[i], nil
		}
	}
	return metrics.ProcessInfo{}, ErrNotFound
}

// emit fires a controller event with a uniform payload.
func (c *Controller) emit(name string, pid uint32, extra map[string]any) {
	if c.emitter == nil {
		return
	}
	payload := map[string]any{"pid": pid}
	for k, v := range extra {
		payload[k] = v
	}
	c.emitter.Emit(name, payload)
}

// Kill terminates a process. confirm is required for system paths when
// ConfirmKillSystem is enabled.
func (c *Controller) Kill(pid uint32, confirm bool) error {
	info, err := c.findProcess(pid)
	if err != nil {
		return err
	}
	if err := c.safety.Check(info, confirm); err != nil {
		return err
	}
	h, err := seamOpenProcessHandle(windows.PROCESS_TERMINATE, pid)
	if err != nil {
		return fmt.Errorf("open process: %w", err)
	}
	defer seamCloseHandleSafe(h)
	if err := seamTerminateProcess(h, 1); err != nil {
		return fmt.Errorf("terminate: %w", err)
	}
	c.emit(EventKilled, pid, map[string]any{"name": info.Name})
	return nil
}

// KillTree terminates a process and every descendant. Each child must
// independently pass the safety check.
func (c *Controller) KillTree(rootPID uint32, confirm bool) (int, error) {
	snap := c.store.Latest()
	if snap == nil {
		return 0, ErrNotFound
	}
	descendants := collectDescendants(snap.Processes, rootPID)
	descendants = append(descendants, rootPID)
	killed := 0
	var firstErr error
	for _, pid := range descendants {
		if err := c.Kill(pid, confirm); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		killed++
	}
	return killed, firstErr
}

func collectDescendants(procs []metrics.ProcessInfo, root uint32) []uint32 {
	children := make(map[uint32][]uint32, len(procs))
	for _, p := range procs {
		children[p.ParentPID] = append(children[p.ParentPID], p.PID)
	}
	var out []uint32
	stack := append([]uint32{}, children[root]...)
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, pid)
		stack = append(stack, children[pid]...)
	}
	return out
}

// Suspend pauses every thread in a process via SuspendThread.
func (c *Controller) Suspend(pid uint32, confirm bool) error {
	info, err := c.findProcess(pid)
	if err != nil {
		return err
	}
	if err := c.safety.Check(info, confirm); err != nil {
		return err
	}
	if err := suspendOrResumeThreads(pid, true); err != nil {
		return err
	}
	c.emit(EventSuspended, pid, map[string]any{"name": info.Name})
	return nil
}

// Resume resumes every thread in a previously suspended process.
func (c *Controller) Resume(pid uint32) error {
	if err := suspendOrResumeThreads(pid, false); err != nil {
		return err
	}
	c.emit(EventResumed, pid, nil)
	return nil
}

func suspendOrResumeThreads(pid uint32, suspend bool) error {
	snap, err := seamCreateToolhelp32Snapshot(winapi.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer seamCloseHandleSafe(snap)

	var te winapi.THREADENTRY32
	if err := seamThread32First(snap, &te); err != nil {
		return err
	}
	const access = windows.THREAD_SUSPEND_RESUME
	var (
		touched  int
		firstErr error
	)
	for {
		if te.OwnerProcessID == pid {
			touched++
			h, err := seamOpenThreadHandle(access, te.ThreadID)
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("open thread %d: %w", te.ThreadID, err)
				}
			} else {
				if suspend {
					_, err = seamSuspendThread(h)
				} else {
					_, err = seamResumeThread(h)
				}
				seamCloseHandleSafe(h)
				if err != nil && firstErr == nil {
					op := "resume"
					if suspend {
						op = "suspend"
					}
					firstErr = fmt.Errorf("%s thread %d: %w", op, te.ThreadID, err)
				}
			}
		}
		if err := seamThread32Next(snap, &te); err != nil {
			if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return fmt.Errorf("enumerate threads: %w", err)
			}
			break
		}
	}
	if touched == 0 {
		return fmt.Errorf("no threads found for pid %d", pid)
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

// SetPriority maps a textual class to a Win32 priority class and applies it.
func (c *Controller) SetPriority(pid uint32, class string, confirm bool) error {
	info, err := c.findProcess(pid)
	if err != nil {
		return err
	}
	if err := c.safety.Check(info, confirm); err != nil {
		return err
	}
	priority, ok := priorityClassFromString(class)
	if !ok {
		return fmt.Errorf("unknown priority class %q", class)
	}
	h, err := seamOpenProcessHandle(windows.PROCESS_SET_INFORMATION, pid)
	if err != nil {
		return err
	}
	defer seamCloseHandleSafe(h)
	if err := seamSetPriorityClass(h, priority); err != nil {
		return err
	}
	c.emit(EventPriority, pid, map[string]any{"class": class})
	return nil
}

func priorityClassFromString(s string) (uint32, bool) {
	switch strings.ToLower(s) {
	case "idle":
		return winapi.IDLE_PRIORITY_CLASS, true
	case "below_normal", "below-normal":
		return winapi.BELOW_NORMAL_PRIORITY_CLASS, true
	case "normal":
		return winapi.NORMAL_PRIORITY_CLASS, true
	case "above_normal", "above-normal":
		return winapi.ABOVE_NORMAL_PRIORITY_CLASS, true
	case "high":
		return winapi.HIGH_PRIORITY_CLASS, true
	case "realtime":
		return winapi.REALTIME_PRIORITY_CLASS, true
	}
	return 0, false
}

// SetAffinity restricts a process to a subset of logical CPUs.
func (c *Controller) SetAffinity(pid uint32, mask uint64, confirm bool) error {
	info, err := c.findProcess(pid)
	if err != nil {
		return err
	}
	if err := c.safety.Check(info, confirm); err != nil {
		return err
	}
	if mask == 0 {
		return fmt.Errorf("affinity mask must include at least one CPU")
	}
	h, err := seamOpenProcessHandle(windows.PROCESS_SET_INFORMATION, pid)
	if err != nil {
		return err
	}
	defer seamCloseHandleSafe(h)
	if err := seamSetProcessAffinityMask(h, uintptr(mask)); err != nil {
		return err
	}
	c.emit(EventAffinity, pid, map[string]any{"mask": mask})
	return nil
}

// Limit creates a Job Object around the PID and applies CPU/memory caps.
// cpuPct is 1..100 (0 disables CPU rate control); maxBytes is the working
// set / process memory cap (0 disables).
func (c *Controller) Limit(pid uint32, cpuPct int, maxBytes uint64, confirm bool) error {
	info, err := c.findProcess(pid)
	if err != nil {
		return err
	}
	if err := c.safety.Check(info, confirm); err != nil {
		return err
	}
	if cpuPct < 0 || cpuPct > 100 {
		return fmt.Errorf("cpu percent out of range: %d", cpuPct)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.jobs[pid]; ok {
		// Replace existing job by closing it and creating fresh.
		seamCloseHandleSafe(existing.job)
		delete(c.jobs, pid)
	}

	job, err := seamCreateJobObject()
	if err != nil {
		return fmt.Errorf("create job: %w", err)
	}

	procH, err := seamOpenProcessHandle(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, pid)
	if err != nil {
		seamCloseHandleSafe(job)
		return fmt.Errorf("open process: %w", err)
	}
	defer seamCloseHandleSafe(procH)

	if err := seamAssignProcessToJobObject(job, procH); err != nil {
		seamCloseHandleSafe(job)
		return fmt.Errorf("assign job: %w", err)
	}

	if maxBytes > 0 {
		var ext winapi.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		ext.BasicLimitInformation.LimitFlags = winapi.JOB_OBJECT_LIMIT_PROCESS_MEMORY
		ext.ProcessMemoryLimit = uintptr(maxBytes)
		if err := seamSetInformationJobObject(
			job,
			winapi.JobObjectExtendedLimitInformation,
			unsafe.Pointer(&ext), // #nosec G103 -- Audited Win32 unsafe interop.
			uint32(unsafe.Sizeof(ext)),
		); err != nil {
			seamCloseHandleSafe(job)
			return fmt.Errorf("set memory limit: %w", err)
		}
	}

	if cpuPct > 0 {
		var rate winapi.JOBOBJECT_CPU_RATE_CONTROL_INFORMATION
		rate.ControlFlags = winapi.JOB_OBJECT_CPU_RATE_CONTROL_ENABLE | winapi.JOB_OBJECT_CPU_RATE_CONTROL_HARD_CAP
		// CpuRate is in 1/100 of a percent (0..10000).
		rate.CpuRate = uint32(cpuPct) * 100
		if err := seamSetInformationJobObject(
			job,
			winapi.JobObjectCpuRateControlInformation,
			unsafe.Pointer(&rate), // #nosec G103 -- Audited Win32 unsafe interop.
			uint32(unsafe.Sizeof(rate)),
		); err != nil {
			seamCloseHandleSafe(job)
			return fmt.Errorf("set cpu limit: %w", err)
		}
	}

	c.jobs[pid] = &jobEntry{job: job, pid: pid, cpuPct: cpuPct, memBytes: maxBytes}
	c.emit(EventLimited, pid, map[string]any{"cpu_pct": cpuPct, "mem_bytes": maxBytes})
	return nil
}

// ClearLimit drops the Job Object for a PID, releasing its caps.
func (c *Controller) ClearLimit(pid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.jobs[pid]
	if !ok {
		return fmt.Errorf("no active limit for pid %d", pid)
	}
	seamCloseHandleSafe(entry.job)
	delete(c.jobs, pid)
	c.emit(EventLimitCleared, pid, nil)
	return nil
}

// LimitInfo describes the active CPU/memory caps for a PID.
type LimitInfo struct {
	PID      uint32 `json:"pid"`
	CPUPct   int    `json:"cpu_pct"`
	MemBytes uint64 `json:"mem_bytes"`
}

// ActiveLimits returns a snapshot of every limited process.
func (c *Controller) ActiveLimits() []LimitInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LimitInfo, 0, len(c.jobs))
	for _, j := range c.jobs {
		out = append(out, LimitInfo{PID: j.pid, CPUPct: j.cpuPct, MemBytes: j.memBytes})
	}
	return out
}
