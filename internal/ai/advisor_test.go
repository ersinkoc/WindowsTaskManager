package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

func TestProviderOf(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "anthropic"},
		{"anthropic", "anthropic"},
		{"Anthropic", "anthropic"},
		{"  ANTHROPIC  ", "anthropic"},
		{"openai", "openai"},
		{"OpenAI", "openai"},
		{"openai-compatible", "openai"},
		{"openrouter", "openai"},
		{"groq", "openai"},
		{"deepseek", "openai"},
		{"ollama", "openai"},
		{"lmstudio", "openai"},
		{"lm-studio", "openai"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		got := providerOf(&config.Config{AI: config.AIConfig{Provider: c.in}})
		if got != c.want {
			t.Errorf("providerOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEffectiveEndpoint(t *testing.T) {
	cases := []struct {
		provider string
		endpoint string
		want     string
	}{
		{"", "", "https://api.anthropic.com/v1/messages"},
		{"anthropic", "", "https://api.anthropic.com/v1/messages"},
		{"openai", "", "https://api.openai.com/v1/chat/completions"},
		{"openai", "http://localhost:11434/v1/chat/completions", "http://localhost:11434/v1/chat/completions"},
		{"groq", "", "https://api.openai.com/v1/chat/completions"}, // alias of openai
	}
	for _, c := range cases {
		got := effectiveEndpoint(&config.Config{AI: config.AIConfig{Provider: c.provider, Endpoint: c.endpoint}})
		if got != c.want {
			t.Errorf("effectiveEndpoint(%q,%q) = %q, want %q", c.provider, c.endpoint, got, c.want)
		}
	}
}

// TestAnthropicCall stands up a fake Anthropic server, runs Analyze, and
// verifies the request shape and the response unwrap.
func TestAnthropicCall(t *testing.T) {
	var receivedBody anthropicRequest
	var receivedKey, receivedVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("x-api-key")
		receivedVersion = r.Header.Get("anthropic-version")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hello from claude"}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		Provider:             "anthropic",
		APIKey:               "sk-ant-test",
		Model:                "claude-test",
		Endpoint:             srv.URL,
		MaxTokens:            128,
		MaxRequestsPerMinute: 60,
		Language:             "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := a.Analyze(ctx, "what is up?")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Answer != "hello from claude" {
		t.Errorf("answer = %q", got.Answer)
	}
	if receivedKey != "sk-ant-test" {
		t.Errorf("x-api-key = %q", receivedKey)
	}
	if receivedVersion != "2023-06-01" {
		t.Errorf("anthropic-version = %q", receivedVersion)
	}
	if receivedBody.Model != "claude-test" {
		t.Errorf("model = %q", receivedBody.Model)
	}
	if len(receivedBody.Messages) != 1 || receivedBody.Messages[0].Role != "user" {
		t.Errorf("messages = %+v", receivedBody.Messages)
	}
	if !strings.Contains(receivedBody.Messages[0].Content, "what is up?") {
		t.Errorf("user question not propagated: %q", receivedBody.Messages[0].Content)
	}
}

// TestOpenAICall covers the /v1/chat/completions path with a fake server,
// including the Bearer auth and extra-headers passthrough.
func TestOpenAICall(t *testing.T) {
	var receivedBody openaiRequest
	var receivedAuth, receivedReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedReferer = r.Header.Get("HTTP-Referer")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello from gpt"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		Provider:             "openai",
		APIKey:               "sk-test",
		Model:                "gpt-test",
		Endpoint:             srv.URL,
		MaxTokens:            128,
		MaxRequestsPerMinute: 60,
		Language:             "en",
		ExtraHeaders:         map[string]string{"HTTP-Referer": "http://wtm.local"},
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := a.Analyze(ctx, "diagnose")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Answer != "hello from gpt" {
		t.Errorf("answer = %q", got.Answer)
	}
	if receivedAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", receivedAuth)
	}
	if receivedReferer != "http://wtm.local" {
		t.Errorf("HTTP-Referer = %q", receivedReferer)
	}
	if receivedBody.Model != "gpt-test" {
		t.Errorf("model = %q", receivedBody.Model)
	}
	if len(receivedBody.Messages) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(receivedBody.Messages))
	}
	if receivedBody.Messages[0].Role != "system" || receivedBody.Messages[1].Role != "user" {
		t.Errorf("messages = %+v", receivedBody.Messages)
	}
	if !strings.Contains(receivedBody.Messages[1].Content, "diagnose") {
		t.Errorf("user question not propagated: %q", receivedBody.Messages[1].Content)
	}
}

func TestOpenAICallRejectsOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat("x", maxProviderResponseBytes+64))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		Provider:             "openai",
		APIKey:               "sk-test",
		Model:                "gpt-test",
		Endpoint:             srv.URL,
		MaxTokens:            128,
		MaxRequestsPerMinute: 60,
		Language:             "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)

	_, err := a.Analyze(context.Background(), "diagnose")
	if err == nil || !strings.Contains(err.Error(), "provider response exceeds") {
		t.Fatalf("expected oversized provider body error, got %v", err)
	}
}

func TestAnalyzeDisabled(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{Enabled: false, MaxRequestsPerMinute: 5}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	if _, err := a.Analyze(context.Background(), "x"); err == nil {
		t.Error("expected error when disabled")
	}
}

func TestUnknownProviderError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "ollama-rest", APIKey: "x", MaxRequestsPerMinute: 5,
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	_, err := a.Analyze(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "unknown AI provider") {
		t.Errorf("expected unknown provider error, got %v", err)
	}
}

func TestChatCarriesRecentConversation(t *testing.T) {
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req openaiRequest
		_ = json.Unmarshal(body, &req)
		if len(req.Messages) > 1 {
			prompts = append(prompts, req.Messages[1].Content)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"chat reply"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srv.URL
	cfg.AI.MaxTokens = 128
	cfg.AI.MaxRequestsPerMinute = 60

	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)
	if _, err := a.Chat(context.Background(), "What is using the CPU?"); err != nil {
		t.Fatalf("first chat: %v", err)
	}
	if _, err := a.Chat(context.Background(), "What should I kill first?"); err != nil {
		t.Fatalf("second chat: %v", err)
	}
	if len(prompts) != 2 {
		t.Fatalf("prompts=%d want 2", len(prompts))
	}
	if strings.Contains(prompts[0], "## RECENT CHAT") {
		t.Fatalf("first prompt should not include chat history: %q", prompts[0])
	}
	if !strings.Contains(prompts[1], "## RECENT CHAT") {
		t.Fatalf("second prompt missing history: %q", prompts[1])
	}
	if !strings.Contains(prompts[1], "User: What is using the CPU?") {
		t.Fatalf("second prompt missing prior user turn: %q", prompts[1])
	}
	if !strings.Contains(prompts[1], "Assistant: chat reply") {
		t.Fatalf("second prompt missing prior assistant turn: %q", prompts[1])
	}
}

func TestBackgroundWatchRunsOnCriticalAlert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"background diagnosis<actions>[{\"type\":\"ignore\",\"name\":\"node.exe\",\"reason\":\"known dev burst\"}]</actions>"}}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srv.URL
	cfg.AI.MaxTokens = 256
	cfg.AI.Scheduler.Enabled = true
	cfg.AI.Scheduler.MinInterval = 0
	cfg.AI.Scheduler.MaxCyclesPerHour = 5
	cfg.AI.Scheduler.MaxReservedTokensPerDay = 5000
	cfg.AI.Scheduler.CooldownAfterError = 0
	cfg.AI.Scheduler.HistoryLimit = 4
	cfg.AI.AutoAnalyzeOnCritical = true

	em := event.NewEmitter()
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, em)

	em.Emit(anomaly.EventAlertRaised, anomaly.Alert{
		ID:          "runaway_cpu/123",
		Type:        "runaway_cpu",
		Severity:    anomaly.SeverityCritical,
		Title:       "Runaway CPU",
		Description: "node.exe is burning CPU",
		PID:         123,
		ProcessName: "node.exe",
	})

	waitFor(t, 2*time.Second, func() bool {
		return a.BackgroundState().LastRun != nil
	})

	state := a.BackgroundState()
	if state.LastRun == nil {
		t.Fatal("expected a background run")
	}
	if state.LastRun.AlertType != "runaway_cpu" {
		t.Fatalf("alert_type=%q want runaway_cpu", state.LastRun.AlertType)
	}
	if !strings.Contains(state.LastRun.Answer, "background diagnosis") {
		t.Fatalf("answer=%q", state.LastRun.Answer)
	}
	if len(state.LastRun.Actions) != 1 || state.LastRun.Actions[0].Type != "ignore" {
		t.Fatalf("actions=%+v", state.LastRun.Actions)
	}
}

