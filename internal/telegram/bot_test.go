//go:build windows

package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

type fakeController struct {
	killed    []uint32
	suspended []uint32
	resumed   []uint32
	err       error
}

type fakeAdvisor struct {
	result  *ai.AnalyzeResult
	err     error
	chat    *ai.AnalyzeResult
	chatOK  bool
	enabled bool
}

func (f *fakeAdvisor) Enabled() bool { return f.enabled }

func (f *fakeAdvisor) Analyze(ctx context.Context, userQuestion string) (*ai.AnalyzeResult, error) {
	return f.result, f.err
}

func (f *fakeAdvisor) Chat(ctx context.Context, userMessage string) (*ai.AnalyzeResult, error) {
	if f.chatOK {
		return f.chat, f.err
	}
	return f.result, f.err
}

func (f *fakeController) Kill(pid uint32, confirm bool) error {
	f.killed = append(f.killed, pid)
	return f.err
}

func (f *fakeController) Suspend(pid uint32, confirm bool) error {
	f.suspended = append(f.suspended, pid)
	return f.err
}

func (f *fakeController) Resume(pid uint32) error {
	f.resumed = append(f.resumed, pid)
	return f.err
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResponse(status int, body any) *http.Response {
	buf, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(buf)),
		Header:     make(http.Header),
	}
}

func stringResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newTestBot(t *testing.T, cfg *config.Config) (*Bot, *fakeController) {
	t.Helper()
	store := storage.NewStore(60, 10)
	alerts := anomaly.NewAlertStore(32)
	ctrl := &fakeController{}
	bot := New(cfg, store, alerts, ctrl, nil, nil, nil)
	return bot, ctrl
}

func newSnapshot() *metrics.SystemSnapshot {
	return &metrics.SystemSnapshot{
		Timestamp: time.Now(),
		CPU:       metrics.CPUMetrics{TotalPercent: 12.3, NumLogical: 8},
		Memory:    metrics.MemoryMetrics{UsedPercent: 45, UsedPhys: 1024, TotalPhys: 4096},
		Network:   metrics.NetworkMetrics{TotalDownBPS: 200, TotalUpBPS: 100},
		Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "low.exe", CPUPercent: 1, WorkingSet: 100, ThreadCount: 4},
			{PID: 2, Name: "high.exe", CPUPercent: 90, WorkingSet: 200, ThreadCount: 8},
			{PID: 3, Name: "low2.exe", CPUPercent: 1.5, WorkingSet: 50, ThreadCount: 1},
		},
	}
}

func TestParseCommand(t *testing.T) {
	cmd, args := parseCommand("/topcpu@wtm_bot 123")
	if cmd != "topcpu" {
		t.Fatalf("cmd=%q want topcpu", cmd)
	}
	if len(args) != 1 || args[0] != "123" {
		t.Fatalf("args=%v want [123]", args)
	}
}

func TestParseCommandEmptyAndPlain(t *testing.T) {
	if cmd, args := parseCommand(""); cmd != "" || args != nil {
		t.Fatalf("empty: cmd=%q args=%v", cmd, args)
	}
	if cmd, args := parseCommand("   "); cmd != "" || args != nil {
		t.Fatalf("whitespace: cmd=%q args=%v", cmd, args)
	}
	if cmd, args := parseCommand("/ALERTS"); cmd != "alerts" {
		t.Fatalf("uppercase: cmd=%q args=%v", cmd, args)
	}
	if cmd, args := parseCommand("plain"); cmd != "plain" || len(args) != 0 {
		t.Fatalf("plain: cmd=%q args=%v", cmd, args)
	}
	if cmd, args := parseCommand("/ask hi there"); cmd != "ask" || len(args) != 2 || args[0] != "hi" || args[1] != "there" {
		t.Fatalf("args: cmd=%q args=%v", cmd, args)
	}
}

func TestIsAllowedChat(t *testing.T) {
	if !isAllowedChat([]int64{1, 2, 3}, 2) {
		t.Fatal("expected chat 2 to be allowed")
	}
	if isAllowedChat([]int64{1, 2, 3}, 9) {
		t.Fatal("expected chat 9 to be rejected")
	}
	if isAllowedChat(nil, 1) {
		t.Fatal("nil allowlist should reject")
	}
}

func TestNewRegistersEmitterHandler(t *testing.T) {
	em := event.NewEmitter()
	cfg := config.DefaultConfig()
	bot := New(cfg, storage.NewStore(60, 10), anomaly.NewAlertStore(32), &fakeController{}, nil, nil, em)
	if bot == nil {
		t.Fatal("bot should be non-nil")
	}
	called := 0
	em.On(anomaly.EventAlertRaised, func(data any) {
		called++
	})
	em.Emit("some.event", nil)
	if called != 0 {
		t.Fatal("emitter should not fire on unrelated event type")
	}
}

func TestNewNilEmitterDoesNotPanic(t *testing.T) {
	cfg := config.DefaultConfig()
	bot := New(cfg, storage.NewStore(60, 10), anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	if bot == nil {
		t.Fatal("bot should be non-nil")
	}
	if bot.rootCtx == nil {
		t.Fatal("rootCtx should default to context.Background")
	}
}

func TestSetConfig(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	newCfg := config.DefaultConfig()
	newCfg.Telegram.BotToken = "new-token"
	bot.SetConfig(newCfg)
	if got := bot.currentConfig().Telegram.BotToken; got != "new-token" {
		t.Fatalf("token=%q want new-token", got)
	}
}

func TestStartNilContext(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.Start(nil)
	if bot.rootCtx == nil {
		t.Fatal("rootCtx should not be nil after Start(nil)")
	}
	// Give loop a moment to exit on its own; it's running with the default config (disabled).
	time.Sleep(20 * time.Millisecond)
}

func TestStartWithContext(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	bot.Start(ctx)
	if bot.rootCtx != ctx {
		t.Fatal("rootCtx should match the provided context")
	}
	cancel()
	time.Sleep(20 * time.Millisecond)
}

func TestBackgroundContextFallback(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.rootCtx = nil
	got := bot.backgroundContext()
	if got == nil {
		t.Fatal("backgroundContext must never return nil")
	}
}

func TestSleepContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if !sleepContext(ctx, 5*time.Millisecond) {
		t.Fatal("sleepContext should return true when timer fires")
	}
	cancel()
	if sleepContext(ctx, time.Second) {
		t.Fatal("sleepContext should return false when ctx is cancelled")
	}
}

func TestLoopDisabled(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		bot.loop(ctx)
		close(done)
	}()
	// Sleep just long enough for the 5s timer inside loop to fire and hit the `continue`.
	time.Sleep(5100 * time.Millisecond)
	cancel()
	<-done
}

func TestLoopContextCancelledImmediately(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bot.loop(ctx)
}

func TestLoopTokenChangeResetsOffset(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token1"
	cfg.Telegram.AllowedChatIDs = []int64{1}
	cfg.Telegram.APIBaseURL = "http://example.test"

	calls := 0
	var lastBody []byte
	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		buf, _ := io.ReadAll(r.Body)
		lastBody = append([]byte{}, buf...)
		return jsonResponse(http.StatusOK, updateResp{OK: true, Result: nil}), nil
	})}
	bot.offset = 99
	bot.lastToken = ""

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bot.loop(ctx)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
	if calls == 0 {
		t.Fatal("loop should have made at least one HTTP call")
	}
	if bot.offset != 0 {
		t.Fatalf("offset=%d, expected reset to 0 on token change", bot.offset)
	}
	if !bytes.Contains(lastBody, []byte(`"offset":0`)) {
		t.Fatalf("expected offset=0 in body, got %s", lastBody)
	}
}

