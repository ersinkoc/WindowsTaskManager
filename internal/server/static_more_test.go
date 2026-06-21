package server

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestServeStaticNoFS(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestServeStaticSPAFallback(t *testing.T) {
	s := &Server{
		staticFS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<html><body>__WTM_CSRF_TOKEN__</body></html>`)},
		},
		csrfToken: "csrf-tok",
	}
	// Missing asset → SPA fallback to index.html.
	req := httptest.NewRequest(http.MethodGet, "/some/missing/asset.js", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "csrf-tok") {
		t.Errorf("expected csrf token injection: %s", rr.Body.String())
	}
}

func TestServeStaticIndex(t *testing.T) {
	s := &Server{
		staticFS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<html>__WTM_CSRF_TOKEN__</html>`)},
		},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "tok") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestServeStaticRootRewritesToIndex(t *testing.T) {
	s := &Server{
		staticFS: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<html>__WTM_CSRF_TOKEN__</html>`)},
		},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestServeStaticRegularAsset(t *testing.T) {
	s := &Server{
		staticFS: fstest.MapFS{
			"app.js": &fstest.MapFile{Data: []byte(`console.log("ok")`)},
		},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "console.log") {
		t.Errorf("body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "javascript") {
		t.Errorf("content-type=%s", rr.Header().Get("Content-Type"))
	}
}

func TestServeStaticIndexFallbackWithNoIndex(t *testing.T) {
	// Both the requested asset AND the index.html are missing → 404.
	s := &Server{
		staticFS:  fstest.MapFS{},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/missing.js", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestServeIndexHTMLErrors(t *testing.T) {
	// Use a reader that always errors.
	rr := httptest.NewRecorder()
	serveIndexHTML(rr, &errReader{}, "tok")
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rr.Code)
	}
}

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, errAlways }

var errAlways = errAlwaysErr{}

type errAlwaysErr struct{}

func (errAlwaysErr) Error() string { return "always fails" }

func TestServeStaticWriteError(t *testing.T) {
	// io.Copy returning an error is the only way to hit the log branch in
	// serveStatic. Use a writer that errors on Write.
	s := &Server{
		staticFS: fstest.MapFS{
			"app.js": &fstest.MapFile{Data: []byte(`console.log("ok")`)},
		},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rr := &failingWriter{}
	s.serveStatic(rr, req)
	// The point is that no panic happens and the log branch executes
	// (visible in test output).
	if rr.code != 0 && rr.code != http.StatusOK {
		t.Errorf("unexpected status code=%d", rr.code)
	}
}

type failingWriter struct {
	headerMap http.Header
	code      int
	body      string
}

func (f *failingWriter) Header() http.Header {
	if f.headerMap == nil {
		f.headerMap = make(http.Header)
	}
	return f.headerMap
}
func (f *failingWriter) Write(b []byte) (int, error) {
	return 0, errors.New("write failed")
}
func (f *failingWriter) WriteHeader(code int) { f.code = code }

func TestServeIndexHTMLWriteError(t *testing.T) {
	// The write-error branch in serveIndexHTML.
	rr := &failingWriter{}
	serveIndexHTML(rr, strings.NewReader("<html>__WTM_CSRF_TOKEN__</html>"), "tok")
	// We just need to exercise the path; whether WriteHeader gets called
	// depends on the writer implementation.
}

func TestServeStaticStatError(t *testing.T) {
	// A file whose Open succeeds but whose Stat fails.
	s := &Server{
		staticFS: &statFailingFS{inner: fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte(`<html>x</html>`)},
		}},
		csrfToken: "tok",
	}
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rr := httptest.NewRecorder()
	s.serveStatic(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// statFailingFS wraps an fs.FS and forces Stat() to fail on every file.
type statFailingFS struct {
	inner fstest.MapFS
}

func (s *statFailingFS) Open(name string) (fs.File, error) {
	f, err := s.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return &statFailingFile{File: f}, nil
}

type statFailingFile struct {
	fs.File
}

func (s *statFailingFile) Stat() (fs.FileInfo, error) {
	return nil, errors.New("stat failure")
}

func TestContentTypeForAllExtensions(t *testing.T) {
	cases := map[string]string{
		"a.html":    "text/html; charset=utf-8",
		"a.htm":     "application/octet-stream",
		"a.css":     "text/css; charset=utf-8",
		"a.js":      "application/javascript; charset=utf-8",
		"a.mjs":     "application/javascript; charset=utf-8",
		"a.json":    "application/json",
		"a.svg":     "image/svg+xml",
		"a.jpg":     "image/jpeg",
		"a.jpeg":    "image/jpeg",
		"a.png":     "image/png",
		"a.webp":    "image/webp",
		"a.ico":     "image/x-icon",
		"a.map":     "application/json",
		"a.woff":    "font/woff",
		"a.woff2":   "font/woff2",
		"a.unknown": "application/octet-stream",
		"":          "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentTypeFor(name); got != want {
			t.Errorf("contentTypeFor(%q)=%q want %q", name, got, want)
		}
	}
}
