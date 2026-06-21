package server

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/event"
)

func TestStartCallsListenAndServeAndFails(t *testing.T) {
	// Make Start bind to a port that's already in use so ListenAndServe
	// returns immediately instead of blocking.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	cfg := defaultCfg()
	cfg.Server.Host = host
	cfg.Server.Port = port
	s, err := New(Options{Emitter: event.NewEmitter(), Cfg: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- s.Start()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected port-already-in-use error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}