func TestLoopGetUpdatesError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.AllowedChatIDs = []int64{1}
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		bot.loop(ctx)
		close(done)
	}()
	// Sleep just long enough for the 3s timer inside loop to fire and hit the `continue`.
	time.Sleep(3100 * time.Millisecond)
	cancel()
	<-done
}

func TestLoopGetUpdatesNonOK(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.AllowedChatIDs = []int64{1}
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, updateResp{OK: false, ErrorCode: 401, Description: "unauthorized"}), nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	go bot.loop(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()
}

func TestLoopSkipsEmptyAndDisallowed(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.AllowedChatIDs = []int64{42}
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, updateResp{
			OK: true,
			Result: []tgUpdate{
				{UpdateID: 1, Message: &tgMessage{Text: "", Chat: tgChat{ID: 42}}},
				{UpdateID: 2, Message: &tgMessage{Text: "  ", Chat: tgChat{ID: 42}}},
				{UpdateID: 3, Message: nil},
				{UpdateID: 4, Message: &tgMessage{Text: "/status", Chat: tgChat{ID: 99}}},
				{UpdateID: 5, Message: &tgMessage{Text: "/start", Chat: tgChat{ID: 42}}},
			},
		}), nil
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		bot.loop(ctx)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done
	if bot.offset < 5 {
		t.Fatalf("offset=%d, expected >= 5 (we processed all updates)", bot.offset)
	}
}

func TestGetUpdatesTimeoutFallback(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.AllowedChatIDs = []int64{1}
	cfg.Telegram.APIBaseURL = "http://example.test"
	cfg.Telegram.PollTimeout = 0

	var captured map[string]any
	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		return jsonResponse(http.StatusOK, updateResp{OK: true, Result: nil}), nil
	})}

	updates, err := bot.getUpdates(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if updates != nil {
		t.Fatalf("updates=%v want nil", updates)
	}
	if v, ok := captured["timeout"]; !ok {
		t.Fatalf("timeout field missing: %v", captured)
	} else if v.(float64) != 25 {
		t.Fatalf("timeout fallback=%v want 25", v)
	}
}

func TestGetUpdatesNonOK(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, updateResp{OK: false, ErrorCode: 400, Description: "bad"}), nil
	})}

	_, err := bot.getUpdates(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err=%v want contains 400", err)
	}
}

func TestAPICallJSONMarshalError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"bad": make(chan int)}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("err=%v", err)
	}
}

func TestAPICallTransportError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("dial fail")
	})}
	if err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"x": 1}, nil); err == nil {
		t.Fatal("expected error from transport")
	}
}

func TestAPICallResponseTooLarge(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	huge := strings.Repeat("x", maxTelegramResponseBytes+10)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return stringResponse(http.StatusOK, huge), nil
	})}
	if err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"x": 1}, nil); err == nil {
		t.Fatal("expected oversize response error")
	}
}

func TestAPICallDecodeError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return stringResponse(http.StatusOK, "not json"), nil
	})}
	if err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"x": 1}, nil); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestAPICallNoBody(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Body != nil {
			buf, _ := io.ReadAll(r.Body)
			if len(buf) != 0 {
				t.Fatalf("expected empty body, got %q", buf)
			}
		}
		return stringResponse(http.StatusOK, "{}"), nil
	})}
	var out map[string]any
	if err := bot.apiCall(context.Background(), cfg, "getMe", nil, &out); err != nil {
		t.Fatal(err)
	}
}

func TestAPICallNewRequestError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	// Control character in URL triggers NewRequestWithContext error.
	cfg.Telegram.APIBaseURL = "http://example.test/\x7f"
	bot, _ := newTestBot(t, cfg)
	if err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"x": 1}, nil); err == nil {
		t.Fatal("expected NewRequest error")
	}
}

func TestAPICallReadBodyError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"
	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(brokenReader{}),
			Header:     make(http.Header),
		}, nil
	})}
	if err := bot.apiCall(context.Background(), cfg, "sendMessage", map[string]any{"x": 1}, nil); err == nil {
		t.Fatal("expected ReadAll error")
	}
}

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { return 0, errors.New("broken pipe") }

func TestHandleMessageAllCommands(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"
	cfg.Telegram.RequireConfirm = false

	bot, ctrl := newTestBot(t, cfg)
	bot.store.SetLatest(newSnapshot())
	bot.alerts.Raise(anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "Runaway", ProcessName: "high.exe", PID: 2})

	var sentTexts []string
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(body, &req)
			sentTexts = append(sentTexts, req.Text)
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
	})}

	ctx := context.Background()

	cases := []struct {
		text    string
		contain string
	}{
		{"/start", "WTM rescue bot"},
		{"/help", "WTM rescue bot"},
		{"/status", "CPU 12.3%"},
		{"/topcpu", "Top CPU processes"},
		{"/alerts", "Active alerts"},
		{"/ask", "Prompt required"},
		{"/kill 1", "Killed low.exe (PID 1)"},
		{"/suspend 1", "Suspended low.exe (PID 1)"},
		{"/resume 1", "Resumed low.exe (PID 1)"},
		{"/unknown", "Unknown command"},
		{"/confirm", "Confirmation code required"},
		{"/cancel", "Confirmation code required"},
	}
	for _, tc := range cases {
		sentTexts = sentTexts[:0]
		bot.handleMessage(ctx, cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: tc.text})
		if len(sentTexts) != 1 {
			t.Fatalf("text=%q: %d sendMessage calls, want 1", tc.text, len(sentTexts))
		}
		if !strings.Contains(sentTexts[0], tc.contain) {
			t.Fatalf("text=%q reply=%q missing %q", tc.text, sentTexts[0], tc.contain)
		}
	}
	if len(ctrl.killed) == 0 {
		t.Fatal("kill should have been called")
	}
	if len(ctrl.suspended) == 0 {
		t.Fatal("suspend should have been called")
	}
	if len(ctrl.resumed) == 0 {
		t.Fatal("resume should have been called")
	}
}

func TestHandleMessageTaskkillAndAI(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, ctrl := newTestBot(t, cfg)
	bot.store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 333, Name: "do.exe", CPUPercent: 80}},
	})

	bot.advisor = &fakeAdvisor{
		enabled: true,
		result:  &ai.AnalyzeResult{Answer: "Use caution."},
	}

	var sentTexts []string
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(body, &req)
			sentTexts = append(sentTexts, req.Text)
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
	})}

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/taskkill do"})
	if !strings.Contains(sentTexts[0], "Pending kill") {
		t.Fatalf("/taskkill reply=%q", sentTexts[0])
	}
	// Confirm to actually run the closure
	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/confirm " + extractCode(sentTexts[0])})

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/ask why is cpu high?"})
	if !strings.Contains(sentTexts[2], "Use caution.") {
		t.Fatalf("/ask reply=%q", sentTexts[2])
	}

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/analyze review state"})
	if !strings.Contains(sentTexts[3], "Use caution.") {
		t.Fatalf("/analyze reply=%q", sentTexts[3])
	}

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/killtop"})
	if !strings.Contains(sentTexts[4], "Pending kill") {
		t.Fatalf("/killtop reply=%q", sentTexts[4])
	}
	// Confirm to run the killtop closure
	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/confirm " + extractCode(sentTexts[4])})

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/suspendtop"})
	if !strings.Contains(sentTexts[6], "Pending suspend") {
		t.Fatalf("/suspendtop reply=%q", sentTexts[6])
	}
	// Confirm to run the suspendtop closure
	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/confirm " + extractCode(sentTexts[6])})

	if len(ctrl.killed) == 0 {
		t.Fatal("expected at least one kill to have run")
	}
	if len(ctrl.suspended) == 0 {
		t.Fatal("expected at least one suspend to have run")
	}
}

func TestHandleMessageNoSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	var sentTexts []string
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(body, &req)
			sentTexts = append(sentTexts, req.Text)
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
	})}

	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/status"})
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "No snapshot yet") {
		t.Fatalf("expected 'No snapshot yet', got %v", sentTexts)
	}
}

func TestHandleMessageEmptyText(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	calls := 0
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
	})}
	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: ""})
	if calls != 0 {
		t.Fatalf("expected 0 HTTP calls for empty text, got %d", calls)
	}
}

func TestHandleMessageSendError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"

	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"ok": false, "error_code": 400, "description": "bad chat"}), nil
	})}
	bot.handleMessage(context.Background(), cfg, &tgMessage{Chat: tgChat{ID: 1}, Text: "/start"})
}

func TestHelpText(t *testing.T) {
	s := helpText()
	if !strings.Contains(s, "WTM rescue bot commands") {
		t.Fatalf("help text missing title: %q", s)
	}
}

func TestStatusTextNoSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.statusText(); got != "No snapshot yet." {
		t.Fatalf("statusText=%q", got)
	}
}

func TestStatusTextWithSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.store.SetLatest(newSnapshot())
	got := bot.statusText()
	if !strings.Contains(got, "CPU 12.3% (8 cores)") {
		t.Fatalf("statusText=%q", got)
	}
	if !strings.Contains(got, "Memory 45.0%") {
		t.Fatalf("statusText=%q", got)
	}
	if !strings.Contains(got, "Top CPU:") {
		t.Fatalf("statusText=%q", got)
	}
}

func TestTopCPUTextNoSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.topCPUText(); got != "No snapshot yet." {
		t.Fatalf("topCPUText=%q", got)
	}
}

func TestTopCPUTextNoProcesses(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now()})
	if got := bot.topCPUText(); got != "No processes found." {
		t.Fatalf("topCPUText=%q", got)
	}
}

func TestTopCPUTextWithProcesses(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.store.SetLatest(newSnapshot())
	got := bot.topCPUText()
	if !strings.Contains(got, "Top CPU processes:") {
		t.Fatalf("topCPUText=%q", got)
	}
	if !strings.Contains(got, "high.exe") {
		t.Fatalf("topCPUText=%q", got)
	}
}

func TestAlertsTextEmpty(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.alertsText(); got != "No active alerts." {
		t.Fatalf("alertsText=%q", got)
	}
}

func TestAlertsTextWithVariants(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	bot.alerts.Raise(anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "Run", ProcessName: "high.exe"})
	bot.alerts.Raise(anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical, Title: "Mem", PID: 99})
	bot.alerts.Raise(anomaly.Alert{Type: "port_conflict", Severity: anomaly.SeverityCritical, Title: "Port"})

	got := bot.alertsText()
	if !strings.Contains(got, "high.exe") {
		t.Fatalf("alertsText missing process name: %q", got)
	}
	if !strings.Contains(got, "PID 99") {
		t.Fatalf("alertsText missing PID: %q", got)
	}
	if !strings.Contains(got, "Port") {
		t.Fatalf("alertsText missing untargeted alert: %q", got)
	}
}

func TestAlertsTextCappedAt8(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	for i := 0; i < 12; i++ {
		bot.alerts.Raise(anomaly.Alert{Type: fmt.Sprintf("t%d", i), Severity: anomaly.SeverityCritical, Title: fmt.Sprintf("A%d", i), PID: uint32(i + 1)})
	}
	got := bot.alertsText()
	count := strings.Count(got, "- [")
	if count > 8 {
		t.Fatalf("alertsText should cap at 8 entries, got %d in %q", count, got)
	}
}

func TestPIDActionRequiresConfirm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	cfg.Telegram.ConfirmTTL = 45 * time.Second

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 4242, Name: "node.exe"}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.pidAction(cfg, 99, []string{"4242"}, func(pid uint32) error {
		return ctrl.Kill(pid, true)
	}, "kill", "killed", true)

	if !strings.Contains(reply, "/confirm ") {
		t.Fatalf("reply=%q missing confirm hint", reply)
	}
	if len(ctrl.killed) != 0 {
		t.Fatalf("kill ran before confirm: %v", ctrl.killed)
	}

	code := extractCode(reply)
	done := bot.confirmAction([]string{code}, 99)
	if !strings.Contains(done, "Killed node.exe (PID 4242)") {
		t.Fatalf("confirm reply=%q", done)
	}
	if len(ctrl.killed) != 1 || ctrl.killed[0] != 4242 {
		t.Fatalf("kill calls=%v want [4242]", ctrl.killed)
	}

	again := bot.confirmAction([]string{code}, 99)
	if again != "Confirmation code not found or expired." {
		t.Fatalf("unexpected second confirm reply=%q", again)
	}
}

func TestPIDActionNoConfirm(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = false

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 5000, Name: "x.exe"}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.pidAction(cfg, 1, []string{"5000"}, func(pid uint32) error {
		return ctrl.Kill(pid, true)
	}, "kill", "killed", false)
	if !strings.Contains(reply, "Killed x.exe (PID 5000)") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestPIDActionNoArgs(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.pidAction(config.DefaultConfig(), 1, nil, func(uint32) error { return nil }, "kill", "killed", true); got != "PID required." {
		t.Fatalf("got=%q", got)
	}
}

func TestPIDActionInvalidPID(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.pidAction(config.DefaultConfig(), 1, []string{"abc"}, func(uint32) error { return nil }, "kill", "killed", true); got != "PID must be a positive integer." {
		t.Fatalf("got=%q", got)
	}
}

func TestPIDActionOverflow(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.pidAction(config.DefaultConfig(), 1, []string{"9999999999"}, func(uint32) error { return nil }, "kill", "killed", true); got != "PID must be a positive integer." {
		t.Fatalf("got=%q", got)
	}
}

func TestPIDActionFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = false

	ctrl := &fakeController{err: errors.New("nope")}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 8888, Name: "n.exe"}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.pidAction(cfg, 1, []string{"8888"}, func(pid uint32) error {
		return ctrl.Kill(pid, true)
	}, "kill", "killed", true)
	if !strings.Contains(reply, "Action failed: nope") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestPIDActionNoSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	bot, _ := newTestBot(t, cfg)
	reply := bot.pidAction(cfg, 1, []string{"1234"}, func(uint32) error { return nil }, "kill", "killed", true)
	if !strings.Contains(reply, "Pending kill PID 1234") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestCancelAction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	cfg.Telegram.ConfirmTTL = 30 * time.Second

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 9001, Name: "chrome.exe"}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.pidAction(cfg, 77, []string{"9001"}, func(pid uint32) error {
		return ctrl.Suspend(pid, true)
	}, "suspend", "suspended", true)
	code := extractCode(reply)

	cancelled := bot.cancelAction([]string{code}, 77)
	if !strings.Contains(cancelled, "Cancelled suspend chrome.exe (PID 9001)") {
		t.Fatalf("cancel reply=%q", cancelled)
	}
	if len(ctrl.suspended) != 0 {
		t.Fatalf("suspend ran after cancel: %v", ctrl.suspended)
	}
}

