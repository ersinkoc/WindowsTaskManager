//go:build windows

package desktop

import (
	"log"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// Window is a frameless window hosting a WebView2 control.
type Window struct {
	wv      webview2.WebView
	done    func()
	hwnd    uintptr
	onClose func()
}

func New(url, title string, width, height int, onDone func()) *Window {
	// Create webview2 in a normal decorated window first
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  uint(width),
			Height: uint(height),
			Center: true,
		},
	})
	if w == nil {
		log.Println("desktop: WebView2 unavailable — install Microsoft Edge WebView2 Runtime")
		return nil
	}

	// webview2 doesn't expose hwnd directly, so we use pointer arithmetic
	// to access its internal handle
	type webview2Fix struct {
		hwnd uintptr
	}

	hwnd := (*webview2Fix)(unsafe.Pointer(&w)).hwnd
	log.Printf("webview2 hwnd: %d\n", hwnd)

	// First show the window so WebView2 initializes content, then strip decorations
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 1) // SW_SHOW first
		makeFrameless(hwnd, width, height)
		procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(width), uintptr(height), 0x0001|0x0003|0x0004|0x0020)
		procShowWindow.Call(hwnd, 9) // SW_SHOWDEFAULT
		procSetForegroundWindow.Call(hwnd)
	}

	win := &Window{wv: w, hwnd: hwnd, done: onDone, onClose: onDone}
	w.Bind("__wtmDesktopClose", win.close)
	w.Bind("__wtmDesktopMinimize", win.minimize)
	w.Bind("__wtmDesktopMaximize", win.maximize)
	w.Navigate(url)
	return win
}

func (w *Window) minimize() {
	if w.hwnd != 0 {
		procShowWindow.Call(w.hwnd, 6)
	}
}

func (w *Window) maximize() {
	if w.hwnd != 0 {
		procShowWindow.Call(w.hwnd, 3)
	}
}

func (w *Window) close() {
	if w.onClose != nil {
		w.onClose()
	}
	w.onClose = nil
	if w.wv != nil {
		w.wv.Destroy()
	}
}

func (w *Window) Run() {
	if w == nil {
		return
	}
	w.wv.Run()
}

// ---------------------------------------------------------------------------
// Frameless window helpers
// ---------------------------------------------------------------------------

var (
	user32                  = windows.NewLazyDLL("user32.dll")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
)

func makeFrameless(hwnd uintptr, width, height int) {
	if hwnd == 0 {
		return
	}

	// Window style constants
	const (
		GWL_STYLE       = ^uintptr(15) // -16 as uintptr
		GWL_EXSTYLE     = ^uintptr(19) // -20 as uintptr
		WS_CAPTION      = uint32(0x00C00000)
		WS_SYSMENU      = uint32(0x00080000)
		WS_SIZEBOX      = uint32(0x00040000)
		WS_MAXIMIZEBOX  = uint32(0x00010000)
		WS_MINIMIZEBOX  = uint32(0x00020000)
		WS_POPUP        = uint32(0x80000000)
		WS_EX_APPWINDOW = uint32(0x00040000)
	)

	// Get current style
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, GWL_STYLE)
	currentStyle := uint32(style)

	// Remove title bar completely; keep popup so window appears in alt-tab
	removeFlags := WS_CAPTION | WS_SYSMENU | WS_SIZEBOX | WS_MAXIMIZEBOX | WS_MINIMIZEBOX
	newStyle := (currentStyle & ^removeFlags) | WS_POPUP

	log.Printf("makeFrameless: style 0x%x -> 0x%x\n", currentStyle, newStyle)

	procSetWindowLongPtrW.Call(hwnd, GWL_STYLE, uintptr(newStyle))

	// Update extended style — no WS_EX_APPWINDOW to avoid Windows adding extra chrome
	exStyle, _, _ := procGetWindowLongPtrW.Call(hwnd, GWL_EXSTYLE)
	procSetWindowLongPtrW.Call(hwnd, GWL_EXSTYLE, exStyle)

	// Resize and force recalculation of non-client area, also shows the window
	procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(width), uintptr(height), 0x0001|0x0003|0x0004|0x0020) // SWP_SHOWWINDOW|SWP_NOMOVE|SWP_NOSIZE|SWP_FRAMECHANGED
}
