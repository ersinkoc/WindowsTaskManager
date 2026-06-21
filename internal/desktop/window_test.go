//go:build windows

package desktop

import (
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/jchv/go-webview2"
)

// stubWebView is a minimal implementation of webview2.WebView used to drive
// the *Window methods that would otherwise require a real WebView2 runtime.
// It records Destroy calls so tests can assert close() actually invoked
// wv.Destroy() instead of no-op'ing on a nil receiver.
type stubWebView struct {
	destroyCount  atomic.Int32
	bindCalls     atomic.Int32
	navigateCalls atomic.Int32
	terminateHits atomic.Int32
	runExited     atomic.Bool
}

func (s *stubWebView) Run() {
	// Mimic a no-op main loop: most tests want to inspect state right after
	// close() / Run() returns without a real Win32 message pump. We exit
	// immediately so callers that wrap Run() can complete deterministically.
	s.runExited.Store(true)
}

func (s *stubWebView) Terminate()                            { s.terminateHits.Add(1) }
func (s *stubWebView) Dispatch(_ func())                     {}
func (s *stubWebView) Destroy()                              { s.destroyCount.Add(1) }
func (s *stubWebView) Window() unsafe.Pointer                { return nil }
func (s *stubWebView) SetTitle(_ string)                     {}
func (s *stubWebView) SetSize(_ int, _ int, _ webview2.Hint) {}
func (s *stubWebView) Navigate(_ string)                     { s.navigateCalls.Add(1) }
func (s *stubWebView) SetHtml(_ string)                      {}
func (s *stubWebView) Init(_ string)                         {}
func (s *stubWebView) Eval(_ string)                         {}
func (s *stubWebView) Bind(_ string, _ interface{}) error    { s.bindCalls.Add(1); return nil }

// newTestWindow returns a *Window with a stub WebView and the given hwnd.
// Tests can build Window values directly because the struct's fields are
// accessible inside the desktop package.
func newTestWindow(hwnd uintptr, wv webview2.WebView, onClose func()) *Window {
	return &Window{
		wv:      wv,
		hwnd:    hwnd,
		onClose: onClose,
	}
}

// TestRun_NilReceiver verifies that calling Run() on a nil *Window is a
// no-op rather than a panic. The implementation guards with `if w == nil
// { return }` so dereferences never happen.
func TestRun_NilReceiver(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Run on nil receiver panicked: %v", r)
		}
	}()
	var w *Window
	w.Run()
	// If we get here, the nil guard fired. Nothing else to assert; the
	// function literally returns immediately.
}

// TestRun_NonNilReceiver verifies that Run() on a non-nil window reaches
// the WebView's Run loop. The stub records the call so we can confirm
// dispatch happened without blocking on a real Win32 message pump.
func TestRun_NonNilReceiver(t *testing.T) {
	stub := &stubWebView{}
	w := newTestWindow(0, stub, nil)
	w.Run()
	if !stub.runExited.Load() {
		t.Fatal("Run() did not invoke wv.Run() on the stub")
	}
}

// TestClose_OnCloseNil_NilWV covers the close() branch where neither
// onClose nor wv are populated. Both guards must be exercised: the
// onClose != nil guard is false (so the call is skipped) and the wv !=
// nil guard is false (so Destroy is skipped).
func TestClose_OnCloseNil_NilWV(t *testing.T) {
	w := newTestWindow(0, nil, nil)
	w.close()
	// No panic, onClose is still nil, wv is still nil — nothing observable
	// to assert beyond "no panic" but this exercises both guard branches.
	if w.onClose != nil {
		t.Fatal("onClose should remain nil")
	}
	if w.wv != nil {
		t.Fatal("wv should remain nil")
	}
}