func TestShouldNotifyTelegramAlertHighValueOnly(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "secret"
	cfg.Telegram.NotifyOnCritical = true
	cfg.Telegram.NotificationMode = "high_value"
	cfg.Telegram.NotificationTypes = []string{"runaway_cpu", "rule:*"}

	if shouldNotifyTelegramAlert(cfg, anomaly.Alert{Type: "hung_process", Severity: anomaly.SeverityCritical}) {
		t.Fatal("hung_process should be suppressed in high_value mode")
	}
	if !shouldNotifyTelegramAlert(cfg, anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical}) {
		t.Fatal("runaway_cpu should pass in high_value mode")
	}
	if shouldNotifyTelegramAlert(cfg, anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical}) {
		t.Fatal("memory_leak should be suppressed when removed from allowlist")
	}
	if !shouldNotifyTelegramAlert(cfg, anomaly.Alert{Type: "rule:KillChrome", Severity: anomaly.SeverityCritical, Action: "kill"}) {
		t.Fatal("rule:* should allow critical rule actions")
	}
}

func TestShouldNotifyTelegramAlertAllBranches(t *testing.T) {
	baseCfg := func() *config.Config {
		c := config.DefaultConfig()
		c.Telegram.Enabled = true
		c.Telegram.BotToken = "secret"
		c.Telegram.NotifyOnCritical = true
		return c
	}

	// nil config
	if shouldNotifyTelegramAlert(nil, anomaly.Alert{Severity: anomaly.SeverityCritical}) {
		t.Fatal("nil cfg should not notify")
	}
	// disabled
	c := baseCfg()
	c.Telegram.Enabled = false
	if shouldNotifyTelegramAlert(c, anomaly.Alert{Severity: anomaly.SeverityCritical}) {
		t.Fatal("disabled cfg should not notify")
	}
	// NotifyOnCritical off
	c = baseCfg()
	c.Telegram.NotifyOnCritical = false
	if shouldNotifyTelegramAlert(c, anomaly.Alert{Severity: anomaly.SeverityCritical}) {
		t.Fatal("NotifyOnCritical off should not notify")
	}
	// missing token
	c = baseCfg()
	c.Telegram.BotToken = ""
	if shouldNotifyTelegramAlert(c, anomaly.Alert{Severity: anomaly.SeverityCritical}) {
		t.Fatal("missing token should not notify")
	}
	// non-critical severity
	c = baseCfg()
	if shouldNotifyTelegramAlert(c, anomaly.Alert{Severity: anomaly.SeverityWarning}) {
		t.Fatal("warning should not notify")
	}
	// all_critical mode
	c = baseCfg()
	c.Telegram.NotificationMode = "all_critical"
	if !shouldNotifyTelegramAlert(c, anomaly.Alert{Type: "hung_process", Severity: anomaly.SeverityCritical}) {
		t.Fatal("all_critical should allow all critical")
	}
	// default empty mode
	c = baseCfg()
	c.Telegram.NotificationMode = ""
	if !shouldNotifyTelegramAlert(c, anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical}) {
		t.Fatal("default mode should fall back to high_value")
	}
	// unknown mode
	c = baseCfg()
	c.Telegram.NotificationMode = "mystery"
	c.Telegram.NotificationTypes = []string{"runaway_cpu"}
	if shouldNotifyTelegramAlert(c, anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical}) {
		t.Fatal("unknown mode should still apply high_value filter")
	}
}

func TestIsHighValueTelegramAlertBranches(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.NotificationTypes = []string{"*"}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "new_process", Severity: anomaly.SeverityCritical}) {
		t.Fatal("wildcard allowlist should match new_process")
	}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "network_anomaly_system", Severity: anomaly.SeverityCritical}) {
		t.Fatal("wildcard allowlist should match network_anomaly_system")
	}
	cfg.Telegram.NotificationTypes = []string{"memory_*"}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical}) {
		t.Fatal("prefix wildcard should match")
	}
	cfg.Telegram.NotificationTypes = nil
	if isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical}) {
		t.Fatal("empty allowlist should reject")
	}
	cfg.Telegram.NotificationTypes = []string{"unknown"}
	if isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "rule:foo", Severity: anomaly.SeverityCritical, Action: "alert"}) {
		t.Fatal("rule with non-kill/suspend action and no rule:* allowlist should reject")
	}
	cfg.Telegram.NotificationTypes = []string{"rule:*"}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "rule:foo", Severity: anomaly.SeverityCritical, Action: "alert"}) {
		t.Fatal("rule:* allowlist should match any rule action")
	}
	cfg.Telegram.NotificationTypes = []string{"rule:*"}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "rule:foo", Severity: anomaly.SeverityCritical, Action: "kill"}) {
		t.Fatal("rule: kill action should match via action check")
	}
	cfg.Telegram.NotificationTypes = []string{"port_*"}
	if !isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "port_conflict", Severity: anomaly.SeverityCritical}) {
		t.Fatal("port_* prefix should match port_conflict")
	}
	// unknown type
	cfg.Telegram.NotificationTypes = []string{"*"}
	if isHighValueTelegramAlert(cfg, anomaly.Alert{Type: "weird_type", Severity: anomaly.SeverityCritical}) {
		t.Fatal("unknown type with wildcard should still reject (only listed types are in the switch)")
	}
}

func TestMatchesTelegramTypeAllowlistBranches(t *testing.T) {
	if !matchesTelegramTypeAllowlist([]string{"a", "b", "c"}, "b") {
		t.Fatal("exact match should be true")
	}
	if matchesTelegramTypeAllowlist([]string{"a", "b"}, "z") {
		t.Fatal("non-match should be false")
	}
	if !matchesTelegramTypeAllowlist([]string{"*"}, "anything") {
		t.Fatal("wildcard should match")
	}
	if !matchesTelegramTypeAllowlist([]string{"mem*"}, "memory_leak") {
		t.Fatal("prefix wildcard should match")
	}
	if matchesTelegramTypeAllowlist([]string{"mem"}, "memory_leak") {
		t.Fatal("non-prefix token should not match")
	}
	// empty / whitespace tokens skipped
	if !matchesTelegramTypeAllowlist([]string{"", "  ", "real"}, "real") {
		t.Fatal("empty tokens should be skipped, real should still match")
	}
	// case insensitive
	if !matchesTelegramTypeAllowlist([]string{"MEMORY_LEAK"}, "memory_leak") {
		t.Fatal("matching should be case-insensitive")
	}
}

