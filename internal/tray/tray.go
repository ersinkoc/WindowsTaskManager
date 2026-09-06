//go:build windows

package tray

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/winapi"
)

// Menu item IDs.
const (
	idOpenDashboard uintptr = 1001
	idOpenConfig    uintptr = 1002
	idQuit          uintptr = 1099
)

// Tray runs a Win32 message loop, owns the notification icon, and shows
// rate-limited balloon notifications when alerts arrive.
type Tray struct {
	cfg        *config.Config
	dashURL    string
	configPath string
	onQuit     func()
	emitter    *event.Emitter

	mu          sync.Mutex
	hwnd        uintptr
	nid         winapi.NOTIFYICONDATAW
	lastBalloon time.Time
}

func New(cfg *config.Config, dashboardURL, configPath string, emitter *event.Emitter, onQuit func()) *Tray {
	return &Tray{
		cfg:        cfg,
		dashURL:    dashboardURL,
		configPath: configPath,
		emitter:    emitter,
		onQuit:     onQuit,
	}
}

// SetConfig hot-swaps the config (notably the rate limit/severity policy).
func (t *Tray) SetConfig(cfg *config.Config) {
	t.mu.Lock()
	t.cfg = cfg
	t.mu.Unlock()
}

// Stop signals the tray message loop to exit. It does not call onQuit
// to avoid double-cancel when Stop is called as part of a wider shutdown.
func (t *Tray) Stop() {
	t.mu.Lock()
	hwnd := t.hwnd
	t.mu.Unlock()
	if hwnd != 0 {
		winapi.PostMessage(hwnd, 0x0012, 0, 0) // WM_QUIT
	}
}

// Run blocks until the tray icon is destroyed. It must be called on a
// dedicated OS thread because Windows requires the message loop to live
// on the thread that registered the window class.
func (t *Tray) Run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := t.create(); err != nil {
		log.Printf("tray: create failed: %v", err)
		return
	}
	defer t.destroy()

	if t.emitter != nil {
		t.emitter.On(anomaly.EventAlertRaised, t.handleAlert)
	}

	var msg winapi.MSG
	for {
		ret := winapi.GetMessage(&msg, 0)
		if ret == 0 || ret == -1 {
			return
		}
		winapi.TranslateMessage(&msg)
		winapi.DispatchMessage(&msg)
	}
}

// className must be unique per process.
const className = "WTM_TrayClass"

var classNamePtr *uint16
var classOnce sync.Once

// The following package-level vars are indirection points so tests can
// inject failure paths without touching the real Win32 calls. In production
// they hold the real implementations; tests reassign them and restore on
// teardown.
var (
	classInit = func() error {
		var initErr error
		classOnce.Do(func() {
			classNamePtr, initErr = windows.UTF16PtrFromString(className)
		})
		return initErr
	}
	getModuleHandleEx = func(handle uint32, moduleName *uint16, module *windows.Handle) error {
		return windows.GetModuleHandleEx(handle, moduleName, module)
	}
	createWindowEx = func(exStyle uint32, className, windowName *uint16, style uint32,
		x, y, width, height int32, parent uintptr, menu, instance uintptr,
		param unsafe.Pointer) (uintptr, error) {
		return winapi.CreateWindowEx(exStyle, className, windowName, style,
			x, y, width, height, parent, menu, instance, param)
	}
	shellNotifyIcon = func(message uint32, nid *winapi.NOTIFYICONDATAW) error {
		return winapi.ShellNotifyIcon(message, nid)
	}
	createPopupMenu = func() uintptr {
		return winapi.CreatePopupMenu()
	}
	registerClassEx = func(c *winapi.WNDCLASSEXW) (uint16, error) {
		return winapi.RegisterClassEx(c)
	}
	trackPopupMenu = func(menu uintptr, flags uint32, x, y int32, hwnd uintptr) uintptr {
		return winapi.TrackPopupMenu(menu, flags, x, y, hwnd)
	}
)

func (t *Tray) create() error {
	if err := classInit(); err != nil {
		return err
	}

	var hInstance windows.Handle
	if err := getModuleHandleEx(0, nil, &hInstance); err != nil {
		return err
	}

	wc := winapi.WNDCLASSEXW{
		Style:     0,
		WndProc:   windows.NewCallback(t.wndProc),
		Instance:  uintptr(hInstance),
		Icon:      winapi.LoadIcon(winapi.IDI_APPLICATION),
		Cursor:    winapi.LoadCursor(winapi.IDC_ARROW),
		ClassName: classNamePtr,
	}
	if _, err := registerClassEx(&wc); err != nil {
		return err
	}

	hwnd, err := createWindowEx(
		0, classNamePtr, classNamePtr, 0,
		0, 0, 0, 0,
		winapi.HWND_MESSAGE,
		0, uintptr(hInstance), nil,
	)
	if err != nil {
		return err
	}
	// hwnd is read by Stop() from other goroutines under t.mu; the write
	// here must take the same lock or the mutex discipline is meaningless.
	t.mu.Lock()
	t.hwnd = hwnd
	t.mu.Unlock()

	t.nid = winapi.NOTIFYICONDATAW{
		Wnd:             hwnd,
		ID:              1,
		Flags:           winapi.NIF_MESSAGE | winapi.NIF_ICON | winapi.NIF_TIP,
		CallbackMessage: winapi.WM_TRAYICON,
		Icon:            winapi.LoadIcon(winapi.IDI_APPLICATION),
	}
	winapi.SetTipString(t.nid.Tip[:], "Windows Task Manager")
	if err := shellNotifyIcon(winapi.NIM_ADD, &t.nid); err != nil {
		return fmt.Errorf("tray: NIM_ADD: %w", err)
	}
	return nil
}