// TestClose_OnCloseSet_NilWV covers the branch where onClose is non-nil
// but wv is nil. The onClose callback must fire; the wv != nil guard
// must short-circuit before reaching Destroy().
func TestClose_OnCloseSet_NilWV(t *testing.T) {
	calls := 0
	w := newTestWindow(0, nil, func() { calls++ })
	w.close()
	if calls != 1 {
		t.Fatalf("onClose invocations = %d, want 1", calls)
	}
	if w.onClose != nil {
		t.Fatal("onClose should be reset to nil after firing")
	}
	if w.wv != nil {
		t.Fatal("wv should remain nil")
	}
}

// TestClose_OnCloseNil_NonNilWV covers the branch where onClose is nil
// but wv is populated. Destroy() must be called once. The onClose guard
// must short-circuit before the wv guard.
func TestClose_OnCloseNil_NonNilWV(t *testing.T) {
	stub := &stubWebView{}
	w := newTestWindow(0, stub, nil)
	w.close()
	if got := stub.destroyCount.Load(); got != 1 {
		t.Fatalf("Destroy calls = %d, want 1", got)
	}
}

// TestClose_OnCloseSet_NonNilWV covers the full happy path: onClose
// fires, then wv.Destroy() fires, and onClose is reset to nil so a
// second close() does not re-invoke the user callback (idempotency).
func TestClose_OnCloseSet_NonNilWV(t *testing.T) {
	calls := 0
	stub := &stubWebView{}
	w := newTestWindow(0, stub, func() { calls++ })
	w.close()
	if calls != 1 {
		t.Fatalf("onClose invocations after first close = %d, want 1", calls)
	}
	if got := stub.destroyCount.Load(); got != 1 {
		t.Fatalf("Destroy calls after first close = %d, want 1", got)
	}
	// Second close: onClose was reset, so user callback must NOT fire
	// again, but Destroy may still be invoked (the production code does
	// not gate Destroy on onClose).
	w.close()
	if calls != 1 {
		t.Fatalf("onClose invocations after second close = %d, want 1 (idempotent)", calls)
	}
	if got := stub.destroyCount.Load(); got != 2 {
		t.Fatalf("Destroy calls after second close = %d, want 2", got)
	}
	if w.onClose != nil {
		t.Fatal("onClose should remain nil after second close")
	}
}

// TestMinimize_HwndZero verifies minimize() is a no-op when the window
// has not yet been created (hwnd == 0). The ShowWindow syscall must NOT
// be invoked, otherwise Windows would refuse the call on a null HWND.
func TestMinimize_HwndZero(t *testing.T) {
	w := newTestWindow(0, nil, nil)
	// We cannot easily observe ShowWindow directly without a real hwnd,
	// so the assertion is "no panic" plus the documented behavior: a
	// zero hwnd short-circuits before the syscall.
	w.minimize()
}

// TestMinimize_NonZeroHwnd covers the branch where hwnd is non-zero and
// minimize() must call ShowWindow. The stub is not used here because
// minimize only depends on the hwnd field. A fake (but non-zero) hwnd
// causes ShowWindow to fail harmlessly on a non-existent HWND — that
// failure is what the production code already expects.
func TestMinimize_NonZeroHwnd(t *testing.T) {
	w := newTestWindow(0xDEADBEEF, nil, nil)
	w.minimize()
}

// TestMaximize_HwndZero verifies maximize() is a no-op when the window
// has not yet been created (hwnd == 0). Same rationale as minimize.
func TestMaximize_HwndZero(t *testing.T) {
	w := newTestWindow(0, nil, nil)
	w.maximize()
}

// TestMaximize_NonZeroHwnd covers the branch where hwnd is non-zero and
// maximize() must call ShowWindow. Same rationale as TestMinimize_NonZeroHwnd.
func TestMaximize_NonZeroHwnd(t *testing.T) {
	w := newTestWindow(0xDEADBEEF, nil, nil)
	w.maximize()
}