func TestAIChatQueuesConfirmableAction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	cfg.Telegram.ConfirmTTL = 45 * time.Second

	executed := []string{}
	bot := New(
		cfg,
		storage.NewStore(60, 10),
		anomaly.NewAlertStore(32),
		&fakeController{},
		&fakeAdvisor{
			enabled: true,
			result: &ai.AnalyzeResult{
				Answer: "Protect claude.exe and add a rule.",
				Actions: []ai.Suggestion{
					{Type: "protect", Name: "claude.exe"},
				},
			},
		},
		func(suggestion ai.Suggestion) error {
			executed = append(executed, suggestion.Type+":"+suggestion.Name)
			return nil
		},
		nil,
	)

	reply := bot.aiChatText(context.Background(), cfg, 55, "make this safer")
	if !strings.Contains(reply, "/confirm ") {
		t.Fatalf("reply=%q missing confirm hint", reply)
	}
	code := extractCode(reply)
	done := bot.confirmAction([]string{code}, 55)
	if !strings.Contains(done, "AI action executed") {
		t.Fatalf("confirm reply=%q", done)
	}
	if len(executed) != 1 || executed[0] != "protect:claude.exe" {
		t.Fatalf("executed=%v", executed)
	}
}

func TestPIDActionFailsClosedWhenConfirmCodeGenerationFails(t *testing.T) {
	old := newConfirmCodeFunc
	newConfirmCodeFunc = func() (string, error) {
		return "", errors.New("entropy unavailable")
	}
	defer func() { newConfirmCodeFunc = old }()

	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 5150, Name: "tool.exe"}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.pidAction(cfg, 12, []string{"5150"}, func(pid uint32) error {
		return ctrl.Kill(pid, true)
	}, "kill", "killed", true)

	if reply != "Failed to create confirmation code; action was not executed." {
		t.Fatalf("reply=%q", reply)
	}
	if len(ctrl.killed) != 0 {
		t.Fatalf("kill should not run on confirmation setup failure: %v", ctrl.killed)
	}
}

func extractCode(reply string) string {
	for _, field := range strings.Fields(reply) {
		if len(field) == 8 && !strings.Contains(field, "/") && strings.ToUpper(field) == field {
			return strings.Trim(field, ".,")
		}
	}
	return ""
}

func TestParseTaskkillArgs(t *testing.T) {
	name, force := parseTaskkillArgs(nil)
	if name != "" || force {
		t.Fatalf("nil args: name=%q force=%v", name, force)
	}
	name, force = parseTaskkillArgs([]string{"/F", "notepad"})
	if name != "notepad" || !force {
		t.Fatalf("/F notepad: name=%q force=%v", name, force)
	}
	name, force = parseTaskkillArgs([]string{"-F", "notepad"})
	if !force {
		t.Fatalf("-F should set force")
	}
	name, force = parseTaskkillArgs([]string{"--FORCE", "notepad"})
	if !force {
		t.Fatalf("--FORCE should set force")
	}
	name, force = parseTaskkillArgs([]string{"/F"})
	if name != "" || !force {
		t.Fatalf("/F alone: name=%q force=%v", name, force)
	}
	name, force = parseTaskkillArgs([]string{"/IM", "notepad"})
	if name != "notepad" {
		t.Fatalf("/IM with name: name=%q", name)
	}
	name, force = parseTaskkillArgs([]string{"chrome"})
	if name != "chrome" || force {
		t.Fatalf("plain: name=%q force=%v", name, force)
	}
	// last non-flag argument is the name
	name, force = parseTaskkillArgs([]string{"/F", "/IM", "chrome"})
	if name != "chrome" || !force {
		t.Fatalf("/F /IM chrome: name=%q force=%v", name, force)
	}
}

func TestNameAction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	cfg.Telegram.ConfirmTTL = 30 * time.Second

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 100, Name: "alpha.exe", CPUPercent: 30},
			{PID: 101, Name: "alpha.exe", CPUPercent: 80},
		},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)

	// no name
	reply := bot.nameAction(cfg, 1, nil, func(uint32) error { return nil }, "kill", "killed", true)
	if reply != "Process name required. Usage: /taskkill [/F] [/IM] <name>" {
		t.Fatalf("no name reply=%q", reply)
	}

	// no match
	reply = bot.nameAction(cfg, 1, []string{"missing"}, func(uint32) error { return nil }, "kill", "killed", true)
	if !strings.Contains(reply, "No process found") {
		t.Fatalf("no match reply=%q", reply)
	}

	// match — confirm queued
	reply = bot.nameAction(cfg, 1, []string{"alpha"}, func(pid uint32) error { return ctrl.Kill(pid, true) }, "kill", "killed", true)
	if !strings.Contains(reply, "Pending kill alpha.exe (PID 101)") {
		t.Fatalf("confirm reply=%q", reply)
	}

	// match — name with .exe suffix
	ctrl2b := &fakeController{}
	bot2b := New(cfg, store, anomaly.NewAlertStore(32), ctrl2b, nil, nil, nil)
	reply = bot2b.nameAction(cfg, 1, []string{"alpha.exe"}, func(pid uint32) error { return ctrl2b.Kill(pid, true) }, "kill", "killed", false)
	if !strings.Contains(reply, "Killed alpha.exe (PID 101)") {
		t.Fatalf(".exe reply=%q", reply)
	}

	// match — force, no confirm
	ctrl2 := &fakeController{}
	bot2 := New(cfg, store, anomaly.NewAlertStore(32), ctrl2, nil, nil, nil)
	reply = bot2.nameAction(cfg, 1, []string{"/F", "alpha"}, func(pid uint32) error { return ctrl2.Kill(pid, true) }, "kill", "killed", true)
	if !strings.Contains(reply, "Killed alpha.exe (PID 101)") {
		t.Fatalf("force reply=%q", reply)
	}
	if len(ctrl2.killed) != 1 || ctrl2.killed[0] != 101 {
		t.Fatalf("force kill calls=%v", ctrl2.killed)
	}

	// match — controller error
	ctrl3 := &fakeController{err: errors.New("denied")}
	bot3 := New(cfg, store, anomaly.NewAlertStore(32), ctrl3, nil, nil, nil)
	reply = bot3.nameAction(cfg, 1, []string{"/F", "alpha"}, func(pid uint32) error { return ctrl3.Kill(pid, true) }, "kill", "killed", true)
	if !strings.Contains(reply, "Action failed: denied") {
		t.Fatalf("error reply=%q", reply)
	}
}

func TestNameActionNoSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	reply := bot.nameAction(cfg, 1, []string{"any"}, func(uint32) error { return nil }, "kill", "killed", true)
	if reply != "No snapshot yet." {
		t.Fatalf("reply=%q", reply)
	}
}

func TestNameActionProtected(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	cfg.Controller.ProtectedProcesses = []string{"alpha.exe"}

	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 200, Name: "alpha.exe", CPUPercent: 90}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	reply := bot.nameAction(cfg, 1, []string{"alpha"}, func(uint32) error { return nil }, "kill", "killed", true)
	if !strings.Contains(reply, "is protected or critical") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestNameActionCriticalPID(t *testing.T) {
	cfg := config.DefaultConfig()
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 201, Name: "crash.exe", CPUPercent: 90, IsCritical: true}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	reply := bot.nameAction(cfg, 1, []string{"crash"}, func(uint32) error { return nil }, "kill", "killed", true)
	if !strings.Contains(reply, "is protected or critical") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestNameActionPID0(t *testing.T) {
	cfg := config.DefaultConfig()
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 0, Name: "idle.exe", CPUPercent: 99}},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	reply := bot.nameAction(cfg, 1, []string{"idle"}, func(uint32) error { return nil }, "kill", "killed", true)
	if !strings.Contains(reply, "is protected or critical") {
		t.Fatalf("reply=%q", reply)
	}
}

func TestFindProcessByNameNoSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	pid, fail := bot.findProcessByName(config.DefaultConfig(), "x")
	if pid != 0 || fail != "No snapshot yet." {
		t.Fatalf("pid=%d fail=%q", pid, fail)
	}
}

func TestTopProcessAction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true

	ctrl := &fakeController{}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "alpha.exe", CPUPercent: 20},
			{PID: 2, Name: "beta.exe", CPUPercent: 90},
		},
	})

	bot := New(cfg, store, anomaly.NewAlertStore(32), ctrl, nil, nil, nil)
	reply := bot.topProcessAction(cfg, 1, func(pid uint32) error { return ctrl.Kill(pid, true) }, "kill", "killed", true)
	if !strings.Contains(reply, "Pending kill beta.exe (PID 2)") {
		t.Fatalf("confirm reply=%q", reply)
	}

	// no confirm
	ctrl2 := &fakeController{}
	bot2 := New(cfg, store, anomaly.NewAlertStore(32), ctrl2, nil, nil, nil)
	reply = bot2.topProcessAction(cfg, 1, func(pid uint32) error { return ctrl2.Kill(pid, true) }, "kill", "killed", false)
	if !strings.Contains(reply, "Killed beta.exe (PID 2)") {
		t.Fatalf("no-confirm reply=%q", reply)
	}

	// controller error
	ctrl3 := &fakeController{err: errors.New("oops")}
	bot3 := New(cfg, store, anomaly.NewAlertStore(32), ctrl3, nil, nil, nil)
	reply = bot3.topProcessAction(cfg, 1, func(pid uint32) error { return ctrl3.Kill(pid, true) }, "kill", "killed", false)
	if !strings.Contains(reply, "Action failed: oops") {
		t.Fatalf("error reply=%q", reply)
	}
}

func TestTopProcessActionNoSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	reply := bot.topProcessAction(config.DefaultConfig(), 1, func(uint32) error { return nil }, "kill", "killed", true)
	if reply != "No snapshot yet." {
		t.Fatalf("reply=%q", reply)
	}
}

func TestTopProcessActionNoCandidate(t *testing.T) {
	cfg := config.DefaultConfig()
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 0, Name: "idle", CPUPercent: 99}},
	})
	bot := New(cfg, store, anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	reply := bot.topProcessAction(cfg, 1, func(uint32) error { return nil }, "kill", "killed", true)
	if reply != "No safe top process found." {
		t.Fatalf("reply=%q", reply)
	}
}

func TestConfirmActionNoCode(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.confirmAction(nil, 1); got != "Confirmation code required." {
		t.Fatalf("got=%q", got)
	}
	if got := bot.confirmAction([]string{""}, 1); got != "Confirmation code required." {
		t.Fatalf("got=%q", got)
	}
}

func TestConfirmActionCodeMissing(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.confirmAction([]string{"AAAAAA"}, 1); got != "Confirmation code not found or expired." {
		t.Fatalf("got=%q", got)
	}
}

func TestConfirmActionWrongChat(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	bot, _ := newTestBot(t, cfg)
	bot.store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}}})
	reply := bot.pidAction(cfg, 1, []string{"1"}, func(uint32) error { return nil }, "kill", "killed", true)
	code := extractCode(reply)
	if got := bot.confirmAction([]string{code}, 99); got != "Confirmation code belongs to another chat." {
		t.Fatalf("got=%q", got)
	}
}

func TestConfirmActionRunError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	bot, _ := newTestBot(t, cfg)
	bot.store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}}})
	reply := bot.pidAction(cfg, 1, []string{"1"}, func(uint32) error { return errors.New("boom") }, "kill", "killed", true)
	code := extractCode(reply)
	if got := bot.confirmAction([]string{code}, 1); !strings.Contains(got, "Action failed: boom") {
		t.Fatalf("got=%q", got)
	}
}

func TestConfirmActionCaseInsensitive(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	bot, _ := newTestBot(t, cfg)
	bot.store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}}})
	reply := bot.pidAction(cfg, 1, []string{"1"}, func(uint32) error { return nil }, "kill", "killed", true)
	code := extractCode(reply)
	lower := strings.ToLower(code)
	if got := bot.confirmAction([]string{lower}, 1); !strings.Contains(got, "Killed x (PID 1)") {
		t.Fatalf("got=%q code=%q", got, code)
	}
}

func TestCancelActionNoCode(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.cancelAction(nil, 1); got != "Confirmation code required." {
		t.Fatalf("got=%q", got)
	}
}

func TestCancelActionCodeMissing(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	if got := bot.cancelAction([]string{"AAAAAA"}, 1); got != "Confirmation code not found or expired." {
		t.Fatalf("got=%q", got)
	}
}

func TestCancelActionWrongChat(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	bot, _ := newTestBot(t, cfg)
	bot.store.SetLatest(&metrics.SystemSnapshot{Timestamp: time.Now(), Processes: []metrics.ProcessInfo{{PID: 1, Name: "x"}}})
	reply := bot.pidAction(cfg, 1, []string{"1"}, func(uint32) error { return nil }, "kill", "killed", true)
	code := extractCode(reply)
	if got := bot.cancelAction([]string{code}, 99); got != "Confirmation code belongs to another chat." {
		t.Fatalf("got=%q", got)
	}
}

func TestStorePendingActionDefaultTTL(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	called := false
	code, err := bot.storePendingAction(1, 0, "desc", "ok", func() error { called = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if code == "" {
		t.Fatal("code should not be empty")
	}
	// The pending action should be retrievable
	if bot.pending[code].Description != "desc" {
		t.Fatalf("desc=%q", bot.pending[code].Description)
	}
	if bot.pending[code].Run == nil {
		t.Fatal("Run function should be set")
	}
	// Invoke Run to confirm it's the same closure
	_ = bot.pending[code].Run()
	if !called {
		t.Fatal("Run function should set called=true")
	}
}

func TestStorePendingActionError(t *testing.T) {
	old := newConfirmCodeFunc
	newConfirmCodeFunc = func() (string, error) { return "", errors.New("nope") }
	defer func() { newConfirmCodeFunc = old }()
	bot, _ := newTestBot(t, config.DefaultConfig())
	if _, err := bot.storePendingAction(1, time.Second, "d", "s", func() error { return nil }); err == nil {
		t.Fatal("expected error")
	}
}

func TestCleanupPendingLocked(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	now := time.Now()
	bot.pending["OLD"] = pendingAction{ExpiresAt: now.Add(-time.Second)}
	bot.pending["NEW"] = pendingAction{ExpiresAt: now.Add(time.Second)}
	bot.cleanupPendingLocked(now)
	if _, ok := bot.pending["OLD"]; ok {
		t.Fatal("OLD should be cleaned up")
	}
	if _, ok := bot.pending["NEW"]; !ok {
		t.Fatal("NEW should remain")
	}
}

func TestPIDActionTexts(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	desc, success := bot.pidActionTexts(42, "kill", "killed")
	if desc != "kill PID 42" || success != "Killed PID 42" {
		t.Fatalf("no snapshot: desc=%q success=%q", desc, success)
	}
	bot.store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{{PID: 42, Name: "hello.exe"}},
	})
	desc, success = bot.pidActionTexts(42, "kill", "killed")
	if desc != "kill hello.exe (PID 42)" || success != "Killed hello.exe (PID 42)" {
		t.Fatalf("with snapshot: desc=%q success=%q", desc, success)
	}
}

