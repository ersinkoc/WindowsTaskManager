//go:build windows

package controller

// This file exposes the controller's dependency on the winapi package as
// overridable package-level function variables. Tests substitute these to
// drive defensive error branches that the real Windows kernel will not
// produce from userspace.
//
// The default values delegate to the real winapi package; tests override
// individual entries in their setup phase and restore them via t.Cleanup.

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

var (
	seamCreateJobObject          = winapi.CreateJobObject
	seamAssignProcessToJobObject = winapi.AssignProcessToJobObject
	seamOpenProcessHandle        = winapi.OpenProcessHandle
	seamSetInformationJobObject  = winapi.SetInformationJobObject
	seamSetPriorityClass         = winapi.SetPriorityClass
	seamSetProcessAffinityMask   = winapi.SetProcessAffinityMask
	seamCreateToolhelp32Snapshot = winapi.CreateToolhelp32Snapshot
	seamThread32First            = func(snap windows.Handle, entry *winapi.THREADENTRY32) error {
		return winapi.Thread32First(snap, entry)
	}
	seamThread32Next = func(snap windows.Handle, entry *winapi.THREADENTRY32) error {
		return winapi.Thread32Next(snap, entry)
	}
	seamOpenThreadHandle = winapi.OpenThreadHandle
	seamSuspendThread    = winapi.SuspendThread
	seamResumeThread     = winapi.ResumeThread
	seamTerminateProcess = winapi.TerminateProcessHandle
	seamCloseHandleSafe  = winapi.CloseHandleSafe
)

// keep imports referenced
var _ = unsafe.Pointer(nil)