func TestBackgroundWatchHonorsCycleLimit(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srv.URL
	cfg.AI.MaxTokens = 256
	cfg.AI.Scheduler.Enabled = true
	cfg.AI.Scheduler.MinInterval = 0
	cfg.AI.Scheduler.MaxCyclesPerHour = 1
	cfg.AI.Scheduler.MaxReservedTokensPerDay = 5000
	cfg.AI.Scheduler.CooldownAfterError = 0
	cfg.AI.AutoAnalyzeOnCritical = true

	em := event.NewEmitter()
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, em)

	first := anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "first", PID: 1, ProcessName: "node.exe"}
	second := anomaly.Alert{Type: "memory_leak", Severity: anomaly.SeverityCritical, Title: "second", PID: 2, ProcessName: "chrome.exe"}

	em.Emit(anomaly.EventAlertRaised, first)
	waitFor(t, 2*time.Second, func() bool {
		return a.BackgroundState().LastRun != nil
	})

	em.Emit(anomaly.EventAlertRaised, second)
	time.Sleep(100 * time.Millisecond)

	state := a.BackgroundState()
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
	if state.LastSkipReason != "cycle_limit" {
		t.Fatalf("last_skip_reason=%q want cycle_limit", state.LastSkipReason)
	}
	if len(state.RecentRuns) != 1 {
		t.Fatalf("recent_runs=%d want 1", len(state.RecentRuns))
	}
}

