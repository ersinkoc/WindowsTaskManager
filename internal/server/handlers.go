package server

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/controller"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
)

// routes wires every endpoint onto the router.
func (s *Server) routes() {
	s.router.Use(loggingMiddleware)
	s.router.Use(securityHeadersMiddleware)
	s.router.Use(localOnlyMiddleware)
	s.router.Use(mutationGuardMiddleware(s.csrfToken))

	// System endpoints.
	s.router.GET("/api/v1/system", s.handleSystem)
	s.router.GET("/api/v1/cpu", s.handleCPU)
	s.router.GET("/api/v1/memory", s.handleMemory)
	s.router.GET("/api/v1/gpu", s.handleGPU)
	s.router.GET("/api/v1/disk", s.handleDisk)
	s.router.GET("/api/v1/network", s.handleNetwork)
	s.router.GET("/api/v1/history", s.handleHistory)
	s.router.GET("/api/v1/info", s.handleInfo)
	s.router.GET("/api/v1/health", s.handleHealth)

	// Process endpoints.
	s.router.GET("/api/v1/processes", s.handleProcesses)
	s.router.GET("/api/v1/processes/tree", s.handleProcessTree)
	s.router.GET("/api/v1/processes/:pid", s.handleProcessByID)
	s.router.GET("/api/v1/processes/:pid/history", s.handleProcessHistory)
	s.router.GET("/api/v1/processes/:pid/children", s.handleProcessChildren)
	s.router.GET("/api/v1/processes/:pid/connections", s.handleProcessConnections)
	s.router.POST("/api/v1/processes/:pid/kill", s.handleKill)
	s.router.POST("/api/v1/processes/:pid/kill-tree", s.handleKillTree)
	s.router.POST("/api/v1/processes/:pid/suspend", s.handleSuspend)
	s.router.POST("/api/v1/processes/:pid/resume", s.handleResume)
	s.router.POST("/api/v1/processes/:pid/priority", s.handlePriority)
	s.router.POST("/api/v1/processes/:pid/affinity", s.handleAffinity)
	s.router.POST("/api/v1/processes/:pid/limit", s.handleLimit)
	s.router.DELETE("/api/v1/processes/:pid/limit", s.handleClearLimit)
	s.router.GET("/api/v1/processes/limits", s.handleListLimits)

	// Port and connection endpoints.
	s.router.GET("/api/v1/ports", s.handlePorts)
	s.router.GET("/api/v1/connections", s.handleConnections)

	// Anomaly endpoints.
	s.router.GET("/api/v1/alerts", s.handleAlerts)
	s.router.GET("/api/v1/alerts/history", s.handleAlertHistory)
	s.router.POST("/api/v1/alerts/clear", s.handleAlertsClear)
	s.router.POST("/api/v1/alerts/:type/dismiss", s.handleAlertDismiss)
	s.router.POST("/api/v1/alerts/:type/:pid/dismiss", s.handleAlertDismiss)
	s.router.POST("/api/v1/alerts/:type/snooze", s.handleAlertSnooze)
	s.router.POST("/api/v1/alerts/:type/:pid/snooze", s.handleAlertSnooze)

	// Config endpoints.
	s.router.GET("/api/v1/config", s.handleConfigGet)
	s.router.PUT("/api/v1/config", s.handleConfigUpdate)

	// AI advisor endpoints.
	s.router.GET("/api/v1/ai/status", s.handleAIStatus)
	s.router.GET("/api/v1/ai/watch", s.handleAIWatch)
	s.router.POST("/api/v1/ai/analyze", s.handleAIAnalyze)
	s.router.POST("/api/v1/ai/chat", s.handleAIChat)
	s.router.POST("/api/v1/ai/execute", s.handleAIExecute)
	s.router.GET("/api/v1/ai/config", s.handleAIConfigGet)
	s.router.POST("/api/v1/ai/config", s.handleAIConfigUpdate)
	s.router.GET("/api/v1/ai/presets", s.handleAIPresets)
	s.router.GET("/api/v1/ai/models", s.handleAIModels)
	s.router.POST("/api/v1/ai/test", s.handleAITest)
	s.router.GET("/api/v1/telegram/config", s.handleTelegramConfigGet)
	s.router.POST("/api/v1/telegram/config", s.handleTelegramConfigUpdate)
	s.router.POST("/api/v1/telegram/test", s.handleTelegramTest)

	// User-defined automation rules.
	s.router.GET("/api/v1/rules", s.handleRulesGet)
	s.router.POST("/api/v1/rules", s.handleRulesUpdate)

	// Per-process protect / ignore toggles.
	s.router.POST("/api/v1/config/protect", s.handleConfigProtectToggle)
	s.router.POST("/api/v1/config/ignore", s.handleConfigIgnoreToggle)

	// SSE stream and embedded UI.
	s.router.GET("/api/v1/stream", s.hub.Handler())
	s.router.SetNotFound(s.serveStatic)
}

