package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
)

func TestHandleAIStatusNilAdvisor(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = nil
	rr := httptest.NewRecorder()
	s.handleAIStatus(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestHandleAIStatusWithAdvisor(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = analyzeStubAdvisor{enabled: true, chatFn: func(string) (string, error) { return "", nil }}
	rr := httptest.NewRecorder()
	s.handleAIStatus(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestHandleAIWatchNilAdvisor(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = nil
	rr := httptest.NewRecorder()
	s.handleAIWatch(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestHandleAIWatchWithAdvisor(t *testing.T) {
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = analyzeStubAdvisor{
		enabled: true,
		chatFn:  func(string) (string, error) { return "", nil },
		bg:      ai.BackgroundState{Enabled: true, Configured: true},
	}
	rr := httptest.NewRecorder()
	s.handleAIWatch(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}

// fullAnalyzeAdvisor is a richer stub for Analyze tests.
type fullAnalyzeAdvisor struct {
	enabled bool
	anFn    func(ctx context.Context, prompt string) (*ai.AnalyzeResult, error)
	chatFn  func(ctx context.Context, msg string) (*ai.AnalyzeResult, error)
	bg      ai.BackgroundState
}

func (s fullAnalyzeAdvisor) Enabled() bool          { return s.enabled }
func (s fullAnalyzeAdvisor) Status() map[string]any { return map[string]any{"enabled": s.enabled} }
func (s fullAnalyzeAdvisor) BackgroundState() ai.BackgroundState {
	return s.bg
}
func (s fullAnalyzeAdvisor) Analyze(ctx context.Context, prompt string) (*ai.AnalyzeResult, error) {
	return s.anFn(ctx, prompt)
}
func (s fullAnalyzeAdvisor) Chat(ctx context.Context, msg string) (*ai.AnalyzeResult, error) {
	return s.chatFn(ctx, msg)
}

func TestHandleAIAnalyzeBranches(t *testing.T) {
	// No advisor
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = nil
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIAnalyze(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	// Disabled advisor
	s.advisor = fullAnalyzeAdvisor{enabled: false}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIAnalyze(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	// Bad JSON
	s.advisor = fullAnalyzeAdvisor{enabled: true}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIAnalyze(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// Advisor error
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		anFn: func(ctx context.Context, p string) (*ai.AnalyzeResult, error) {
			return nil, errors.New("rate limit exceeded")
		},
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIAnalyze(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "rate limit") {
		t.Errorf("expected rate-limit message, got %s", rr.Body.String())
	}

	// Happy path with actions → exercise rememberAISuggestions
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		anFn: func(ctx context.Context, p string) (*ai.AnalyzeResult, error) {
			return &ai.AnalyzeResult{
				Answer: "analysis complete",
				Actions: []ai.Suggestion{
					{ID: "sug-1", Type: "kill", PID: 42},
				},
			}, nil
		},
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIAnalyze(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "analysis complete") {
		t.Errorf("body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sug-1") {
		t.Errorf("expected action id in body, got %s", rr.Body.String())
	}
}

func TestHandleAIChatBranches(t *testing.T) {
	// No advisor
	s, _ := fullTestServer(t, "", nil, nil)
	s.advisor = nil
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	// Disabled
	s.advisor = fullAnalyzeAdvisor{enabled: false}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", rr.Code)
	}

	// Bad JSON
	s.advisor = fullAnalyzeAdvisor{enabled: true}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// Empty message
	s.advisor = fullAnalyzeAdvisor{enabled: true}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":""}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// Whitespace message
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}

	// Chat error
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		chatFn: func(ctx context.Context, m string) (*ai.AnalyzeResult, error) {
			return nil, errors.New("ai disabled upstream")
		},
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not configured") {
		t.Errorf("expected 'not configured', got %s", rr.Body.String())
	}

	// Happy path with actions
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		chatFn: func(ctx context.Context, m string) (*ai.AnalyzeResult, error) {
			return &ai.AnalyzeResult{
				Answer:  "answer for " + m,
				Actions: []ai.Suggestion{{ID: "sug-2", Type: "suspend", PID: 7}},
			}, nil
		},
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	s.handleAIChat(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "answer for hi") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestPublicAIErrorMessageBranches(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{nil, "AI provider request failed"},
		{errors.New("rate limit"), "AI rate limit exceeded; try again later"},
		{errors.New("Rate Limit Exceeded"), "AI rate limit exceeded; try again later"},
		{errors.New("advisor disabled"), "AI advisor not configured"},
		{errors.New("random internal error"), "AI provider request failed"},
	}
	for _, c := range cases {
		if got := publicAIErrorMessage(c.in); got != c.want {
			t.Errorf("publicAIErrorMessage(%v)=%q want %q", c.in, got, c.want)
		}
	}
}
