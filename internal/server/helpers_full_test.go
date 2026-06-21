package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONNilPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusNoContent, nil)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Content-Type") == "" {
		t.Fatal("content type should be set even on nil payload")
	}
}

func TestWriteJSONEncodesPayload(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"k": "v"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"k":"v"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestReadJSONRejectsNilBody(t *testing.T) {
	// httptest.NewRequest converts nil body to http.NoBody, so we have to
	// construct the request manually with a truly nil Body.
	req := &http.Request{
		Method: http.MethodPost,
		URL:    nil,
		Header: http.Header{},
		Body:   nil,
	}
	rr := httptest.NewRecorder()
	var dst struct {
		Name string `json:"name"`
	}
	if readJSON(rr, req, &dst) {
		t.Fatal("expected failure for nil body")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestReadJSONRejectsMalformed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	var dst struct {
		Name string `json:"name"`
	}
	if readJSON(rr, req, &dst) {
		t.Fatal("expected failure for malformed body")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestReadJSONRejectsUnknownField(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"x","extra":1}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	var dst struct {
		Name string `json:"name"`
	}
	if readJSON(rr, req, &dst) {
		t.Fatal("expected failure for unknown field")
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestReadJSONAcceptsValid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	var dst struct {
		Name string `json:"name"`
	}
	if !readJSON(rr, req, &dst) {
		t.Fatalf("expected success; body=%s", rr.Body.String())
	}
	if dst.Name != "alice" {
		t.Fatalf("Name=%q", dst.Name)
	}
}
