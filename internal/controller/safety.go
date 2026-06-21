//go:build windows

package controller

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

// Common errors returned by the controller surface.
var (
	ErrProtected     = errors.New("process is protected")
	ErrCritical      = errors.New("process is marked critical by Windows")
	ErrConfirmNeeded = errors.New("operation requires explicit confirmation")
	ErrNotFound      = errors.New("process not found")
	ErrSelf          = errors.New("operation not allowed on the running Windows Task Manager process")
)

// Safety enforces the kill/suspend/limit policies declared in config.
type Safety struct {
	mu      sync.RWMutex
	cfg     *config.Config
	selfPID uint32
}

func NewSafety(cfg *config.Config) *Safety {
	return newSafetyWithPID(cfg, os.Getpid())
}

// newSafetyWithPID builds a Safety with an explicit self PID. Exposed
// only to the test package via the same-package access; production code
// always goes through NewSafety.
func newSafetyWithPID(cfg *config.Config, pid int) *Safety {
	selfPID := uint32(0)
	if pid > 0 {
		if uint64(pid) > uint64(math.MaxUint32) {
			selfPID = math.MaxUint32
		} else {
			selfPID = uint32(pid)
		}
	}
	return &Safety{
		cfg:     cfg,
		selfPID: selfPID,
	}
}

// SetConfig swaps the active config (called on hot reload).
func (s *Safety) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// Check verifies whether destructive action is allowed against `info`.
// `confirmed` indicates the user has provided explicit confirmation
// for confirm-required cases.
func (s *Safety) Check(info metrics.ProcessInfo, confirmed bool) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	if cfg == nil {
		return ErrProtected
	}
	if info.PID == s.selfPID {
		return ErrSelf
	}
	if info.PID == 0 || info.PID == 4 {
		return ErrCritical
	}
	if info.IsCritical {
		return ErrCritical
	}
	name := strings.ToLower(info.Name)
	for _, p := range cfg.Controller.ProtectedProcesses {
		if strings.EqualFold(p, name) || strings.EqualFold(p, info.Name) {
			return ErrProtected
		}
	}
	if isSystemPath(info.ExePath) && cfg.Controller.ConfirmKillSystem && !confirmed {
		return ErrConfirmNeeded
	}
	return nil
}

// isSystemPath reports whether an executable lives under %SystemRoot%.
func isSystemPath(path string) bool {
	if path == "" {
		return false
	}
	systemRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	root := strings.ToLower(filepath.Clean(systemRoot))
	p := strings.ToLower(filepath.Clean(path))
	if !strings.HasSuffix(root, `\`) {
		root += `\`
	}
	return strings.HasPrefix(p, root)
}
