//go:build windows

package config

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

const watcherWaitInterval = 500 * time.Millisecond

// waitForSingleObject is the Windows API used by Start to block until a
// directory-change notification fires. It is a package-level variable so
// tests can substitute a stub that returns synthetic values to exercise the
// error / default branches of Start, which are unreachable in normal
// operation against a valid change-notification handle.
var waitForSingleObject = windows.WaitForSingleObject

// Watcher uses native Windows directory change notifications to reload the
// active config file after atomic save/rename operations.
type Watcher struct {
	path     string
	onChange func(*Config)
	debounce time.Duration
}

func NewWatcher(path string, onChange func(*Config)) *Watcher {
	return &Watcher{
		path:     path,
		onChange: onChange,
		debounce: 300 * time.Millisecond,
	}
}

// Start begins watching. If native change notification cannot be established,
// it falls back to conservative stat polling so config reload remains available.
func (w *Watcher) Start(ctx context.Context) {
	dir := filepath.Dir(w.path)
	mask := uint32(windows.FILE_NOTIFY_CHANGE_LAST_WRITE | windows.FILE_NOTIFY_CHANGE_FILE_NAME)
	handle, err := windows.FindFirstChangeNotification(dir, false, mask)
	if err != nil {
		w.pollStart(ctx)
		return
	}
	defer windows.FindCloseChangeNotification(handle)

	lastMod := w.currentModTime()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		event, err := waitForSingleObject(handle, uint32(watcherWaitInterval/time.Millisecond))
		if err != nil {
			w.pollStart(ctx)
			return
		}

		switch event {
		case uint32(windows.WAIT_OBJECT_0):
			_ = windows.FindNextChangeNotification(handle)
			lastMod = w.reloadIfChanged(lastMod)
		case uint32(windows.WAIT_TIMEOUT):
			continue
		default:
			w.pollStart(ctx)
			return
		}
	}
}

func (w *Watcher) reloadIfChanged(lastMod time.Time) time.Time {
	info, err := os.Stat(w.path)
	if err != nil {
		return lastMod
	}
	if !info.ModTime().After(lastMod) {
		return lastMod
	}
	time.Sleep(w.debounce)
	cfg, err := Load(w.path)
	if err == nil && w.onChange != nil {
		w.onChange(cfg)
	}
	return info.ModTime()
}

func (w *Watcher) currentModTime() time.Time {
	info, err := os.Stat(w.path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func (w *Watcher) pollStart(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastMod := w.currentModTime()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastMod = w.reloadIfChanged(lastMod)
		}
	}
}