func (t *Tray) destroy() {
	if t.hwnd == 0 {
		return
	}
	_ = winapi.ShellNotifyIcon(winapi.NIM_DELETE, &t.nid)
	winapi.DestroyWindow(t.hwnd)
	t.mu.Lock()
	t.hwnd = 0
	t.mu.Unlock()
}

// wndProc is the window procedure callback. Must match WNDPROC signature.
func (t *Tray) wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch lowWordUint32(msg) {
	case winapi.WM_TRAYICON:
		switch lowWordUint32(lParam) {
		case winapi.WM_LBUTTONDBLCLK, winapi.WM_LBUTTONUP:
			t.openDashboard()
		case winapi.WM_RBUTTONUP:
			t.showMenu()
		}
		return 0
	case winapi.WM_COMMAND:
		switch uintptr(wParam & 0xFFFF) {
		case idOpenDashboard:
			t.openDashboard()
		case idOpenConfig:
			t.openConfig()
		case idQuit:
			t.quit()
		}
		return 0
	case winapi.WM_DESTROY:
		winapi.PostQuitMessage(0)
		return 0
	}
	return winapi.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (t *Tray) showMenu() {
	menu := createPopupMenu()
	if menu == 0 {
		return
	}
	defer winapi.DestroyMenu(menu)
	_ = winapi.AppendMenu(menu, 0, idOpenDashboard, "Open Dashboard")
	_ = winapi.AppendMenu(menu, 0, idOpenConfig, "Open Config Folder")
	_ = winapi.AppendMenu(menu, 0x800 /*MF_SEPARATOR*/, 0, "")
	_ = winapi.AppendMenu(menu, 0, idQuit, "Quit")

	pt, _ := winapi.GetCursorPos()
	winapi.SetForegroundWindow(t.hwnd)
	cmd := trackPopupMenu(
		menu,
		winapi.TPM_LEFTALIGN|winapi.TPM_RIGHTBUTTON|winapi.TPM_RETURNCMD,
		pt.X, pt.Y, t.hwnd,
	)
	if cmd != 0 {
		winapi.PostMessage(t.hwnd, winapi.WM_COMMAND, cmd, 0)
	}
}

func (t *Tray) openDashboard() {
	if err := winapi.ShellExecute("open", t.dashURL, "", "", winapi.SW_SHOWNORMAL); err != nil {
		log.Printf("tray: open dashboard: %v", err)
	}
}

func (t *Tray) openConfig() {
	if t.configPath == "" {
		return
	}
	_ = winapi.ShellExecute("open", t.configPath, "", "", winapi.SW_SHOWNORMAL)
}

func (t *Tray) quit() {
	if t.onQuit != nil {
		t.onQuit()
	}
	winapi.PostQuitMessage(0)
}

// handleAlert is invoked by the event emitter on every raised alert.
func (t *Tray) handleAlert(data any) {
	alert, ok := data.(anomaly.Alert)
	if !ok {
		return
	}
	t.mu.Lock()
	cfg := t.cfg
	last := t.lastBalloon
	t.mu.Unlock()

	if !cfg.Notifications.TrayBalloon {
		return
	}
	if !severityAtLeast(alert.Severity, cfg.Notifications.BalloonMinSeverity) {
		return
	}
	if time.Since(last) < cfg.Notifications.BalloonRateLimit {
		return
	}

	t.mu.Lock()
	t.lastBalloon = time.Now()
	t.mu.Unlock()

	t.showBalloon(alert.Title, alert.Description, balloonIcon(alert.Severity))
}

func severityAtLeast(s anomaly.Severity, threshold string) bool {
	rank := func(x string) int {
		switch x {
		case "info":
			return 0
		case "warning":
			return 1
		case "critical":
			return 2
		}
		return 0
	}
	return rank(string(s)) >= rank(threshold)
}

func balloonIcon(s anomaly.Severity) uint32 {
	switch s {
	case anomaly.SeverityCritical:
		return winapi.NIIF_ERROR
	case anomaly.SeverityWarning:
		return winapi.NIIF_WARNING
	}
	return winapi.NIIF_INFO
}

func (t *Tray) showBalloon(title, body string, icon uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nid.Flags |= winapi.NIF_INFO
	t.nid.InfoFlags = icon
	copyToFixed(t.nid.InfoTitle[:], title)
	copyToFixed(t.nid.Info[:], body)
	if err := winapi.ShellNotifyIcon(winapi.NIM_MODIFY, &t.nid); err != nil {
		log.Printf("tray: balloon: %v", err)
	}
}

// copyToFixed copies s into a fixed-size Win32 UTF-16 buffer, always
// leaving a NUL terminator: on truncation the final slot is reserved for
// the terminator instead of being filled with data. An unterminated
// szInfo/szInfoTitle makes Shell_NotifyIconW scan past the field into
// adjacent NOTIFYICONDATAW memory.
func copyToFixed(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	limit := len(dst) - 1 // reserve the final slot for the terminator
	src := windows.StringToUTF16(s)
	if len(src) > limit {
		copy(dst, src[:limit])
		dst[limit] = 0
		return
	}
	// src fits and already carries its own NUL terminator.
	copy(dst, src)
}

// Compile-time check that we use unsafe (silences linters that may flag the
// import as unused via aliasing — currently no direct usages).
var _ = unsafe.Sizeof(0)

func lowWordUint32(v uintptr) uint32 {
	// #nosec G115 -- Windows message parameters are 32-bit values.
	return uint32(v & 0xFFFFFFFF)
}