// ----- system handlers -----

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleCPU(w http.ResponseWriter, r *http.Request) {
	if snap := s.store.Latest(); snap != nil {
		writeJSON(w, http.StatusOK, snap.CPU)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	if snap := s.store.Latest(); snap != nil {
		writeJSON(w, http.StatusOK, snap.Memory)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
}

func (s *Server) handleGPU(w http.ResponseWriter, r *http.Request) {
	if snap := s.store.Latest(); snap != nil {
		writeJSON(w, http.StatusOK, snap.GPU)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
}

func (s *Server) handleDisk(w http.ResponseWriter, r *http.Request) {
	if snap := s.store.Latest(); snap != nil {
		writeJSON(w, http.StatusOK, snap.Disk)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	if snap := s.store.Latest(); snap != nil {
		writeJSON(w, http.StatusOK, snap.Network)
		return
	}
	writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")
	if since == "" {
		writeJSON(w, http.StatusOK, s.store.SystemHistory())
		return
	}
	secs, err := strconv.ParseInt(since, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_since", "since must be unix seconds")
		return
	}
	writeJSON(w, http.StatusOK, s.store.SystemHistorySince(time.Unix(secs, 0)))
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":         s.version,
		"go_version":      runtime.Version(),
		"num_cpu":         runtime.NumCPU(),
		"goroutines":      runtime.NumGoroutine(),
		"self_pid":        os.Getpid(),
		"interval_ms":     cfg.Monitoring.Interval.Milliseconds(),
		"history_minutes": cfg.Monitoring.HistoryDuration.Minutes(),
		"sse_clients":     s.hub.ClientCount(),
		"tracked_pids":    s.store.TrackedProcessCount(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ----- process handlers -----

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	q := r.URL.Query()
	procs := append([]metrics.ProcessInfo(nil), snap.Processes...)

	if filter := strings.ToLower(q.Get("name")); filter != "" {
		out := procs[:0]
		for _, p := range procs {
			if strings.Contains(strings.ToLower(p.Name), filter) {
				out = append(out, p)
			}
		}
		procs = out
	}

	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "cpu"
	}
	desc := q.Get("order") != "asc"
	sortProcesses(procs, sortBy, desc)

	if limitStr := q.Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n < len(procs) {
			procs = procs[:n]
		}
	}
	writeJSON(w, http.StatusOK, procs)
}

func sortProcesses(procs []metrics.ProcessInfo, by string, desc bool) {
	less := func(i, j int) bool { return procs[i].PID < procs[j].PID }
	switch by {
	case "cpu":
		less = func(i, j int) bool { return procs[i].CPUPercent < procs[j].CPUPercent }
	case "memory", "ws":
		less = func(i, j int) bool { return procs[i].WorkingSet < procs[j].WorkingSet }
	case "private":
		less = func(i, j int) bool { return procs[i].PrivateBytes < procs[j].PrivateBytes }
	case "name":
		less = func(i, j int) bool { return procs[i].Name < procs[j].Name }
	case "pid":
		less = func(i, j int) bool { return procs[i].PID < procs[j].PID }
	case "threads":
		less = func(i, j int) bool { return procs[i].ThreadCount < procs[j].ThreadCount }
	}
	if desc {
		orig := less
		less = func(i, j int) bool { return orig(j, i) }
	}
	sort.Slice(procs, less)
}

func (s *Server) handleProcessTree(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	writeJSON(w, http.StatusOK, snap.ProcessTree)
}

func (s *Server) handleProcessByID(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	for i := range snap.Processes {
		if snap.Processes[i].PID == pid {
			writeJSON(w, http.StatusOK, snap.Processes[i])
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "no such pid")
}

func (s *Server) handleProcessHistory(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.store.ProcessHistory(pid))
}

func (s *Server) handleProcessChildren(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	children := make([]metrics.ProcessInfo, 0)
	for _, proc := range snap.Processes {
		if proc.ParentPID == pid {
			children = append(children, proc)
		}
	}
	writeJSON(w, http.StatusOK, children)
}

func (s *Server) handleProcessConnections(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	rows := make([]metrics.PortBinding, 0)
	for _, binding := range snap.PortBindings {
		if binding.PID == pid {
			rows = append(rows, binding)
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := s.controller.Kill(pid, confirm); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid})
}

func (s *Server) handleKillTree(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	killed, err := s.controller.KillTree(pid, confirm)
	if err != nil && killed == 0 {
		s.controllerError(w, err)
		return
	}
	resp := map[string]any{"ok": true, "killed": killed}
	if err != nil {
		resp["partial_error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleSuspend(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := s.controller.Suspend(pid, confirm); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid})
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	if err := s.controller.Resume(pid); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid})
}

func (s *Server) handlePriority(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Class string `json:"class"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := s.controller.SetPriority(pid, body.Class, confirm); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid, "class": body.Class})
}

func (s *Server) handleAffinity(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Mask uint64 `json:"mask"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := s.controller.SetAffinity(pid, body.Mask, confirm); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid, "mask": body.Mask})
}

func (s *Server) handleLimit(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	var body struct {
		CPUPct   int    `json:"cpu_pct"`
		MemBytes uint64 `json:"mem_bytes"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	confirm := r.URL.Query().Get("confirm") == "true"
	if err := s.controller.Limit(pid, body.CPUPct, body.MemBytes, confirm); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid})
}

func (s *Server) handleClearLimit(w http.ResponseWriter, r *http.Request) {
	pid, ok := parsePIDParam(w, r)
	if !ok {
		return
	}
	if err := s.controller.ClearLimit(pid); err != nil {
		s.controllerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pid": pid})
}

func (s *Server) handleListLimits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.controller.ActiveLimits())
}

func (s *Server) controllerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, controller.ErrProtected):
		writeError(w, http.StatusForbidden, "protected", err.Error())
	case errors.Is(err, controller.ErrCritical):
		writeError(w, http.StatusForbidden, "critical", err.Error())
	case errors.Is(err, controller.ErrSelf):
		writeError(w, http.StatusForbidden, "self_protected", err.Error())
	case errors.Is(err, controller.ErrConfirmNeeded):
		writeError(w, http.StatusConflict, "confirm_required", err.Error())
	case errors.Is(err, controller.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// ----- ports / alerts / config / ai handlers -----

func (s *Server) handlePorts(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	writeJSON(w, http.StatusOK, snap.PortBindings)
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	snap := s.store.Latest()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "no_data", "no snapshot yet")
		return
	}
	rows := append([]metrics.PortBinding(nil), snap.PortBindings...)
	if rawPID := strings.TrimSpace(r.URL.Query().Get("pid")); rawPID != "" {
		pid, err := strconv.ParseUint(rawPID, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_pid", "pid must be uint32")
			return
		}
		out := rows[:0]
		for _, row := range rows {
			if row.PID == uint32(pid) {
				out = append(out, row)
			}
		}
		rows = out
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.alerts.Active())
}

func (s *Server) handleAlertHistory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.alerts.History())
}

// handleAlertsClear wipes the active alert set without touching history.
// The UI exposes this as a single "Clear" button so the user can recover
// from a noisy burst without restarting the whole process.
func (s *Server) handleAlertsClear(w http.ResponseWriter, r *http.Request) {
	removed := s.alerts.ClearAll()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

func (s *Server) handleAlertDismiss(w http.ResponseWriter, r *http.Request) {
	key, ok := alertKeyFromRequest(w, r)
	if !ok {
		return
	}
	s.alerts.ClearByKey(key)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": key})
}

func (s *Server) handleAlertSnooze(w http.ResponseWriter, r *http.Request) {
	typeName := strings.TrimSpace(Param(r, "type"))
	if typeName == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "type required")
		return
	}
	var pid uint32
	if rawPID := Param(r, "pid"); rawPID != "" {
		n, err := strconv.ParseUint(rawPID, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "pid must be uint32")
			return
		}
		pid = uint32(n)
	}
	duration := 30 * time.Minute
	if raw := strings.TrimSpace(r.URL.Query().Get("duration")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_duration", "duration must be a positive Go duration like 30m")
			return
		}
		duration = parsed
	}
	until := time.Now().Add(duration)
	s.alerts.Snooze(typeName, pid, until)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"type":        typeName,
		"pid":         pid,
		"snoozed_for": duration.String(),
		"until":       until,
	})
}

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	current := *s.cfg
	s.mu.RUnlock()
	current.AI.APIKey = maskSecret(current.AI.APIKey)
	current.AI.ExtraHeaders = redactHeaderValues(current.AI.ExtraHeaders)
	current.Telegram.BotToken = maskSecret(current.Telegram.BotToken)
	writeJSON(w, http.StatusOK, &current)
}

func (s *Server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if s.advisor == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, s.advisor.Status())
}

func (s *Server) handleAIWatch(w http.ResponseWriter, r *http.Request) {
	if s.advisor == nil {
		writeJSON(w, http.StatusOK, ai.BackgroundState{})
		return
	}
	writeJSON(w, http.StatusOK, s.advisor.BackgroundState())
}

func (s *Server) handleAIAnalyze(w http.ResponseWriter, r *http.Request) {
	if s.advisor == nil || !s.advisor.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "ai_disabled", "AI advisor not configured")
		return
	}
	var body struct {
		Prompt string `json:"prompt"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	resp, err := s.advisor.Analyze(r.Context(), body.Prompt)
	if err != nil {
		log.Printf("ai analyze: %v", err)
		writeError(w, http.StatusBadGateway, "ai_error", publicAIErrorMessage(err))
		return
	}
	s.rememberAISuggestions(resp.Actions)
	writeJSON(w, http.StatusOK, map[string]any{
		"answer":  resp.Answer,
		"actions": resp.Actions,
	})
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if s.advisor == nil || !s.advisor.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "ai_disabled", "AI advisor not configured")
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, http.StatusBadRequest, "invalid_message", "message required")
		return
	}
	resp, err := s.advisor.Chat(r.Context(), body.Message)
	if err != nil {
		log.Printf("ai chat: %v", err)
		writeError(w, http.StatusBadGateway, "ai_error", publicAIErrorMessage(err))
		return
	}
	s.rememberAISuggestions(resp.Actions)
	writeJSON(w, http.StatusOK, map[string]any{
		"answer":  resp.Answer,
		"actions": resp.Actions,
		"cached":  resp.Cached,
	})
}

