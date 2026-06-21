//go:build windows

package platform

import (
	"errors"
	"testing"
)

// TestAcquireSingleInstance_Success verifies the happy path: a fresh mutex
// name acquires cleanly, the release func is non-nil, and calling it
// releases the underlying handle without panicking. Calling release a
// second time must remain safe — CloseHandle on an invalid handle returns
// ERROR_INVALID_HANDLE which is intentionally swallowed by the
// implementation, so re-invocation is idempotent at the user-visible level.
func TestAcquireSingleInstance_Success(t *testing.T) {
	const name = `Local\WTM-Test-SingleInstance-Success`

	release, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if release == nil {
		t.Fatal("release func is nil on success")
	}

	release()
	// Second call must not panic.
	release()
}

// TestAcquireSingleInstance_AlreadyRunning_CreateMutex verifies the
// ERROR_ALREADY_EXISTS branch returned by CreateMutex itself: while the
// first instance still holds the mutex, a second AcquireSingleInstance
// for the same name must return ErrAlreadyRunning and a nil release func.
//
// This also exercises the inner `if h != 0 { CloseHandle(h) }` block of
// the source — when CreateMutex fails with ERROR_ALREADY_EXISTS, the
// wrapper still hands back a non-zero handle that points to the existing
// mutex; we must close it so we don't leak the handle.
func TestAcquireSingleInstance_AlreadyRunning_CreateMutex(t *testing.T) {
	const name = `Local\WTM-Test-SingleInstance-AlreadyRunning`

	release, err := AcquireSingleInstance(name)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer release()

	second, err := AcquireSingleInstance(name)
	if !errors.Is(err, ErrAlreadyRunning) {
		if second != nil {
			second()
		}
		t.Fatalf("second acquire err=%v want %v", err, ErrAlreadyRunning)
	}
	if second != nil {
		t.Fatal("release func should be nil when ErrAlreadyRunning is returned")
	}
}

// TestAcquireSingleInstance_InvalidName covers the UTF16PtrFromString
// error branch. golang.org/x/sys/windows.UTF16PtrFromString returns
// syscall.EINVAL for any string containing an embedded NUL byte, and
// AcquireSingleInstance must propagate that error as-is — NOT as
// ErrAlreadyRunning — and return a nil release func.
func TestAcquireSingleInstance_InvalidName(t *testing.T) {
	// A name with an embedded NUL byte is the documented failure mode
	// of UTF16PtrFromString (and any other API that builds a
	// NUL-terminated UTF-16 buffer).
	nameWithNUL := "Local\000WTM-Embedded-NUL"

	release, err := AcquireSingleInstance(nameWithNUL)
	if err == nil {
		if release != nil {
			release()
		}
		t.Fatalf("expected error for name with NUL byte, got nil")
	}
	if errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("invalid-name error should not be ErrAlreadyRunning, got %v", err)
	}
	if release != nil {
		t.Fatal("release func should be nil when UTF16PtrFromString fails")
	}
}

// TestAcquireSingleInstance_CreateMutexOtherError covers the branch
// where windows.CreateMutex returns a non-ERROR_ALREADY_EXISTS error.
// This happens for malformed names that pass UTF16PtrFromString but
// are rejected by CreateMutexW (e.g., a Local\ name containing
// additional path segments, which Windows rejects with
// ERROR_PATH_NOT_FOUND). AcquireSingleInstance must propagate the
// error verbatim — NOT as ErrAlreadyRunning — and return a nil
// release func.
func TestAcquireSingleInstance_CreateMutexOtherError(t *testing.T) {
	// Local\Bad\Path\With\Slashes is a syntactically valid Go string
	// and a valid UTF-16 sequence, but Windows rejects it as an
	// invalid object name. The CreateMutex wrapper surfaces this as
	// a non-nil error that is NOT ERROR_ALREADY_EXISTS.
	const badName = `Local\Bad\Path\With\Slashes`

	release, err := AcquireSingleInstance(badName)
	if err == nil {
		if release != nil {
			release()
		}
		t.Fatalf("expected error for malformed mutex name, got nil")
	}
	if errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("malformed-name error should not be ErrAlreadyRunning, got %v", err)
	}
	if release != nil {
		t.Fatal("release func should be nil when CreateMutex fails")
	}
}

// TestAcquireSingleInstance_UniqueNames verifies that two distinct mutex
// names both succeed and operate independently — guards against a
// regression where the name parameter is ignored.
func TestAcquireSingleInstance_UniqueNames(t *testing.T) {
	const a = `Local\WTM-Test-SingleInstance-UniqueA`
	const b = `Local\WTM-Test-SingleInstance-UniqueB`

	releaseA, err := AcquireSingleInstance(a)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	defer releaseA()

	releaseB, err := AcquireSingleInstance(b)
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	defer releaseB()

	if releaseA == nil || releaseB == nil {
		t.Fatal("release funcs must be non-nil on success")
	}
}

// TestErrAlreadyRunning_IsExported verifies that ErrAlreadyRunning is a
// stable, non-nil sentinel error that callers can compare against with
// errors.Is.
func TestErrAlreadyRunning_IsExported(t *testing.T) {
	if ErrAlreadyRunning == nil {
		t.Fatal("ErrAlreadyRunning is nil")
	}
	if ErrAlreadyRunning.Error() == "" {
		t.Fatal("ErrAlreadyRunning has empty message")
	}
}