func TestSelectTopProcessNoSnapshot(t *testing.T) {
	bot, _ := newTestBot(t, config.DefaultConfig())
	candidate, fail := bot.selectTopProcess(config.DefaultConfig())
	if candidate != nil || fail != "No snapshot yet." {
		t.Fatalf("candidate=%v fail=%q", candidate, fail)
	}
}

func TestSelectTopProcessSkipsProtected(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = []string{"top.exe"}
	store := storage.NewStore(60, 10)
	store.SetLatest(&metrics.SystemSnapshot{
		Timestamp: time.Now(),
		Processes: []metrics.ProcessInfo{
			{PID: 1, Name: "top.exe", CPUPercent: 99},
			{PID: 2, Name: "other.exe", CPUPercent: 50},
		},
	})
	bot := New(cfg, store, anomaly.NewAlertStore(32), &fakeController{}, nil, nil, nil)
	candidate, fail := bot.selectTopProcess(cfg)
	if fail != "" {
		t.Fatalf("fail=%q", fail)
	}
	if candidate == nil || candidate.PID != 2 {
		t.Fatalf("candidate=%+v", candidate)
	}
}

func TestTopProcessesByCPU(t *testing.T) {
	in := []metrics.ProcessInfo{
		{PID: 1, CPUPercent: 10},
		{PID: 2, CPUPercent: 50},
		{PID: 3, CPUPercent: 30},
	}
	out := topProcessesByCPU(in, 2)
	if len(out) != 2 || out[0].PID != 2 || out[1].PID != 3 {
		t.Fatalf("out=%+v", out)
	}
	// limit >= len
	out = topProcessesByCPU(in, 10)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	// limit 0
	out = topProcessesByCPU(in, 0)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	// tie-break by PID
	in2 := []metrics.ProcessInfo{
		{PID: 5, CPUPercent: 50},
		{PID: 3, CPUPercent: 50},
	}
	out = topProcessesByCPU(in2, 2)
	if out[0].PID != 3 {
		t.Fatalf("tie-break: out[0].PID=%d", out[0].PID)
	}
}

func TestSendMessageNonOK(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"
	bot, _ := newTestBot(t, cfg)
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, map[string]any{"ok": false, "error_code": 403, "description": "forbidden"}), nil
	})}
	if err := bot.sendMessage(context.Background(), cfg, 1, "hi"); err == nil {
		t.Fatal("expected error for non-OK response")
	}
}

func TestHandleAlertRaised(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "token"
	cfg.Telegram.APIBaseURL = "http://example.test"
	cfg.Telegram.NotifyOnCritical = true
	cfg.Telegram.AllowedChatIDs = []int64{1, 2}
	cfg.Telegram.NotificationTypes = []string{"runaway_cpu"}

	bot, _ := newTestBot(t, cfg)
	sent := 0
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			sent++
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true}), nil
	})}

	// Wrong type — silently ignored.
	bot.handleAlertRaised("not an alert")
	if sent != 0 {
		t.Fatalf("expected 0 sends, got %d", sent)
	}

	// Suppressed by shouldNotify.
	bot.handleAlertRaised(anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical})
	if sent != 0 {
		t.Fatalf("expected 0 sends, got %d", sent)
	}

	// Notified.
	bot.handleAlertRaised(anomaly.Alert{
		Type: "runaway_cpu", Severity: anomaly.SeverityCritical,
		Title: "Runaway", Description: "Long running", ProcessName: "high.exe", PID: 99,
	})
	if sent != 2 {
		t.Fatalf("expected 2 sends (2 chats), got %d", sent)
	}

	// send error path
	bot.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("net down")
	})}
	bot.handleAlertRaised(anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "x"})
}

func TestAIReplyEmptyPrompt(t *testing.T) {
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	if got := bot.aiChatText(context.Background(), cfg, 1, "   "); !strings.Contains(got, "Prompt required") {
		t.Fatalf("chat got=%q", got)
	}
	if got := bot.aiAnalyzeText(context.Background(), cfg, 1, "   "); !strings.Contains(got, "Prompt required") {
		t.Fatalf("analyze got=%q", got)
	}
}

func TestAIReplyAdvisorNilOrDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	if got := bot.aiChatText(context.Background(), cfg, 1, "why?"); got != "AI advisor not configured." {
		t.Fatalf("nil advisor got=%q", got)
	}
	bot.advisor = &fakeAdvisor{enabled: false}
	if got := bot.aiChatText(context.Background(), cfg, 1, "why?"); got != "AI advisor not configured." {
		t.Fatalf("disabled advisor got=%q", got)
	}
}

func TestAIReplyAdvisorError(t *testing.T) {
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	bot.advisor = &fakeAdvisor{enabled: true, err: errors.New("model down")}
	if got := bot.aiChatText(context.Background(), cfg, 1, "why?"); !strings.Contains(got, "AI error: model down") {
		t.Fatalf("got=%q", got)
	}
}

func TestAIReplySuggestionsAndCap(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Telegram.RequireConfirm = true
	executed := []string{}
	bot, _ := newTestBot(t, cfg)
	bot.advisor = &fakeAdvisor{
		enabled: true,
		result: &ai.AnalyzeResult{
			Answer: "answer",
			Actions: []ai.Suggestion{
				{Type: "kill", Name: "a.exe", PID: 1},
				{Type: "suspend", Name: "b.exe", PID: 2},
				{Type: "protect", Name: "c.exe"},
				{Type: "ignore", Name: "d.exe"},
				{Type: "add_rule", Rule: &ai.RuleSuggestion{Name: "r1"}},
				{Type: "weird"},
			},
		},
	}
	bot.executeAI = func(s ai.Suggestion) error {
		executed = append(executed, s.Type)
		return nil
	}

	got := bot.aiChatText(context.Background(), cfg, 1, "do something")
	if !strings.Contains(got, "Queued AI actions:") {
		t.Fatalf("got=%q", got)
	}
	if !strings.Contains(got, "2 more action(s) omitted") {
		t.Fatalf("got=%q missing omit notice", got)
	}

	// Test chat mode uses Chat not Analyze.
	bot.advisor = &fakeAdvisor{
		enabled: true,
		chatOK:  true,
		chat:    &ai.AnalyzeResult{Answer: "chat answer"},
	}
	got = bot.aiChatText(context.Background(), cfg, 1, "hi")
	if !strings.Contains(got, "chat answer") {
		t.Fatalf("chat mode got=%q", got)
	}
}

func TestQueueAISuggestionNoExecutor(t *testing.T) {
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	bot.executeAI = nil
	out := bot.queueAISuggestion(1, time.Second, ai.Suggestion{Type: "kill", Name: "x.exe", PID: 5})
	if !strings.Contains(out, "(execution unavailable)") {
		t.Fatalf("out=%q", out)
	}
}