// ----- static UI handler -----

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticFS == nil {
		http.NotFound(w, r)
		return
	}
	upath := strings.TrimPrefix(r.URL.Path, "/")
	if upath == "" {
		upath = "index.html"
	}
	f, err := s.staticFS.Open(upath)
	if err != nil {
		// SPA fallback to index.html.
		f2, err2 := s.staticFS.Open("index.html")
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		defer f2.Close()
		serveIndexHTML(w, f2, s.csrfToken)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		http.NotFound(w, r)
		return
	}
	if strings.EqualFold(upath, "index.html") {
		serveIndexHTML(w, f, s.csrfToken)
		return
	}
	w.Header().Set("Content-Type", contentTypeFor(upath))
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("serve static %s: %v", upath, err)
	}
}

func serveIndexHTML(w http.ResponseWriter, r io.Reader, csrfToken string) {
	body, err := io.ReadAll(r)
	if err != nil {
		http.Error(w, "failed to read index", http.StatusInternalServerError)
		return
	}
	body = bytes.ReplaceAll(body, []byte("__WTM_CSRF_TOKEN__"), []byte(csrfToken))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(body); err != nil {
		log.Printf("serve index.html: %v", err)
	}
}

func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".map":
		return "application/json"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	}
	return "application/octet-stream"
}

func publicAIErrorMessage(err error) string {
	if err == nil {
		return "AI provider request failed"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate limit"):
		return "AI rate limit exceeded; try again later"
	case strings.Contains(msg, "disabled"):
		return "AI advisor not configured"
	default:
		return "AI provider request failed"
	}
}

func alertKeyFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	typeName := strings.TrimSpace(Param(r, "type"))
	if typeName == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "type required")
		return "", false
	}
	var pid uint32
	if rawPID := Param(r, "pid"); rawPID != "" {
		n, err := strconv.ParseUint(rawPID, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_param", "pid must be uint32")
			return "", false
		}
		pid = uint32(n)
	}
	key := typeName
	if pid > 0 {
		key += "/" + strconv.FormatUint(uint64(pid), 10)
	}
	return key, true
}
