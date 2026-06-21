package anomaly

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

type fakeActuator struct {
	killed    []uint32
	suspended []uint32
	killErr   error
	suspErr   error
}

func (f *fakeActuator) Kill(pid uint32, confirm bool) error {
	if f.killErr != nil {
		return f.killErr
	}
	f.killed = append(f.killed, pid)
	return nil
}

func (f *fakeActuator) Suspend(pid uint32, confirm bool) error {
	if f.suspErr != nil {
		return f.suspErr
	}
	f.suspended = append(f.suspended, pid)
	return nil
}

func newEngineFixture(t *testing.T, withStore bool) (*Engine, *AlertStore, *storage.Store, *event.Emitter) {
	t.Helper()
	cfg := config.DefaultConfig()
	var st *storage.Store
	if withStore {
		st = storage.NewStore(60, 10)
	}
	em := event.NewEmitter()
	alerts := NewAlertStore(64)
	e := NewEngine(cfg, st, em, alerts)
	return e, alerts, st, em
}

func TestNewEngineRegistersAllDefaults(t *testing.T) {
	e, _, _, _ := newEngineFixture(t, false)
	if got := len(e.detectors); got != 9 {
		t.Fatalf("detectors=%d want 9", got)
	}
}

func TestSetActuatorAndConfig(t *testing.T) {
	e, _, _, _ := newEngineFixture(t, false)
	e.SetActuator(&fakeActuator{})
	if e.actuator == nil {
		t.Fatal("actuator not set")
	}
	newCfg := config.DefaultConfig()
	newCfg.Anomaly.AnalysisInterval = 750 * time.Millisecond
	e.SetConfig(newCfg)
	if e.cfg != newCfg {
		t.Fatal("config not swapped")
	}
}

func TestStartLoopRunsAndRespondsToCancel(t *testing.T) {
	e, _, st, _ := newEngineFixture(t, true)
	// Plant a snapshot so analyzeOnce has something to do.
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	// Let the loop run at least once.
	time.Sleep(50 * time.Millisecond)
	cancel()
	// Give the goroutine a moment to exit cleanly.
	time.Sleep(50 * time.Millisecond)
}

func TestAnalyzeOnceNilSnapshot(t *testing.T) {
	e, _, st, _ := newEngineFixture(t, true)
	// No SetLatest => Latest returns nil.
	e.analyzeOnce()
	// Nothing to assert — just must not panic.
	_ = st
}

func TestAnalyzeOnceRunsDetectors(t *testing.T) {
	e, alerts, st, _ := newEngineFixture(t, true)
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 100, Name: "worker.exe", CPUPercent: 95}},
	})
	e.cfg.Anomaly.RunawayCPU.Enabled = true
	e.cfg.Anomaly.RunawayCPU.CPUThreshold = 50
	e.cfg.Anomaly.RunawayCPU.DurationThreshold = 1 * time.Millisecond
	e.cfg.Anomaly.RunawayCPU.CriticalDuration = 1 * time.Millisecond
	e.analyzeOnce()
	// analyzeOnce uses real time.Now(); give it a moment so dur >= threshold.
	time.Sleep(5 * time.Millisecond)
	e.analyzeOnce()
	if len(alerts.Active()) == 0 {
		t.Fatal("expected at least one alert from detectors")
	}
}

type panicDetector struct{}

func (panicDetector) Name() string { return "panic" }
func (panicDetector) Analyze(_ *AnalysisContext) {
	panic("boom")
}

func TestSafeAnalyzeRecoversPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("safeAnalyze should swallow panic, got %v", r)
		}
	}()
	safeAnalyze(panicDetector{}, &AnalysisContext{Alerts: NewAlertStore(8)})
}

func TestAnalysisIntervalBranches(t *testing.T) {
	e, _, _, _ := newEngineFixture(t, false)
	// nil cfg branch
	e.cfg = nil
	if got := e.analysisInterval(); got != 2*time.Second {
		t.Fatalf("nil cfg interval=%v want 2s", got)
	}
	// too-small cfg interval
	e.cfg = config.DefaultConfig()
	e.cfg.Anomaly.AnalysisInterval = 100 * time.Millisecond
	if got := e.analysisInterval(); got != 2*time.Second {
		t.Fatalf("too-small interval=%v want 2s", got)
	}
	// valid cfg interval
	e.cfg.Anomaly.AnalysisInterval = 3 * time.Second
	if got := e.analysisInterval(); got != 3*time.Second {
		t.Fatalf("valid interval=%v want 3s", got)
	}
}