func TestBackgroundWatchEvaluatesAutoPolicy(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok<actions>[{\"type\":\"ignore\",\"name\":\"node.exe\",\"reason\":\"known dev workload\"}]</actions>"}}]}`))
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srv.URL
	cfg.AI.MaxTokens = 256
	cfg.AI.Scheduler.Enabled = true
	cfg.AI.Scheduler.MinInterval = 0
	cfg.AI.Scheduler.MaxCyclesPerHour = 5
	cfg.AI.Scheduler.MaxReservedTokensPerDay = 5000
	cfg.AI.Scheduler.CooldownAfterError = 0
	cfg.AI.AutoAnalyzeOnCritical = true
	cfg.AI.AutoAction.Enabled = true
	cfg.AI.AutoAction.DryRun = true
	cfg.AI.AutoAction.AllowedActions = []string{"ignore"}
	cfg.AI.AutoAction.RequireRepeatCycles = 2

	em := event.NewEmitter()
	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, em)

	alert := anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "cpu", PID: 123, ProcessName: "node.exe"}

	em.Emit(anomaly.EventAlertRaised, alert)
	waitFor(t, 2*time.Second, func() bool {
		return len(a.BackgroundState().RecentRuns) >= 1
	})

	first := a.BackgroundState().LastRun
	if first == nil || len(first.Actions) != 1 || first.Actions[0].Policy == nil {
		t.Fatalf("first run policy missing: %+v", first)
	}
	if first.Actions[0].Policy.Status != "needs_repeat" {
		t.Fatalf("first policy=%+v want needs_repeat", first.Actions[0].Policy)
	}

	em.Emit(anomaly.EventAlertRaised, alert)
	waitFor(t, 2*time.Second, func() bool {
		state := a.BackgroundState()
		return len(state.RecentRuns) >= 2
	})

	second := a.BackgroundState().LastRun
	if second == nil || len(second.Actions) != 1 || second.Actions[0].Policy == nil {
		t.Fatalf("second run policy missing: %+v", second)
	}
	if second.Actions[0].Policy.Status != "dry_run_eligible" {
		t.Fatalf("second policy=%+v want dry_run_eligible", second.Actions[0].Policy)
	}
	if second.AutoCandidates != 1 {
		t.Fatalf("auto_candidates=%d want 1", second.AutoCandidates)
	}
	if calls < 1 {
		t.Fatalf("calls=%d want >=1", calls)
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for condition")
}

func TestBackgroundRunUsesScheduledConfigSnapshot(t *testing.T) {
	srvOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"from scheduled config"}}]}`))
	}))
	defer srvOne.Close()

	srvTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"from live config"}}]}`))
	}))
	defer srvTwo.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srvOne.URL
	cfg.AI.MaxTokens = 256
	cfg.AI.MaxRequestsPerMinute = 60

	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)

	liveCfg := cloneConfig(cfg)
	liveCfg.AI.Endpoint = srvTwo.URL
	a.SetConfig(liveCfg)

	alert := anomaly.Alert{Type: "runaway_cpu", Severity: anomaly.SeverityCritical, Title: "cpu", PID: 123, ProcessName: "node.exe"}
	a.runBackgroundAnalysis(cloneConfig(cfg), alert, cfg.AI.MaxTokens, time.Now())

	run := a.BackgroundState().LastRun
	if run == nil {
		t.Fatal("expected a background run")
	}
	if !strings.Contains(run.Answer, "from scheduled config") {
		t.Fatalf("answer=%q want scheduled config response", run.Answer)
	}
}

func TestBackgroundRunHonorsRootContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Provider = "openai"
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = srv.URL
	cfg.AI.MaxTokens = 256
	cfg.AI.MaxRequestsPerMinute = 60

	a := NewAdvisor(cfg, storage.NewStore(60, 10), func() []anomaly.Alert { return nil }, nil)
	rootCtx, cancel := context.WithCancel(context.Background())
	a.Start(rootCtx)
	cancel()

	done := make(chan struct{})
	go func() {
		a.runBackgroundAnalysis(cloneConfig(cfg), anomaly.Alert{
			Type:        "runaway_cpu",
			Severity:    anomaly.SeverityCritical,
			Title:       "cpu",
			PID:         123,
			ProcessName: "node.exe",
		}, cfg.AI.MaxTokens, time.Now())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("background run did not stop after root context cancellation")
	}

	run := a.BackgroundState().LastRun
	if run == nil {
		t.Fatal("expected a background run result")
	}
	if run.Error == "" || !strings.Contains(strings.ToLower(run.Error), "context canceled") {
		t.Fatalf("error=%q want context canceled", run.Error)
	}
}

// TestCallAnthropicMarshalError exercises the json.Marshal error branch in
// callAnthropic by overriding marshalProviderBody to return an error.
func TestCallAnthropicMarshalError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		Provider:             "anthropic",
		APIKey:               "sk",
		Model:                "m",
		Endpoint:             "http://localhost:1",
		MaxTokens:            64,
		MaxRequestsPerMinute: 60,
		Language:             "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	orig := marshalProviderBody
	marshalProviderBody = func(v any) ([]byte, error) {
		return nil, errors.New("synthetic marshal failure")
	}
	defer func() { marshalProviderBody = orig }()

	_, _, err := a.callAnthropic(context.Background(), cfg, "x")
	if err == nil || !strings.Contains(err.Error(), "synthetic marshal failure") {
		t.Errorf("expected synthetic marshal failure, got %v", err)
	}
}

// TestCallOpenAIMarshalError exercises the json.Marshal error branch in
// callOpenAI by overriding marshalProviderBody to return an error.
func TestCallOpenAIMarshalError(t *testing.T) {
	cfg := &config.Config{AI: config.AIConfig{
		Enabled:              true,
		Provider:             "openai",
		APIKey:               "k",
		Model:                "m",
		Endpoint:             "http://localhost:1",
		MaxTokens:            64,
		MaxRequestsPerMinute: 60,
		Language:             "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)

	orig := marshalProviderBody
	marshalProviderBody = func(v any) ([]byte, error) {
		return nil, errors.New("synthetic openai marshal failure")
	}
	defer func() { marshalProviderBody = orig }()

	_, _, err := a.callOpenAI(context.Background(), cfg, "x")
	if err == nil || !strings.Contains(err.Error(), "synthetic openai marshal failure") {
		t.Errorf("expected synthetic openai marshal failure, got %v", err)
	}
}

// TestCallProviderRoutesBothProviders ensures both callAnthropic and
// callOpenAI execute through the callProvider dispatcher (the default
// branch is already covered by TestUnknownProviderError).
func TestCallProviderRoutesAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "anthropic", APIKey: "sk",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	ans, usage, err := a.callProvider(context.Background(), cfg, "x")
	if err != nil {
		t.Fatalf("callProvider: %v", err)
	}
	if ans != "hi" {
		t.Errorf("ans=%q", ans)
	}
	if usage != nil {
		t.Errorf("expected nil usage for response without usage block, got %+v", usage)
	}
}

func TestCallProviderRoutesOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Enabled: true, Provider: "openai", APIKey: "k",
		Endpoint: srv.URL, Model: "m", MaxTokens: 64, MaxRequestsPerMinute: 60, Language: "en",
	}}
	a := NewAdvisor(cfg, storage.NewStore(60, 10), nil, nil)
	ans, _, err := a.callProvider(context.Background(), cfg, "x")
	if err != nil {
		t.Fatalf("callProvider: %v", err)
	}
	if ans != "ok" {
		t.Errorf("ans=%q", ans)
	}
}

// TestMarshalProviderBodyDefaultIsJSONMarshal guards against accidental
// reassignment of the package-level variable at init time.
func TestMarshalProviderBodyDefaultIsJSONMarshal(t *testing.T) {
	if marshalProviderBody == nil {
		t.Fatal("marshalProviderBody is nil")
	}
	// json.Marshal of a struct with only primitive fields cannot fail.
	out, err := marshalProviderBody(struct {
		A string `json:"a"`
	}{A: "x"})
	if err != nil {
		t.Fatalf("default marshal: %v", err)
	}
	if string(out) != `{"a":"x"}` {
		t.Errorf("got %s", out)
	}
}
