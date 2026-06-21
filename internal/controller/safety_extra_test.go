//go:build windows

package controller

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

// TestSafetyCheckNilConfig verifies the nil-cfg branch.
func TestSafetyCheckNilConfig(t *testing.T) {
	s := NewSafety(nil)
	err := s.Check(metrics.ProcessInfo{PID: 1, Name: "x"}, true)
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("err=%v want ErrProtected", err)
	}
}

// TestSafetyCheckSelfPID covers the ErrSelf branch (also exercised by the
// existing TestSafetyRejectsSelfPID but here with a fresh Safety instance).
func TestSafetyCheckSelfPID(t *testing.T) {
	s := NewSafety(config.DefaultConfig())
	err := s.Check(metrics.ProcessInfo{
		PID:  uint32(os.Getpid()),
		Name: "wtm.exe",
	}, true)
	if !errors.Is(err, ErrSelf) {
		t.Fatalf("err=%v want ErrSelf", err)
	}
}

// TestSafetyCheckCriticalSystemPIDs covers the PID 0 and PID 4 branches.
func TestSafetyCheckCriticalSystemPIDs(t *testing.T) {
	s := NewSafety(config.DefaultConfig())
	for _, pid := range []uint32{0, 4} {
		err := s.Check(metrics.ProcessInfo{PID: pid, Name: "system"}, true)
		if !errors.Is(err, ErrCritical) {
			t.Fatalf("pid=%d err=%v want ErrCritical", pid, err)
		}
	}
}

// TestSafetyCheckIsCritical covers the IsCritical flag branch.
func TestSafetyCheckIsCritical(t *testing.T) {
	s := NewSafety(config.DefaultConfig())
	err := s.Check(metrics.ProcessInfo{PID: 12345, Name: "x", IsCritical: true}, true)
	if !errors.Is(err, ErrCritical) {
		t.Fatalf("err=%v want ErrCritical", err)
	}
}

// TestSafetyCheckProtectedByName covers the protected-process branch in both
// the lowercased-name path and the raw-name path.
func TestSafetyCheckProtectedByName(t *testing.T) {
	s := NewSafety(config.DefaultConfig())
	for _, name := range []string{"csrss.exe", "CSRSS.EXE", "Lsass.exe"} {
		err := s.Check(metrics.ProcessInfo{PID: 99999, Name: name}, true)
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("name=%s err=%v want ErrProtected", name, err)
		}
	}
}

// TestSafetyCheckSystemPathConfirm exercises the system-path + confirm branch.
func TestSafetyCheckSystemPathConfirm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = nil // remove default protections
	cfg.Controller.ConfirmKillSystem = true
	s := NewSafety(cfg)

	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}

	// Unconfirmed → ErrConfirmNeeded.
	err := s.Check(metrics.ProcessInfo{
		PID:     99999,
		Name:    "imaginary.exe",
		ExePath: root + `\System32\imaginary.exe`,
	}, false)
	if !errors.Is(err, ErrConfirmNeeded) {
		t.Fatalf("err=%v want ErrConfirmNeeded", err)
	}

	// Confirmed → no error.
	err = s.Check(metrics.ProcessInfo{
		PID:     99999,
		Name:    "imaginary.exe",
		ExePath: root + `\System32\imaginary.exe`,
	}, true)
	if err != nil {
		t.Fatalf("confirmed err=%v", err)
	}
}

// TestSafetyCheckNonSystemPathPasses ensures a non-system exe passes cleanly.
func TestSafetyCheckNonSystemPathPasses(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = nil
	s := NewSafety(cfg)
	err := s.Check(metrics.ProcessInfo{
		PID:     99999,
		Name:    "userapp.exe",
		ExePath: `C:\Users\me\app\userapp.exe`,
	}, false)
	if err != nil {
		t.Fatalf("err=%v want nil", err)
	}
}

// TestSafetySetConfigHotReload verifies SetConfig swaps the active config.
func TestSafetySetConfigHotReload(t *testing.T) {
	s := NewSafety(config.DefaultConfig())
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = []string{"never-match.exe"}
	s.SetConfig(cfg)
	err := s.Check(metrics.ProcessInfo{PID: 1, Name: "never-match.exe"}, true)
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("err=%v want ErrProtected", err)
	}
}

