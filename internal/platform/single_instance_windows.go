//go:build windows

package platform

import (
	"errors"

	"golang.org/x/sys/windows"
)

var ErrAlreadyRunning = errors.New("another Windows Task Manager instance is already running")

// AcquireSingleInstance reserves a named system-wide mutex for the lifetime
// of the current process. Call the returned release func during shutdown.
func AcquireSingleInstance(name string) (func(), error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	h, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		// The golang.org/x/sys/windows.CreateMutex wrapper uses the
		// directive [failretval == 0 || e1 == ERROR_ALREADY_EXISTS],
		// so a returned non-nil error covers both "handle is 0" AND
		// "the named mutex already exists". We only treat the latter
		// as ErrAlreadyRunning; other errors are propagated verbatim.
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			if h != 0 {
				_ = windows.CloseHandle(h)
			}
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}

	return func() {
		_ = windows.CloseHandle(h)
	}, nil
}
