package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

// AIAdvisor is the minimal interface the server needs from the AI package.
type AIAdvisor interface {
	Enabled() bool
	Status() map[string]any
	Analyze(ctx context.Context, prompt string) (*ai.AnalyzeResult, error)
	Chat(ctx context.Context, message string) (*ai.AnalyzeResult, error)
	BackgroundState() ai.BackgroundState
}

// Server is the HTTP server hosting the REST API, SSE stream, and embedded UI.
type Server struct {
	cfg        *config.Config
	cfgPath    string
	onCfgApply func(*config.Config)
	store      *storage.Store
	controller *controller.Controller
	alerts     *anomaly.AlertStore
	emitter    *event.Emitter
	advisor    AIAdvisor
	staticFS   fs.FS
	version    string
	csrfToken  string

	router *Router
	hub    *SSEHub
	aiExec *aiSuggestionStore

	mu      sync.RWMutex
	httpSrv *http.Server
}

// Options bundles every dependency the Server needs.
type Options struct {
	Cfg        *config.Config
	CfgPath    string
	OnCfgApply func(*config.Config) // invoked after a successful in-process config update
	Store      *storage.Store
	Controller *controller.Controller
	Alerts     *anomaly.AlertStore
	Emitter    *event.Emitter
	Advisor    AIAdvisor
	StaticFS   fs.FS
	Version    string
}

func New(opts Options) (*Server, error) {
	version := opts.Version
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	csrfToken, err := newCSRFSafeToken()
	if err != nil {
		return nil, fmt.Errorf("server: generate csrf token: %w", err)
	}
	s := &Server{
		cfg:        opts.Cfg,
		cfgPath:    opts.CfgPath,
		onCfgApply: opts.OnCfgApply,
		store:      opts.Store,
		controller: opts.Controller,
		alerts:     opts.Alerts,
		emitter:    opts.Emitter,
		advisor:    opts.Advisor,
		staticFS:   opts.StaticFS,
		version:    version,
		csrfToken:  csrfToken,
		router:     NewRouter(),
		hub:        NewSSEHub(opts.Emitter),
		aiExec:     newAISuggestionStore(15 * time.Minute),
	}
	s.routes()
	return s, nil
}

// SetConfig hot-swaps the active config (after a watcher reload).
func (s *Server) SetConfig(cfg *config.Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

// Start begins serving HTTP. Returns when the listener exits.
func (s *Server) Start() error {
	s.mu.Lock()
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Keep disabled for long-lived SSE streams; finite write deadlines
		// can terminate healthy dashboards mid-session.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
	s.mu.Unlock()
	log.Printf("HTTP server listening on http://%s", addr)
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully terminates the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	srv := s.httpSrv
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// loggingMiddleware logs every request with method, path, status, duration.
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next(rec, r)
		// #nosec G706 -- method/path are normalized by sanitizeLogToken before logging.
		log.Printf(
			"%s %s %d %s",
			sanitizeLogToken(r.Method),
			sanitizeLogToken(r.URL.Path),
			rec.status,
			time.Since(start),
		)
	}
}

func sanitizeLogToken(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func securityHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next(w, r)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush forwards to the underlying writer so the SSE handler's
// `w.(http.Flusher)` type assertion still works through this wrapper.
// Without this, the stream endpoint returns "streaming unsupported"
// and the dashboard stays stuck on "offline".
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// localOnlyMiddleware rejects any non-loopback request. The HTTP server is
// scoped to local control by design.
func localOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden: invalid remote address", http.StatusForbidden)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden: local-only API", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func mutationGuardMiddleware(csrfToken string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next(w, r)
				return
			}
			if !validOrigin(r) {
				http.Error(w, "forbidden: invalid origin", http.StatusForbidden)
				return
			}
			if r.Header.Get("X-WTM-CSRF") != csrfToken {
				http.Error(w, "forbidden: missing csrf token", http.StatusForbidden)
				return
			}
			next(w, r)
		}
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if !(strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())) {
		return false
	}
	return samePort(u.Port(), requestPort(r.Host))
}

func requestPort(hostport string) string {
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		_ = host
		return port
	}
	return ""
}

func samePort(originPort, requestPort string) bool {
	if originPort == "" {
		originPort = "80"
	}
	if requestPort == "" {
		requestPort = "80"
	}
	return originPort == requestPort
}

// secureRandomRead is the source of cryptographic randomness used by
// newCSRFSafeToken. It is a var (not the direct rand.Read call) so tests
// can substitute a deterministic failure to exercise the error branch.
var secureRandomRead = rand.Read

func newCSRFSafeToken() (string, error) {
	var buf [32]byte
	if _, err := secureRandomRead(buf[:]); err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}