// TestSafetyCheckConfirmDisabledOnNonSystem verifies that with
// ConfirmKillSystem=false the system path is allowed.
func TestSafetyCheckConfirmDisabledOnNonSystem(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = nil
	cfg.Controller.ConfirmKillSystem = false
	s := NewSafety(cfg)
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	err := s.Check(metrics.ProcessInfo{
		PID:     99999,
		Name:    "imaginary.exe",
		ExePath: root + `\System32\imaginary.exe`,
	}, false)
	if err != nil {
		t.Fatalf("err=%v want nil when confirm disabled", err)
	}
}

// TestIsSystemPathCoversBranches exercises isSystemPath directly.
func TestIsSystemPathCoversBranches(t *testing.T) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"non-system path", `C:\Program Files\App\app.exe`, false},
		{"system path under root", root + `\System32\foo.exe`, true},
		// Bare root (with or without trailing slash) is not a prefix match
		// because the implementation normalises the root to end with `\`
		// and then requires the path to start with that longer prefix.
		{"system root bare", root, false},
		{"system root with trailing slash", root + `\`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isSystemPath(tc.path)
			if got != tc.want {
				t.Fatalf("isSystemPath(%q)=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestIsSystemPathMissingEnv covers the SystemRoot fallback to C:\Windows.
// We simulate this by setting SystemRoot to empty via Setenv; restore on exit.
func TestIsSystemPathMissingEnv(t *testing.T) {
	orig, had := os.LookupEnv("SystemRoot")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SystemRoot", orig)
		} else {
			_ = os.Unsetenv("SystemRoot")
		}
	})
	_ = os.Setenv("SystemRoot", "")
	// Also test that a whitespace-only SystemRoot still falls back to the default.
	_ = os.Setenv("SystemRoot", "   ")

	if !isSystemPath(`C:\Windows\System32\foo.exe`) {
		t.Fatal("expected true for default root")
	}
	if isSystemPath(`D:\Other\app.exe`) {
		t.Fatal("expected false for non-default root")
	}
}

// TestIsSystemPathNoTrailingSlash covers the suffix-addition branch.
func TestIsSystemPathNoTrailingSlash(t *testing.T) {
	// Point SystemRoot at a path without trailing slash and verify matching.
	orig, had := os.LookupEnv("SystemRoot")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("SystemRoot", orig)
		} else {
			_ = os.Unsetenv("SystemRoot")
		}
	})
	_ = os.Setenv("SystemRoot", `C:\MyWin`)
	if !isSystemPath(`C:\MyWin\System32\foo.exe`) {
		t.Fatal("expected true under custom SystemRoot without trailing slash")
	}
	if isSystemPath(`C:\Other\foo.exe`) {
		t.Fatal("expected false outside custom SystemRoot")
	}
}

// TestSafetyNewSafetyClampsOverflowPID covers the defensive MaxUint32
// branch in newSafetyWithPID: when the input pid exceeds MaxUint32, the
// stored selfPID is clamped to math.MaxUint32.
func TestSafetyNewSafetyClampsOverflowPID(t *testing.T) {
	// math.MaxUint32 = 4294967295 on 32-bit, 0xFFFFFFFF.
	s := newSafetyWithPID(config.DefaultConfig(), int(^uint32(0))+1)
	if s.selfPID != math.MaxUint32 {
		t.Fatalf("selfPID=%d want %d", s.selfPID, uint32(math.MaxUint32))
	}
}

// TestSafetyNewSafetyNonPositive covers the pid<=0 branch (selfPID=0).
func TestSafetyNewSafetyNonPositive(t *testing.T) {
	for _, pid := range []int{0, -1, -100} {
		s := newSafetyWithPID(config.DefaultConfig(), pid)
		if s.selfPID != 0 {
			t.Fatalf("pid=%d selfPID=%d want 0", pid, s.selfPID)
		}
	}
}