func TestQueueAISuggestionError(t *testing.T) {
	old := newConfirmCodeFunc
	newConfirmCodeFunc = func() (string, error) { return "", errors.New("nope") }
	defer func() { newConfirmCodeFunc = old }()
	cfg := config.DefaultConfig()
	bot, _ := newTestBot(t, cfg)
	bot.executeAI = func(s ai.Suggestion) error { return nil }
	out := bot.queueAISuggestion(1, time.Second, ai.Suggestion{Type: "kill", Name: "x.exe", PID: 5})
	if !strings.Contains(out, "(failed to queue confirmation)") {
		t.Fatalf("out=%q", out)
	}
}

func TestQueueAISuggestionSuccess(t *testing.T) {
	cfg := config.DefaultConfig()
	executed := []string{}
	bot, _ := newTestBot(t, cfg)
	bot.executeAI = func(s ai.Suggestion) error { executed = append(executed, s.Type); return nil }
	out := bot.queueAISuggestion(1, time.Second, ai.Suggestion{Type: "kill", Name: "x.exe", PID: 5})
	if !strings.Contains(out, "/confirm ") {
		t.Fatalf("out=%q", out)
	}
	code := extractCode(out)
	if !strings.Contains(bot.confirmAction([]string{code}, 1), "AI action executed") {
		t.Fatalf("confirm should succeed: out=%q", out)
	}
	if len(executed) != 1 || executed[0] != "kill" {
		t.Fatalf("executed=%v", executed)
	}
}

func TestDescribeAISuggestion(t *testing.T) {
	cases := []struct {
		suggestion ai.Suggestion
		desc       string
		success    string
	}{
		{ai.Suggestion{Type: "kill", Name: "x.exe", PID: 5}, "AI: kill x.exe (PID 5)", "AI action executed: killed x.exe (PID 5)"},
		{ai.Suggestion{Type: "kill", PID: 7}, "AI: kill PID 7", "AI action executed: killed PID 7"},
		{ai.Suggestion{Type: "suspend", Name: "y.exe", PID: 8}, "AI: suspend y.exe (PID 8)", "AI action executed: suspended y.exe (PID 8)"},
		{ai.Suggestion{Type: "protect", Name: "z.exe"}, "AI: protect z.exe", "AI action executed: protected z.exe"},
		{ai.Suggestion{Type: "ignore", Name: "i.exe"}, "AI: ignore i.exe", "AI action executed: ignored i.exe"},
		{ai.Suggestion{Type: "add_rule", Rule: &ai.RuleSuggestion{Name: "rule1"}}, "AI: add rule rule1", "AI action executed: rule added rule1"},
		{ai.Suggestion{Type: "weird"}, "AI action", "AI action executed"},
	}
	for _, tc := range cases {
		d, s := describeAISuggestion(tc.suggestion)
		if d != tc.desc || s != tc.success {
			t.Fatalf("suggestion=%+v desc=%q success=%q", tc.suggestion, d, s)
		}
	}
}

func TestIsProtectedProcess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Controller.ProtectedProcesses = []string{"alpha.exe", "  Beta  "}
	if !isProtectedProcess(cfg, "ALPHA.EXE") {
		t.Fatal("alpha.exe should be protected (case-insensitive)")
	}
	if !isProtectedProcess(cfg, "beta") {
		t.Fatal("beta should match '  Beta  ' after trim+case-fold")
	}
	if isProtectedProcess(cfg, "gamma") {
		t.Fatal("gamma should not be protected")
	}
	if isProtectedProcess(nil, "alpha.exe") {
		t.Fatal("nil cfg should return false")
	}
	if isProtectedProcess(cfg, "") {
		t.Fatal("empty name should return false")
	}
}

func TestNormalizeConfirmCode(t *testing.T) {
	if got := normalizeConfirmCode(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	if got := normalizeConfirmCode([]string{""}); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := normalizeConfirmCode([]string{"  abc12  "}); got != "ABC12" {
		t.Fatalf("trim+upper: %q", got)
	}
}

func TestNewConfirmCode(t *testing.T) {
	code, err := newConfirmCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) == 0 {
		t.Fatal("code should be non-empty")
	}
	// All chars are uppercase letters/digits.
	for _, c := range code {
		if !((c >= 'A' && c <= 'Z') || (c >= '2' && c <= '7')) {
			t.Fatalf("unexpected char %q in code %q", c, code)
		}
	}
	// Should be unique.
	c2, _ := newConfirmCode()
	if code == c2 {
		t.Fatalf("codes collided: %q %q", code, c2)
	}
}

func TestNewConfirmCodeRandError(t *testing.T) {
	old := randReadFunc
	randReadFunc = func(p []byte) (int, error) { return 0, errors.New("entropy exhausted") }
	defer func() { randReadFunc = old }()
	if _, err := newConfirmCode(); err == nil {
		t.Fatal("expected error from randReadFunc")
	}
}

func TestDescribeProcess(t *testing.T) {
	if got := describeProcess("chrome.exe", 42); got != "chrome.exe (PID 42)" {
		t.Fatalf("got=%q", got)
	}
	if got := describeProcess("", 42); got != "PID 42" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatConfirmTTL(t *testing.T) {
	if got := formatConfirmTTL(45 * time.Second); got != "45s" {
		t.Fatalf("got=%q", got)
	}
	if got := formatConfirmTTL(2 * time.Minute); got != "2m" {
		t.Fatalf("got=%q", got)
	}
	got := formatConfirmTTL(90 * time.Second)
	if !strings.HasSuffix(got, "s") {
		t.Fatalf("got=%q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Fatalf("got=%q", got)
	}
	if got := truncate("hello", 5); got != "hello" {
		t.Fatalf("got=%q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Fatalf("got=%q", got)
	}
	if got := truncate("hi", 0); got != "" {
		t.Fatalf("got=%q", got)
	}
	if got := truncate("hi", 1); got != "h" {
		t.Fatalf("got=%q", got)
	}
}

func TestTitleWord(t *testing.T) {
	if got := titleWord(""); got != "" {
		t.Fatalf("got=%q", got)
	}
	if got := titleWord("killed"); got != "Killed" {
		t.Fatalf("got=%q", got)
	}
	if got := titleWord("X"); got != "X" {
		t.Fatalf("got=%q", got)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := formatBytes(0); got != "0B" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(512); got != "512B" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(2048); got != "2.0K" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(2 * 1024 * 1024); got != "2.0M" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(3 * 1024 * 1024 * 1024); got != "3.0G" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(4 * 1024 * 1024 * 1024 * 1024); got != "4.0T" {
		t.Fatalf("got=%q", got)
	}
	if got := formatBytes(5 * 1024 * 1024 * 1024 * 1024 * 1024); got != "5.0P" {
		t.Fatalf("got=%q", got)
	}
	// Saturates at P (exp is capped at 4)
	if got := formatBytes(10 * 1024 * 1024 * 1024 * 1024 * 1024 * 1024); got != "10240.0P" {
		t.Fatalf("got=%q", got)
	}
}

func TestMin(t *testing.T) {
	if min(1, 2) != 1 {
		t.Fatal("min(1,2)")
	}
	if min(2, 1) != 1 {
		t.Fatal("min(2,1)")
	}
	if min(3, 3) != 3 {
		t.Fatal("min(3,3)")
	}
}
