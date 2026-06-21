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

func TestHandleAITestBranches(t *testing.T) {
	// No API key → 400 no_api_key
	s, _ := fullTestServer(t, "", nil, nil)
	cfg := defaultCfg()
	cfg.AI.APIKey = ""
	s.cfg = cfg
	rr := httptest.NewRecorder()
	s.handleAITest(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "no_api_key") {
		t.Errorf("body=%s", rr.Body.String())
	}

	// No endpoint → 400 no_endpoint
	cfg.AI.APIKey = "sk-test"
	cfg.AI.Endpoint = ""
	s.cfg = cfg
	rr = httptest.NewRecorder()
	s.handleAITest(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "no_endpoint") {
		t.Errorf("body=%s", rr.Body.String())
	}

	// Disabled → 400 ai_disabled
	cfg.AI.Endpoint = "https://example.com"
	cfg.AI.Enabled = false
	s.cfg = cfg
	rr = httptest.NewRecorder()
	s.handleAITest(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "ai_disabled") {
		t.Errorf("body=%s", rr.Body.String())
	}

	// Chat error → 502
	cfg.AI.Enabled = true
	s.cfg = cfg
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		chatFn: func(ctx context.Context, msg string) (*ai.AnalyzeResult, error) {
			return nil, errors.New("upstream timed out")
		},
	}
	rr = httptest.NewRecorder()
	s.handleAITest(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Happy path
	s.advisor = fullAnalyzeAdvisor{
		enabled: true,
		chatFn: func(ctx context.Context, msg string) (*ai.AnalyzeResult, error) {
			return &ai.AnalyzeResult{Answer: "ok"}, nil
		},
	}
	rr = httptest.NewRecorder()
	s.handleAITest(rr, httptest.NewRequest(http.MethodPost, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Errorf("body=%s", rr.Body.String())
	}
}
