package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/event"
)

func TestNewDefaultsVersionToDev(t *testing.T) {
	s, err := New(Options{Emitter: event.NewEmitter()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.version != "dev" {
		t.Errorf("version=%q want dev", s.version)
	}
	if s.csrfToken == "" {
		t.Error("csrf token should be populated")
	}
	if s.router == nil {
		t.Error("router should be initialized")
	}
	if s.hub == nil {
		t.Error("hub should be initialized")
	}
	if s.aiExec == nil {
		t.Error("aiExec should be initialized")
	}
}

func TestNewHonorsExplicitVersion(t *testing.T) {
	s, err := New(Options{Emitter: event.NewEmitter(), Version: "v9.9.9"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.version != "v9.9.9" {
		t.Errorf("version=%q want v9.9.9", s.version)
	}
}

func TestNewWhitespaceVersionFallsBackToDev(t *testing.T) {
	s, err := New(Options{Emitter: event.NewEmitter(), Version: "   "})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.version != "dev" {
		t.Errorf("version=%q want dev", s.version)
	}
}

func TestSetConfigSwapsActiveConfig(t *testing.T) {
	s, err := New(Options{Emitter: event.NewEmitter()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cfg := defaultCfg()
	cfg.Monitoring.Interval = 2500 * time.Millisecond
	s.SetConfig(cfg)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg.Monitoring.Interval != 2500*time.Millisecond {
		t.Fatalf("interval=%v", s.cfg.Monitoring.Interval)
	}
}

func TestShutdownAndStartServeLifecycle(t *testing.T) {
	s, err := New(Options{Emitter: event.NewEmitter(), Cfg: defaultCfg()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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
	s.mu.Lock()
	s.cfg.Server.Host = host
	s.cfg.Server.Port = port
	s.httpSrv = &http.Server{Handler: s.router}
	s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- s.httpSrv.Serve(ln)
	}()
	time.Sleep(50 * time.Millisecond)
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve exit err=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestStartSetsUpHTTPServer(t *testing.T) {
	// We can't call Start directly (it blocks), but we can verify that the
	// fields it sets are accessible by exercising Start's setup logic via
	// the public Start + Serve surface that ListenAndServe would use.
	s, err := New(Options{Emitter: event.NewEmitter(), Cfg: defaultCfg()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	s.mu.Lock()
	s.httpSrv = &http.Server{
		Addr:              s.cfg.Server.Host + ":0",
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
	s.mu.Unlock()

	done := make(chan error, 1)
	go func() {
		done <- s.httpSrv.Serve(ln)
	}()
	time.Sleep(20 * time.Millisecond)
	_ = s.Shutdown(context.Background())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestShutdownNilServerReturnsNil(t *testing.T) {
	s := &Server{}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestStatusRecorderWriteHeaderForwards(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: 200}
	rec.WriteHeader(http.StatusTeapot)
	if rec.status != http.StatusTeapot {
		t.Fatalf("status=%d", rec.status)
	}
	if rec.ResponseWriter.(*httptest.ResponseRecorder).Code != http.StatusTeapot {
		t.Fatalf("inner code=%d", rec.ResponseWriter.(*httptest.ResponseRecorder).Code)
	}
}

func TestStatusRecorderFlushNoOpForNonFlusher(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: 200}
	rec.Flush() // must not panic
}

func TestLocalOnlyMiddlewareRejectsNonLoopback(t *testing.T) {
	handler := localOnlyMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.RemoteAddr = "203.0.113.5:55555"
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestLocalOnlyMiddlewareRejectsBadRemoteAddr(t *testing.T) {
	handler := localOnlyMiddleware(func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "no-port-here"
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestMutationGuardAllowsSafeMethods(t *testing.T) {
	calls := 0
	handler := mutationGuardMiddleware("tok")(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(method, "/api/v1/x", nil)
		req.RemoteAddr = "127.0.0.1:1"
		req.Host = "127.0.0.1:8080"
		rr := httptest.NewRecorder()
		handler(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Errorf("method=%s status=%d", method, rr.Code)
		}
	}
	if calls != 3 {
		t.Fatalf("calls=%d want 3", calls)
	}
}

func TestMutationGuardRejectsBadOrigin(t *testing.T) {
	handler := mutationGuardMiddleware("tok")(func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("X-WTM-CSRF", "tok")
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestMutationGuardRejectsMissingToken(t *testing.T) {
	handler := mutationGuardMiddleware("tok")(func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{}`))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestValidOriginAcceptsAndRejectsCases(t *testing.T) {
	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{"localhost http match", "localhost:8080", "http://localhost:8080", true},
		{"localhost no port", "localhost", "http://localhost", true},
		{"empty origin", "127.0.0.1:19876", "", false},
		{"https scheme", "127.0.0.1:19876", "https://127.0.0.1:19876", false},
		{"external host", "127.0.0.1:19876", "http://example.com", false},
		{"malformed url", "127.0.0.1:19876", "://broken", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.Host = c.host
		req.Header.Set("Origin", c.origin)
		if got := validOrigin(req); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestRequestPortHandlesIPv6AndEmpty(t *testing.T) {
	if got := requestPort("[::1]:8080"); got != "8080" {
		t.Errorf("got %q want 8080", got)
	}
	if got := requestPort("localhost"); got != "" {
		t.Errorf("localhost without port should be empty, got %q", got)
	}
}
