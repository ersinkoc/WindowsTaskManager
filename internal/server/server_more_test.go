package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/event"
)

func TestParsePIDParamHandlesSuccessAndErrors(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/42", nil)
	req = req.WithContext(context.WithValue(req.Context(), paramKey{}, Params{"pid": "42"}))
	rr := httptest.NewRecorder()

	pid, ok := parsePIDParam(rr, req)
	if !ok || pid != 42 {
		t.Fatalf("pid=%d ok=%v want 42,true", pid, ok)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/processes", nil)
	missingRR := httptest.NewRecorder()
	if _, ok := parsePIDParam(missingRR, missingReq); ok {
		t.Fatal("expected missing pid to fail")
	}
	if missingRR.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", missingRR.Code, http.StatusBadRequest)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/processes/nope", nil)
	invalidReq = invalidReq.WithContext(context.WithValue(invalidReq.Context(), paramKey{}, Params{"pid": "nope"}))
	invalidRR := httptest.NewRecorder()
	if _, ok := parsePIDParam(invalidRR, invalidReq); ok {
		t.Fatal("expected invalid pid to fail")
	}
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d", invalidRR.Code, http.StatusBadRequest)
	}
}

func TestRequestPortAndSamePortDefaultTo80(t *testing.T) {
	if got := requestPort("127.0.0.1:19876"); got != "19876" {
		t.Fatalf("requestPort returned %q", got)
	}
	if got := requestPort("localhost"); got != "" {
		t.Fatalf("requestPort(%q)=%q want empty", "localhost", got)
	}
	if !samePort("", "") {
		t.Fatal("empty ports should default to 80 and match")
	}
	if samePort("8080", "19876") {
		t.Fatal("different ports should not match")
	}
}

func TestShutdownReturnsNilWithoutServer(t *testing.T) {
	s := &Server{}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestStatusRecorderFlushForwards(t *testing.T) {
	writer := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	rec := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}

	rec.Flush()

	if writer.flushes != 1 {
		t.Fatalf("flushes=%d want 1", writer.flushes)
	}
}

func TestSSEHubBroadcastSendsPayloadToClients(t *testing.T) {
	hub := &SSEHub{
		clients: map[uint64]*sseClient{
			1: {id: 1, send: make(chan sseMsg, 1), closed: make(chan struct{})},
		},
	}

	hub.broadcast("snapshot", map[string]any{"ok": true})

	select {
	case msg := <-hub.clients[1].send:
		if msg.event != "snapshot" {
			t.Fatalf("event=%q want snapshot", msg.event)
		}
		if !strings.Contains(string(msg.data), `"ok":true`) {
			t.Fatalf("payload=%q", msg.data)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("broadcast did not reach client")
	}
}

func TestSSEHubHandlerSendsHelloAndCleansUp(t *testing.T) {
	hub := NewSSEHub(event.NewEmitter())
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil).WithContext(ctx)
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		hub.Handler()(rr, req)
		close(done)
	}()

	waitForClientCount(t, hub, 1)
	cancel()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("SSE handler did not exit after context cancellation")
	}

	if !strings.Contains(rr.Body.String(), "event: hello") {
		t.Fatalf("body=%q want hello event", rr.Body.String())
	}
	if hub.ClientCount() != 0 {
		t.Fatalf("client_count=%d want 0", hub.ClientCount())
	}
}

func TestNewSSEHubAllowsNilEmitter(t *testing.T) {
	hub := NewSSEHub(nil)
	if hub == nil {
		t.Fatal("expected hub")
	}
	if hub.ClientCount() != 0 {
		t.Fatalf("client_count=%d want 0", hub.ClientCount())
	}
}

func TestSanitizeLogTokenRemovesControlChars(t *testing.T) {
	got := sanitizeLogToken("GET\n/api/v1\r/system")
	if strings.ContainsAny(got, "\r\n\t") {
		t.Fatalf("sanitized token still contains control chars: %q", got)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushRecorder) Flush() {
	f.flushes++
}

func waitForClientCount(t *testing.T, hub *SSEHub, want int) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if hub.ClientCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("client_count=%d want %d", hub.ClientCount(), want)
}

// TestNewCSRFSafeTokenError exercises the rand.Read failure branch of
// newCSRFSafeToken by swapping secureRandomRead with a deterministic
// error-returning function. This is the only way to cover the branch
// in normal testing — the real crypto/rand.Read does not fail on any
// supported platform.
func TestNewCSRFSafeTokenError(t *testing.T) {
	saved := secureRandomRead
	defer func() { secureRandomRead = saved }()

	secureRandomRead = func([]byte) (int, error) {
		return 0, errors.New("simulated entropy failure")
	}

	token, err := newCSRFSafeToken()
	if err == nil {
		t.Fatalf("expected error, got token=%q", token)
	}
	if token != "" {
		t.Errorf("token=%q want empty on error", token)
	}
	if !strings.Contains(err.Error(), "simulated entropy failure") {
		t.Errorf("err=%v want wrap of simulated failure", err)
	}
}

// TestNewReturnsErrorWhenCSRFFails covers the New() error path triggered
// when newCSRFSafeToken cannot generate a token. With secureRandomRead
// stubbed to fail, New must return the wrapped error rather than a
// partially-constructed *Server.
func TestNewReturnsErrorWhenCSRFFails(t *testing.T) {
	saved := secureRandomRead
	defer func() { secureRandomRead = saved }()

	secureRandomRead = func([]byte) (int, error) {
		return 0, errors.New("entropy denied")
	}

	s, err := New(Options{Version: "test"})
	if err == nil {
		t.Fatalf("expected error from New, got server=%+v", s)
	}
	if s != nil {
		t.Errorf("server=%+v want nil on error", s)
	}
	if !strings.Contains(err.Error(), "entropy denied") {
		t.Errorf("err=%v want wrap of entropy denied", err)
	}
}
