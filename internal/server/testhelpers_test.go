package server

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// defaultCfg returns the project's default config — convenient shorthand.
func defaultCfg() *config.Config { return config.DefaultConfig() }

// fullTestServer builds a Server wired with the full dependency set.
func fullTestServer(t *testing.T, cfgPath string, cfg *config.Config, store *storage.Store) (*Server, *[]*config.Config) {
	t.Helper()
	if cfg == nil {
		cfg = defaultCfg()
	}
	if store == nil {
		store = storage.NewStore(60, 10)
	}
	var applied []*config.Config
	s, err := New(Options{
		Cfg:        cfg,
		CfgPath:    cfgPath,
		Store:      store,
		Controller: controller.NewController(cfg, store, event.NewEmitter()),
		Alerts:     anomaly.NewAlertStore(64),
		Emitter:    event.NewEmitter(),
		Version:    "test-version",
		OnCfgApply: func(c *config.Config) {
			applied = append(applied, c)
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, &applied
}

// pingSpawn starts a long-running ping child and returns its PID plus a
// cleanup function. Mirrors controller_extra_test.go's helper.
func pingSpawn(t *testing.T, seconds int) (uint32, func()) {
	t.Helper()
	count := seconds + 30
	cmd := exec.Command("ping", "-n", strconv.Itoa(count), "127.0.0.1")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn ping: %v", err)
	}
	pid := uint32(cmd.Process.Pid)
	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return pid, cleanup
}

// setStoreSnap installs a snapshot with the supplied processes.
func setStoreSnap(t *testing.T, store *storage.Store, procs ...metrics.ProcessInfo) {
	t.Helper()
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: procs,
	})
}

// paramCtx decorates a request with Params in its context.
func paramCtx(req *http.Request, params map[string]string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), paramKey{}, Params(params)))
}

// analyzeStubAdvisor satisfies AIAdvisor with controllable Analyze / Chat
// functions and an explicit BackgroundState.
type analyzeStubAdvisor struct {
	enabled bool
	chatFn  func(message string) (string, error)
	anFn    func(prompt string) (string, error)
	bg      ai.BackgroundState
}

func (s analyzeStubAdvisor) Enabled() bool { return s.enabled }
func (s analyzeStubAdvisor) Status() map[string]any {
	return map[string]any{"enabled": s.enabled, "ok": true}
}
func (s analyzeStubAdvisor) Analyze(_ context.Context, prompt string) (*ai.AnalyzeResult, error) {
	if s.anFn == nil {
		return &ai.AnalyzeResult{Answer: "unused"}, nil
	}
	ans, err := s.anFn(prompt)
	if err != nil {
		return nil, err
	}
	return &ai.AnalyzeResult{Answer: ans}, nil
}
func (s analyzeStubAdvisor) Chat(_ context.Context, message string) (*ai.AnalyzeResult, error) {
	answer, err := s.chatFn(message)
	if err != nil {
		return nil, err
	}
	return &ai.AnalyzeResult{Answer: answer}, nil
}
func (s analyzeStubAdvisor) BackgroundState() ai.BackgroundState { return s.bg }

// homeDir is the test process working directory used by telegram test.
func homeDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// buildServerWith builds a server with a custom alert store and emitter;
// useful for tests that need to seed alerts or capture events.
func buildServerWith(t *testing.T, cfg *config.Config, store *storage.Store, alerts *anomaly.AlertStore, em *event.Emitter) *Server {
	t.Helper()
	if cfg == nil {
		cfg = defaultCfg()
	}
	if store == nil {
		store = storage.NewStore(60, 10)
	}
	if alerts == nil {
		alerts = anomaly.NewAlertStore(64)
	}
	if em == nil {
		em = event.NewEmitter()
	}
	s, err := New(Options{
		Cfg:        cfg,
		Store:      store,
		Controller: controller.NewController(cfg, store, em),
		Alerts:     alerts,
		Emitter:    em,
		Version:    "v",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}
