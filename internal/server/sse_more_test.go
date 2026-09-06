package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/event"
)

func TestSSEHubBroadcastMarshalFailure(t *testing.T) {
	// channels can't be JSON-marshalled; should silently drop.
	hub := &SSEHub{
		clients: map[uint64]*sseClient{
			1: {id: 1, send: make(chan sseMsg, 1), closed: make(chan struct{})},
		},
	}
	hub.broadcast("bad", make(chan int))
	// No client should receive anything.
	select {
	case msg := <-hub.clients[1].send:
		t.Fatalf("unexpected msg: %v", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSSEHubBroadcastSkipsSlowClients(t *testing.T) {
	// Use a buffered channel of 0 so the default branch hits.
	hub := &SSEHub{
		clients: map[uint64]*sseClient{
			1: {id: 1, send: make(chan sseMsg), closed: make(chan struct{})},
		},
	}
	hub.broadcast("snap", map[string]any{"a": 1})
	// No panic, no blocking.
	time.Sleep(20 * time.Millisecond)
}

func TestNewSSEHubSubscribesViaEmitter(t *testing.T) {
	em := event.NewEmitter()
	hub := NewSSEHub(em)
	if hub == nil {
		t.Fatal("expected hub")
	}
	// Register a client so the broadcast has somewhere to go.
	hub.clients[1] = &sseClient{id: 1, send: make(chan sseMsg, 1), closed: make(chan struct{})}
	em.Emit("test", map[string]any{"x": 1})
	// emitter dispatches via goroutines; give it a moment.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case msg := <-hub.clients[1].send:
			if msg.event != "test" {
				t.Errorf("event=%q", msg.event)
			}
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Fatal("broadcast did not arrive")
}

func TestSSEHandlerNonFlusher(t *testing.T) {
	// Use a wrapper that explicitly does NOT expose http.Flusher through
	// its outer type (so the type assertion fails immediately).
	hub := NewSSEHub(nil)
	rr := &nonFlusherWriter{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	hub.Handler()(rr, req)
	if rr.code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.code)
	}
	if !strings.Contains(rr.body, "streaming unsupported") {
		t.Errorf("body=%s", rr.body)
	}
}

// nonFlusherWriter is an http.ResponseWriter that does NOT implement
// http.Flusher. We can't embed httptest.ResponseRecorder here because it
// exposes Flush through the embedded pointer.
type nonFlusherWriter struct {
	code      int
	body      string
	headerMap http.Header
}

func (n *nonFlusherWriter) Header() http.Header {
	if n.headerMap == nil {
		n.headerMap = make(http.Header)
	}
	return n.headerMap
}
func (n *nonFlusherWriter) Write(b []byte) (int, error) {
	n.body += string(b)
	return len(b), nil
}
func (n *nonFlusherWriter) WriteHeader(code int) { n.code = code }

// lockedRecorder synchronizes writes coming from the SSE handler goroutine
// with reads from the test goroutine: httptest.ResponseRecorder is not safe
// for concurrent use, so polling a live recorder's body is a data race.
type lockedRecorder struct {
	mu sync.Mutex
	rr *httptest.ResponseRecorder
}

func (l *lockedRecorder) Header() http.Header { return l.rr.Header() }

func (l *lockedRecorder) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rr.Write(b)
}

func (l *lockedRecorder) WriteHeader(code int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rr.WriteHeader(code)
}

func (l *lockedRecorder) Flush() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rr.Flush()
}

func (l *lockedRecorder) body() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rr.Body.String()
}

// TestSSEHandlerHeartbeat covers the heartbeat branch in the SSE handler
// (line 2825 in coverage). The handler's heartbeat ticker fires every 25s,
// so this test takes ~26 seconds to run. Skipped by default in -short mode.
func TestSSEHandlerHeartbeat(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heartbeat test in short mode")
	}
	hub := NewSSEHub(nil)
	rec := &lockedRecorder{rr: httptest.NewRecorder()}
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		hub.Handler()(rec, req)
		close(done)
	}()
	// Wait for client to register and the hello to be flushed.
	waitForClientCount(t, hub, 1)
	// Wait for the heartbeat (every 25s) to fire at least once.
	deadline := time.Now().Add(27 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rec.body(), ": ping") {
			cancel()
			<-done
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatal("heartbeat ping not observed within 27s")
}

func TestSSEHandlerSendsEvent(t *testing.T) {
	hub := NewSSEHub(nil)
	rr := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		hub.Handler()(rr, req)
		close(done)
	}()
	// Wait for client to register.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if hub.ClientCount() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Broadcast a message.
	hub.broadcast("ping", map[string]any{"hello": "world"})
	// Cancel and wait for handler to return.
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not exit")
	}
	// Body should contain hello event.
	body := rr.Body.String()
	if !strings.Contains(body, "ping") {
		t.Errorf("body missing ping: %s", body)
	}
	if !strings.Contains(body, "hello") {
		t.Errorf("body missing hello: %s", body)
	}
}

// nonFlusherRecorder wraps httptest.ResponseRecorder and explicitly omits
// http.Flusher (the default already does, but be explicit).
type nonFlusherRecorder struct {
	*httptest.ResponseRecorder
}

func TestSanitizeLogTokenNilSafe(t *testing.T) {
	// Pass an empty string and a tab character.
	got := sanitizeLogToken("\t\x00")
	if got != "" {
		t.Errorf("expected empty after control char strip, got %q", got)
	}
}

func TestWriteErrorHasCorrectShape(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusTeapot, "MY_CODE", "my message")
	if rr.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"code":"MY_CODE"`) {
		t.Errorf("body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"message":"my message"`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestWriteJSONEncoderErrorSwallowed(t *testing.T) {
	// Channels can't be encoded, which exercises the _ = err branch.
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, make(chan int))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

// Write bytes directly so we can confirm the header was set.
func TestWriteJSONSetsContentTypeBeforeWrite(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]any{"k": "v"})
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Errorf("content-type=%q", got)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"k":"v"`)) {
		t.Errorf("body=%s", rr.Body.String())
	}
}
