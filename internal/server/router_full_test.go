package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterDefault404(t *testing.T) {
	rt := NewRouter()
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestServeHTTPRoutesParamAndSetsContext(t *testing.T) {
	rt := NewRouter()
	called := false
	rt.GET("/x/:name", func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := Param(r, "name"); got != "alice" {
			t.Errorf("name=%q want alice", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/x/alice", nil)
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusNoContent {
		t.Fatalf("called=%v code=%d", called, rr.Code)
	}
}

func TestServeHTTPMiddlewareWrapsInReverseOrder(t *testing.T) {
	rt := NewRouter()
	var order []string
	rt.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "first")
			next(w, r)
		}
	})
	rt.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "second")
			next(w, r)
		}
	})
	rt.GET("/x", func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
		w.WriteHeader(http.StatusOK)
	})
	rt.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	// Middleware wraps handler from end to start, so the outermost wrapper
	// is the *first* registered middleware (which is the last one applied).
	if len(order) != 3 || order[0] != "first" || order[1] != "second" || order[2] != "handler" {
		t.Fatalf("order=%v", order)
	}
}

func TestMatchReturnsFalseOnNoRoute(t *testing.T) {
	rt := NewRouter()
	rt.GET("/a/b", func(w http.ResponseWriter, r *http.Request) {})
	if h, _, ok := rt.match("GET", []string{"a", "c"}); ok || h != nil {
		t.Fatalf("expected no match")
	}
}

func TestSetNotFoundOverridesDefault(t *testing.T) {
	rt := NewRouter()
	rt.SetNotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	rr := httptest.NewRecorder()
	rt.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rr.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestParamReturnsEmptyForMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := Param(req, "missing"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitPathEdgeCases(t *testing.T) {
	if got := splitPath(""); got != nil {
		t.Errorf("splitPath empty=%v", got)
	}
	if got := splitPath("/"); got != nil {
		t.Errorf("splitPath /=%v", got)
	}
	if got := splitPath("//a//"); len(got) != 1 || got[0] != "a" {
		t.Errorf("splitPath //a//=%v", got)
	}
}
