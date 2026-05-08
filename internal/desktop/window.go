//go:build windows

package desktop

import (
	"log"

	"github.com/jchv/go-webview2"
)

type Window struct {
	wv   webview2.WebView
	done func()
}

func New(url, title string, width, height int, onDone func()) *Window {
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
	w.SetSize(width, height, webview2.HintNone)

	win := &Window{wv: w, done: onDone}
	w.Bind("__wtmDesktopClose", win.close)
	w.Navigate(url)
	return win
}

func (w *Window) close() {
	if w.done != nil {
		w.done()
	}
	w.done = nil
	w.wv.Destroy()
}

func (w *Window) Run() {
	if w == nil || w.wv == nil {
		return
	}
	w.wv.Run()
}
