package event

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewEmitter(t *testing.T) {
	e := NewEmitter()
	if e == nil {
		t.Fatal("NewEmitter returned nil")
	}
	if e.subs == nil {
		// subs is nil-slice until first Subscribe, that's fine
		// but typedOn must be initialized
	}
	if e.typedOn == nil {
		t.Fatal("typedOn map was not initialized")
	}
	if e.sem == nil {
		t.Fatal("sem channel was not initialized")
	}
	if cap(e.sem) != 64 {
		t.Fatalf("sem capacity = %d, want 64", cap(e.sem))
	}
}

func TestSubscribeReceivesAllEvents(t *testing.T) {
	e := NewEmitter()
	type captured struct {
		eventType string
		data      any
	}
	got := make(map[string]any)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(3)

	e.Subscribe(func(eventType string, data any) {
		mu.Lock()
		got[eventType] = data
		mu.Unlock()
		wg.Done()
	})

	e.Emit("a", 1)
	e.Emit("b", "hello")
	e.Emit("c", nil)

	waitOrFail(t, &wg, time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("got %d distinct events, want 3 (%v)", len(got), got)
	}
	if got["a"] != 1 {
		t.Errorf("a payload = %v, want 1", got["a"])
	}
	if got["b"] != "hello" {
		t.Errorf("b payload = %v, want %q", got["b"], "hello")
	}
	if v, ok := got["c"]; !ok || v != nil {
		t.Errorf("c payload = %v (present=%v), want <nil>", v, ok)
	}
}

func TestSubscribeIsConcurrentSafe(t *testing.T) {
	e := NewEmitter()
	const n = 100
	var counter int64
	for i := 0; i < n; i++ {
		e.Subscribe(func(eventType string, data any) {
			atomic.AddInt64(&counter, 1)
		})
	}
	e.Emit("x", nil)
	// Wait for all goroutines
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&counter) == n {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := atomic.LoadInt64(&counter); got != n {
		t.Fatalf("counter = %d, want %d", got, n)
	}
}

func TestOnInvokesTypedHandler(t *testing.T) {
	e := NewEmitter()
	var got any
	var mu sync.Mutex
	done := make(chan struct{}, 1)

	e.On("hello", func(data any) {
		mu.Lock()
		got = data
		mu.Unlock()
		done <- struct{}{}
	})

	e.Emit("hello", "world")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("typed handler was not invoked")
	}

	mu.Lock()
	defer mu.Unlock()
	if got != "world" {
		t.Fatalf("data = %v, want %q", got, "world")
	}
}

func TestOnFiltersByEventType(t *testing.T) {
	e := NewEmitter()
	var helloCount, otherCount int64
	var wg sync.WaitGroup
	// 2 hello emits + 1 other emit = 3 total invocations
	wg.Add(3)

	e.On("hello", func(data any) {
		atomic.AddInt64(&helloCount, 1)
		wg.Done()
	})
	e.On("other", func(data any) {
		atomic.AddInt64(&otherCount, 1)
		wg.Done()
	})

	e.Emit("hello", nil)
	e.Emit("hello", nil)
	e.Emit("other", nil)

	waitOrFail(t, &wg, time.Second)

	if got := atomic.LoadInt64(&helloCount); got != 2 {
		t.Errorf("helloCount = %d, want 2", got)
	}
	if got := atomic.LoadInt64(&otherCount); got != 1 {
		t.Errorf("otherCount = %d, want 1", got)
	}
}

func TestOnMultipleHandlersSameType(t *testing.T) {
	e := NewEmitter()
	var a, b int64
	var wg sync.WaitGroup
	wg.Add(2)

	e.On("evt", func(data any) {
		atomic.AddInt64(&a, 1)
		wg.Done()
	})
	e.On("evt", func(data any) {
		atomic.AddInt64(&b, 1)
		wg.Done()
	})

	e.Emit("evt", 42)

	waitOrFail(t, &wg, time.Second)

	if atomic.LoadInt64(&a) != 1 || atomic.LoadInt64(&b) != 1 {
		t.Errorf("a=%d b=%d, want both 1", a, b)
	}
}

func TestEmitWithoutHandlers(t *testing.T) {
	e := NewEmitter()
	// Should not block or panic
	e.Emit("nothing", nil)
}

func TestEmitDispatchesBothSubscribersAndTypedHandlers(t *testing.T) {
	e := NewEmitter()
	var subHit, onHit int64
	var wg sync.WaitGroup
	wg.Add(2)

	e.Subscribe(func(eventType string, data any) {
		atomic.AddInt64(&subHit, 1)
		wg.Done()
	})
	e.On("evt", func(data any) {
		atomic.AddInt64(&onHit, 1)
		wg.Done()
	})

	e.Emit("evt", "payload")
	waitOrFail(t, &wg, time.Second)

	if atomic.LoadInt64(&subHit) != 1 {
		t.Errorf("subHit = %d, want 1", subHit)
	}
	if atomic.LoadInt64(&onHit) != 1 {
		t.Errorf("onHit = %d, want 1", onHit)
	}
}

func TestEmitTypedHandlerReceivesData(t *testing.T) {
	e := NewEmitter()
	type payload struct{ Value int }
	var got payload
	var mu sync.Mutex
	done := make(chan struct{}, 1)

	e.On("typed", func(data any) {
		mu.Lock()
		got = data.(payload)
		mu.Unlock()
		done <- struct{}{}
	})

	want := payload{Value: 7}
	e.Emit("typed", want)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("typed handler not invoked")
	}

	mu.Lock()
	defer mu.Unlock()
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestEmitDoesNotBlockOnSlowSubscriber(t *testing.T) {
	e := NewEmitter()
	blocked := make(chan struct{})
	release := make(chan struct{})
	e.Subscribe(func(eventType string, data any) {
		close(blocked)
		<-release
	})

	start := time.Now()
	e.Emit("test", 1)
	elapsed := time.Since(start)

	select {
	case <-blocked:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscriber was not invoked")
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Emit blocked for %s", elapsed)
	}
	close(release)
}

func TestEmitRecoversFromSubscriberPanic(t *testing.T) {
	e := NewEmitter()
	done := make(chan struct{}, 1)

	e.Subscribe(func(eventType string, data any) {
		panic("boom")
	})
	e.Subscribe(func(eventType string, data any) {
		done <- struct{}{}
	})

	e.Emit("test", 1)

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("healthy subscriber was not invoked after panic")
	}
}

func TestEmitRecoversFromTypedHandlerPanic(t *testing.T) {
	e := NewEmitter()
	done := make(chan struct{}, 1)

	e.On("evt", func(data any) {
		panic("typed boom")
	})
	e.On("evt", func(data any) {
		done <- struct{}{}
	})

	e.Emit("evt", nil)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("healthy typed handler was not invoked after panic")
	}
}

func TestDispatchSaturatedDropsNotification(t *testing.T) {
	e := NewEmitter()
	// Replace the semaphore with one of capacity 0 to force saturation immediately.
	e.sem = make(chan struct{}, 0)

	// Drain a blocking subscriber to keep the semaphore full once we add back.
	// With cap=0, every dispatch hits the default branch and logs.
	e.Subscribe(func(eventType string, data any) {})

	// This should not block and should hit the default branch.
	done := make(chan struct{})
	go func() {
		e.Emit("saturated", nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Emit blocked when dispatcher was saturated")
	}
}

// waitOrFail blocks until wg is done or fails the test on timeout.
func waitOrFail(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for handlers")
	}
}