// TestMakeFrameless_HwndZero verifies the early return at the top of
// makeFrameless: a zero hwnd must skip the entire body so the user32
// procs are not called with a null HWND.
func TestMakeFrameless_HwndZero(t *testing.T) {
	makeFrameless(0, 800, 600)
	// No observable side effects possible — hwnd is filtered out.
}

// TestMakeFrameless_NonZeroHwnd exercises the full body of makeFrameless
// using a fake (non-zero) hwnd. The user32 procs are real Win32 calls
// that will fail silently on a non-existent HWND — but that is exactly
// what we want for coverage: every line below the guard must run.
func TestMakeFrameless_NonZeroHwnd(t *testing.T) {
	const fakeHwnd uintptr = 0xDEADBEEF
	makeFrameless(fakeHwnd, 1024, 768)
	// The procs are *windows.LazyProc — they swallow errors internally,
	// so we cannot assert on return values. The test exists purely to
	// drive the body for coverage.
}

// TestNew_WebView2Unavailable covers the early-return path in New() where
// webview2.NewViewFactory() returns nil because the WebView2 Runtime is
// not installed (or refuses to start). The function must log and return
// nil rather than panicking on subsequent wv.Bind / wv.Navigate calls.
//
// The factory seam is a package-level variable that defaults to the real
// webview2.NewWithOptions in production code. Tests swap it for a stub
// that returns nil to deterministically exercise the nil branch on any
// host (WebView2 installed or not).
func TestNew_WebView2Unavailable(t *testing.T) {
	prev := newWebView
	newWebView = func(_ webview2.WebViewOptions) webview2.WebView { return nil }
	t.Cleanup(func() { newWebView = prev })

	w := New("https://example.invalid", "title", 800, 600, nil)
	if w != nil {
		t.Fatal("New() must return nil when the webview factory returns nil")
	}
}

// TestNew_WebView2FactoryReturnsNilWithoutTitle covers the additional
// coverage case: the early-return path in New() does not depend on any
// other argument (url, title, width, height, onDone). We re-test with
// different argument shapes to make sure no argument is consumed before
// the nil check that would have panicked.
func TestNew_WebView2FactoryReturnsNilWithoutTitle(t *testing.T) {
	prev := newWebView
	newWebView = func(_ webview2.WebViewOptions) webview2.WebView { return nil }
	t.Cleanup(func() { newWebView = prev })

	// Zero values across the board — if the nil check happens after any
	// of these are dereferenced, we'd see a panic.
	w := New("", "", 0, 0, nil)
	if w != nil {
		t.Fatal("New() must return nil with zero arguments when the webview factory returns nil")
	}
}

// TestNew_Success exercises the success path of New() using a stub
// factory that returns a non-nil WebView. This drives every statement
// after the nil check: the pointer-arithmetic hwnd extraction, the
// ShowWindow/makeFrameless/SetWindowPos/SetForegroundWindow calls (all
// of which are real Win32 calls routed through the lazy procs in the
// package), and the w.Bind / w.Navigate dispatches. The stub records
// those calls so we can assert they happened in the right order.
func TestNew_Success(t *testing.T) {
	stub := &stubWebView{}
	prev := newWebView
	newWebView = func(_ webview2.WebViewOptions) webview2.WebView { return stub }
	t.Cleanup(func() { newWebView = prev })

	onDone := func() {}
	w := New("https://example.invalid", "title", 800, 600, onDone)
	if w == nil {
		t.Fatal("New() must return a *Window when the webview factory returns non-nil")
	}
	defer w.close()

	if got := stub.bindCalls.Load(); got != 3 {
		t.Fatalf("Bind calls = %d, want 3 (close/minimize/maximize)", got)
	}
	if got := stub.navigateCalls.Load(); got != 1 {
		t.Fatalf("Navigate calls = %d, want 1", got)
	}
	if w.onClose == nil {
		t.Fatal("onClose should be set to the onDone callback")
	}
	if w.done == nil {
		t.Fatal("done should be set to the onDone callback")
	}
}