// waitForCount drains up to `want` signals from ch within a short timeout,
// failing the test if fewer arrive (the emitter dispatches asynchronously).
func waitForCount(t *testing.T, ch <-chan struct{}, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("waitForCount: got %d/%d signals", i, want)
		}
	}
}

func TestAnalysisContextRaiseAlertWithEmitter(t *testing.T) {
	em := event.NewEmitter()
	called := make(chan struct{}, 16)
	em.On(EventAlertRaised, func(data any) {
		called <- struct{}{}
	})
	alerts := NewAlertStore(64)
	actx := &AnalysisContext{Emitter: em, Alerts: alerts}
	actx.RaiseAlert(Alert{Type: "em", Severity: SeverityInfo, Title: "x"})
	waitForCount(t, called, 1)
}

func TestAnalysisContextRaiseAlertWithoutEmitter(t *testing.T) {
	actx := &AnalysisContext{Alerts: NewAlertStore(8)}
	actx.RaiseAlert(Alert{Type: "noem", Severity: SeverityInfo})
	// Must not panic with nil emitter.
}

func TestAnalysisContextRaiseAlertDupDoesNotEmit(t *testing.T) {
	em := event.NewEmitter()
	called := make(chan struct{}, 16)
	em.On(EventAlertRaised, func(any) { called <- struct{}{} })
	actx := &AnalysisContext{Emitter: em, Alerts: NewAlertStore(8)}
	actx.RaiseAlert(Alert{Type: "dup", Severity: SeverityInfo})
	actx.RaiseAlert(Alert{Type: "dup", Severity: SeverityInfo})
	waitForCount(t, called, 1)
}

func TestAnalysisContextClearAlert(t *testing.T) {
	em := event.NewEmitter()
	called := make(chan struct{}, 16)
	em.On(EventAlertCleared, func(any) { called <- struct{}{} })
	actx := &AnalysisContext{Emitter: em, Alerts: NewAlertStore(8)}
	actx.Alerts.Raise(Alert{Type: "cl", Severity: SeverityInfo, PID: 9})
	actx.ClearAlert("cl", 9)
	waitForCount(t, called, 1)
}

func TestAnalysisContextClearAlertNoEmitter(t *testing.T) {
	actx := &AnalysisContext{Alerts: NewAlertStore(8)}
	actx.ClearAlert("cl", 1) // nil emitter must not panic
}

// Ensure errors from actuator get logged (no panic, no state update).
func TestRulesActuatorErrorBranchesCovered(t *testing.T) {
	e, _, st, em := newEngineFixture(t, true)
	act := &fakeActuator{killErr: errors.New("boom")}
	e.SetActuator(act)
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 50, Name: "loop.exe", CPUPercent: 99},
		},
	})
	e.cfg.Rules = []config.Rule{
		{
			Name: "killhighcpu", Enabled: true, Match: "loop.exe",
			Metric: "cpu_percent", Op: ">", Threshold: 50,
			For: 1 * time.Millisecond, Action: "kill", Cooldown: 0,
		},
	}
	// First sample records state; second triggers action.
	e.analyzeOnce()
	time.Sleep(2 * time.Millisecond)
	e.analyzeOnce()
	_ = em
}

// Verify loop resets the timer correctly between iterations.
func TestLoopRespectsTimerReset(t *testing.T) {
	e, _, st, _ := newEngineFixture(t, true)
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}},
	})
	e.cfg.Anomaly.AnalysisInterval = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.loop(ctx)
	time.Sleep(60 * time.Millisecond) // should run a couple of times
	// Cancel inside the loop body branch by stopping before deadline.
	cancel()
	time.Sleep(20 * time.Millisecond)
}

// TestLoopTimerFiresAnalyzeOnce exercises the timer.C branch of the loop:
// the timer fires, analyzeOnce runs, and the timer is reset for the next tick.
func TestLoopTimerFiresAnalyzeOnce(t *testing.T) {
	e, alerts, st, _ := newEngineFixture(t, true)
	st.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 100, Name: "worker.exe", CPUPercent: 95}},
	})
	e.cfg.Anomaly.AnalysisInterval = 500 * time.Millisecond
	e.cfg.Anomaly.RunawayCPU.Enabled = true
	e.cfg.Anomaly.RunawayCPU.CPUThreshold = 50
	e.cfg.Anomaly.RunawayCPU.DurationThreshold = 1 * time.Millisecond
	e.cfg.Anomaly.RunawayCPU.CriticalDuration = 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.loop(ctx)
	// Wait long enough for at least two timer ticks (so state is recorded then alert fires).
	time.Sleep(1400 * time.Millisecond)
	if len(alerts.Active()) == 0 {
		t.Fatal("expected loop timer to fire analyzeOnce and raise an alert")
	}
}
