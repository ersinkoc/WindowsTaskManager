//go:build windows

package telegram

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ersinkoc/WindowsTaskManager/internal/ai"
	"github.com/ersinkoc/WindowsTaskManager/internal/anomaly"
	"github.com/ersinkoc/WindowsTaskManager/internal/config"
	"github.com/ersinkoc/WindowsTaskManager/internal/event"
	"github.com/ersinkoc/WindowsTaskManager/internal/metrics"
	"github.com/ersinkoc/WindowsTaskManager/internal/storage"
)

type processController interface {
	Kill(pid uint32, confirm bool) error
	Suspend(pid uint32, confirm bool) error
	Resume(pid uint32) error
}

type aiAdvisor interface {
	Enabled() bool
	Analyze(ctx context.Context, userQuestion string) (*ai.AnalyzeResult, error)
	Chat(ctx context.Context, userMessage string) (*ai.AnalyzeResult, error)
}

type Bot struct {
	mu         sync.RWMutex
	cfg        *config.Config
	store      *storage.Store
	alerts     *anomaly.AlertStore
	controller processController
	advisor    aiAdvisor
	executeAI  func(ai.Suggestion) error
	httpClient *http.Client

	offset    int64
	lastToken string
	pending   map[string]pendingAction
	rootCtx   context.Context
}

type pendingAction struct {
	ChatID      int64
	Description string
	SuccessText string
	ExpiresAt   time.Time
	Run         func() error
}

const maxTelegramResponseBytes = 1 << 20

var newConfirmCodeFunc = newConfirmCode

func New(cfg *config.Config, store *storage.Store, alerts *anomaly.AlertStore, ctrl processController, advisor aiAdvisor, executeAI func(ai.Suggestion) error, emitter *event.Emitter) *Bot {
	b := &Bot{
		cfg:        cfg,
		store:      store,
		alerts:     alerts,
		controller: ctrl,
		advisor:    advisor,
		executeAI:  executeAI,
		httpClient: &http.Client{Timeout: 40 * time.Second},
		pending:    make(map[string]pendingAction),
		rootCtx:    context.Background(),
	}
	if emitter != nil {
		emitter.On(anomaly.EventAlertRaised, b.handleAlertRaised)
	}
	return b
}

func (b *Bot) SetConfig(cfg *config.Config) {
	b.mu.Lock()
	b.cfg = cfg
	b.mu.Unlock()
}

func (b *Bot) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	b.mu.Lock()
	b.rootCtx = ctx
	b.mu.Unlock()
	go b.loop(ctx)
}

func (b *Bot) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg := b.currentConfig()
		if cfg == nil || !cfg.Telegram.Enabled || cfg.Telegram.BotToken == "" || len(cfg.Telegram.AllowedChatIDs) == 0 {
			if !sleepContext(ctx, 5*time.Second) {
				return
			}
			continue
		}

		token := cfg.Telegram.BotToken
		if token != b.lastToken {
			b.offset = 0
			b.lastToken = token
		}

		updates, err := b.getUpdates(ctx, cfg)
		if err != nil {
			log.Printf("telegram: getUpdates: %v", err)
			if !sleepContext(ctx, 3*time.Second) {
				return
			}
			continue
		}
		for _, upd := range updates {
			b.offset = upd.UpdateID + 1
			if upd.Message == nil || strings.TrimSpace(upd.Message.Text) == "" {
				continue
			}
			if !isAllowedChat(cfg.Telegram.AllowedChatIDs, upd.Message.Chat.ID) {
				continue
			}
			b.handleMessage(ctx, cfg, upd.Message)
		}
	}
}

func (b *Bot) currentConfig() *config.Config {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cfg
}

func (b *Bot) backgroundContext() context.Context {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.rootCtx != nil {
		return b.rootCtx
	}
	return context.Background()
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

type updateResp struct {
	OK          bool       `json:"ok"`
	Result      []tgUpdate `json:"result"`
	Description string     `json:"description"`
	ErrorCode   int        `json:"error_code"`
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message,omitempty"`
}

type tgMessage struct {
	MessageID int64   `json:"message_id"`
	Text      string  `json:"text"`
	Chat      tgChat  `json:"chat"`
	From      *tgUser `json:"from,omitempty"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	Username string `json:"username"`
}

func (b *Bot) getUpdates(ctx context.Context, cfg *config.Config) ([]tgUpdate, error) {
	timeoutSec := int(cfg.Telegram.PollTimeout / time.Second)
	if timeoutSec < 1 {
		timeoutSec = 25
	}
	body := map[string]any{
		"offset":          b.offset,
		"timeout":         timeoutSec,
		"allowed_updates": []string{"message"},
	}
	var resp updateResp
	if err := b.apiCall(ctx, cfg, "getUpdates", body, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram %d: %s", resp.ErrorCode, resp.Description)
	}
	return resp.Result, nil
}

func (b *Bot) apiCall(ctx context.Context, cfg *config.Config, method string, body any, dst any) error {
	base := strings.TrimRight(cfg.Telegram.APIBaseURL, "/")
	url := fmt.Sprintf("%s/bot%s/%s", base, cfg.Telegram.BotToken, method)

	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxTelegramResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxTelegramResponseBytes {
		return fmt.Errorf("telegram response exceeds %d bytes", maxTelegramResponseBytes)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, cfg *config.Config, msg *tgMessage) {
	cmd, args := parseCommand(msg.Text)
	if cmd == "" {
		return
	}
	var reply string
	switch cmd {
	case "start", "help":
		reply = helpText()
	case "status":
		reply = b.statusText()
	case "topcpu":
		reply = b.topCPUText()
	case "alerts":
		reply = b.alertsText()
	case "ask", "ai":
		reply = b.aiChatText(ctx, cfg, msg.Chat.ID, strings.Join(args, " "))
	case "analyze":
		reply = b.aiAnalyzeText(ctx, cfg, msg.Chat.ID, strings.Join(args, " "))
	case "kill":
		reply = b.pidAction(cfg, msg.Chat.ID, args, func(pid uint32) error { return b.controller.Kill(pid, true) }, "kill", "killed", true)
	case "taskkill":
		reply = b.nameAction(cfg, msg.Chat.ID, args, func(pid uint32) error { return b.controller.Kill(pid, true) }, "kill", "killed", true)
	case "suspend":
		reply = b.pidAction(cfg, msg.Chat.ID, args, func(pid uint32) error { return b.controller.Suspend(pid, true) }, "suspend", "suspended", true)
	case "resume":
		reply = b.pidAction(cfg, msg.Chat.ID, args, b.controller.Resume, "resume", "resumed", false)
	case "killtop":
		reply = b.topProcessAction(cfg, msg.Chat.ID, func(pid uint32) error { return b.controller.Kill(pid, true) }, "kill", "killed", true)
	case "suspendtop":
		reply = b.topProcessAction(cfg, msg.Chat.ID, func(pid uint32) error { return b.controller.Suspend(pid, true) }, "suspend", "suspended", true)
	case "confirm":
		reply = b.confirmAction(args, msg.Chat.ID)
	case "cancel":
		reply = b.cancelAction(args, msg.Chat.ID)
	default:
		reply = "Unknown command. Send /help."
	}
	if err := b.sendMessage(ctx, cfg, msg.Chat.ID, reply); err != nil {
		log.Printf("telegram: send reply: %v", err)
	}
}

func parseCommand(text string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", nil
	}
	cmd := strings.TrimPrefix(fields[0], "/")
	if idx := strings.IndexByte(cmd, '@'); idx >= 0 {
		cmd = cmd[:idx]
	}
	return strings.ToLower(cmd), fields[1:]
}

func helpText() string {
	return strings.Join([]string{
		"WTM rescue bot commands:",
		"/status - CPU, memory, and top processes",
		"/topcpu - highest CPU processes",
		"/alerts - active anomaly alerts",
		"/ask <question> - chat with the AI advisor",
		"/analyze <question> - analyze current state and queue AI actions",
		"/kill <pid> - kill a process by PID",
		"/taskkill [/F] [/IM] <name> - kill by process name (like Windows taskkill)",
		"/suspend <pid> - suspend a process",
		"/resume <pid> - resume a process",
		"/killtop - kill the highest CPU non-protected process",
		"/suspendtop - suspend the highest CPU non-protected process",
		"/confirm <code> - confirm a pending kill/suspend action",
		"/cancel <code> - cancel a pending kill/suspend action",
		"",
		"Name matching: /taskkill notepad matches notepad.exe (extension optional)",
		"Force flag: /taskkill /F notepad skips confirmation",
	}, "\n")
}

func (b *Bot) statusText() string {
	snap := b.store.Latest()
	if snap == nil {
		return "No snapshot yet."
	}
	top := topProcessesByCPU(snap.Processes, 3)
	lines := []string{
		fmt.Sprintf("CPU %.1f%% (%d cores)", snap.CPU.TotalPercent, snap.CPU.NumLogical),
		fmt.Sprintf("Memory %.1f%% (%s / %s)", snap.Memory.UsedPercent, formatBytes(snap.Memory.UsedPhys), formatBytes(snap.Memory.TotalPhys)),
		fmt.Sprintf("Network ↓ %s/s ↑ %s/s", formatBytes(snap.Network.TotalDownBPS), formatBytes(snap.Network.TotalUpBPS)),
		"Top CPU:",
	}
	for _, p := range top {
		lines = append(lines, fmt.Sprintf("- %s PID %d CPU %.1f%% MEM %s", p.Name, p.PID, p.CPUPercent, formatBytes(p.WorkingSet)))
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) topCPUText() string {
	snap := b.store.Latest()
	if snap == nil {
		return "No snapshot yet."
	}
	top := topProcessesByCPU(snap.Processes, 8)
	if len(top) == 0 {
		return "No processes found."
	}
	lines := []string{"Top CPU processes:"}
	for _, p := range top {
		lines = append(lines, fmt.Sprintf("- %s PID %d CPU %.1f%% MEM %s Threads %d", p.Name, p.PID, p.CPUPercent, formatBytes(p.WorkingSet), p.ThreadCount))
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) alertsText() string {
	items := b.alerts.Active()
	if len(items) == 0 {
		return "No active alerts."
	}
	if len(items) > 8 {
		items = items[:8]
	}
	lines := []string{"Active alerts:"}
	for _, a := range items {
		target := ""
		if a.ProcessName != "" {
			target = " " + a.ProcessName
		} else if a.PID != 0 {
			target = " PID " + strconv.FormatUint(uint64(a.PID), 10)
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s%s", strings.ToUpper(string(a.Severity)), a.Title, target))
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) pidAction(cfg *config.Config, chatID int64, args []string, fn func(uint32) error, actionLabel, verb string, requiresConfirm bool) string {
	if len(args) < 1 {
		return "PID required."
	}
	pid64, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		return "PID must be a positive integer."
	}
	pid := uint32(pid64)
	if requiresConfirm && cfg != nil && cfg.Telegram.RequireConfirm {
		description, success := b.pidActionTexts(pid, actionLabel, verb)
		return b.queueConfirmation(chatID, cfg.Telegram.ConfirmTTL, description, success, func() error {
			return fn(pid)
		})
	}
	if err := fn(pid); err != nil {
		return fmt.Sprintf("Action failed: %v", err)
	}
	_, success := b.pidActionTexts(pid, actionLabel, verb)
	return success
}

func (b *Bot) nameAction(cfg *config.Config, chatID int64, args []string, fn func(uint32) error, actionLabel, verb string, requiresConfirm bool) string {
	name, force := parseTaskkillArgs(args)
	if name == "" {
		return "Process name required. Usage: /taskkill [/F] [/IM] <name>"
	}

	pid, failText := b.findProcessByName(cfg, name)
	if pid == 0 {
		return failText
	}

	target := describeProcess("", pid)
	if snap := b.store.Latest(); snap != nil {
		target = describeProcess(snap.ProcessName(pid), pid)
	}

	if requiresConfirm && !force && cfg != nil && cfg.Telegram.RequireConfirm {
		description := fmt.Sprintf("%s %s", actionLabel, target)
		success := fmt.Sprintf("%s %s", titleWord(verb), target)
		return b.queueConfirmation(chatID, cfg.Telegram.ConfirmTTL, description, success, func() error {
			return fn(pid)
		})
	}
	if err := fn(pid); err != nil {
		return fmt.Sprintf("Action failed: %v", err)
	}
	return fmt.Sprintf("%s %s", titleWord(verb), target)
}

func parseTaskkillArgs(args []string) (name string, force bool) {
	if len(args) == 0 {
		return "", false
	}
	// Handle Windows-style /F flag anywhere in args
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		upper := strings.ToUpper(arg)
		if upper == "/F" || upper == "-F" || upper == "--FORCE" {
			force = true
			continue
		}
		if upper == "/IM" && len(args) > 1 {
			// /IM next arg is the name
			continue
		}
		filtered = append(filtered, arg)
	}
	if len(filtered) == 0 {
		return "", force
	}
	// Last non-flag argument is the process name
	return filtered[len(filtered)-1], force
}

func (b *Bot) findProcessByName(cfg *config.Config, name string) (uint32, string) {
	snap := b.store.Latest()
	if snap == nil {
		return 0, "No snapshot yet."
	}
	nameLower := strings.ToLower(name)
	// Strip .exe suffix for matching
	if strings.HasSuffix(nameLower, ".exe") {
		nameLower = strings.TrimSuffix(nameLower, ".exe")
	}
	var candidates []metrics.ProcessInfo
	for _, p := range snap.Processes {
		pNameLower := strings.ToLower(p.Name)
		if strings.HasSuffix(pNameLower, ".exe") {
			pNameLower = strings.TrimSuffix(pNameLower, ".exe")
		}
		if pNameLower == nameLower {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return 0, fmt.Sprintf("No process found matching %q.", name)
	}
	// Return highest CPU candidate
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CPUPercent > candidates[j].CPUPercent
	})
	best := candidates[0]
	if best.PID == 0 || best.IsCritical || isProtectedProcess(cfg, best.Name) {
		return 0, fmt.Sprintf("Process %q is protected or critical; use /kill %d directly to force.", name, best.PID)
	}
	return best.PID, ""
}

func (b *Bot) topProcessAction(cfg *config.Config, chatID int64, fn func(uint32) error, actionLabel, verb string, requiresConfirm bool) string {
	candidate, failText := b.selectTopProcess(cfg)
	if candidate == nil {
		return failText
	}
	descTarget := describeProcess(candidate.Name, candidate.PID)
	success := fmt.Sprintf("%s %s", titleWord(verb), descTarget)
	if requiresConfirm && cfg != nil && cfg.Telegram.RequireConfirm {
		description := fmt.Sprintf("%s %s (CPU %.1f%%)", actionLabel, descTarget, candidate.CPUPercent)
		return b.queueConfirmation(chatID, cfg.Telegram.ConfirmTTL, description, success, func() error {
			return fn(candidate.PID)
		})
	}
	if err := fn(candidate.PID); err != nil {
		return fmt.Sprintf("Action failed: %v", err)
	}
	return fmt.Sprintf("%s (CPU %.1f%%)", success, candidate.CPUPercent)
}

func (b *Bot) confirmAction(args []string, chatID int64) string {
	code := normalizeConfirmCode(args)
	if code == "" {
		return "Confirmation code required."
	}

	b.mu.Lock()
	now := time.Now()
	b.cleanupPendingLocked(now)
	action, ok := b.pending[code]
	if !ok {
		b.mu.Unlock()
		return "Confirmation code not found or expired."
	}
	if action.ChatID != chatID {
		b.mu.Unlock()
		return "Confirmation code belongs to another chat."
	}
	delete(b.pending, code)
	b.mu.Unlock()

	if err := action.Run(); err != nil {
		return fmt.Sprintf("Action failed: %v", err)
	}
	return action.SuccessText
}

func (b *Bot) cancelAction(args []string, chatID int64) string {
	code := normalizeConfirmCode(args)
	if code == "" {
		return "Confirmation code required."
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanupPendingLocked(time.Now())
	action, ok := b.pending[code]
	if !ok {
		return "Confirmation code not found or expired."
	}
	if action.ChatID != chatID {
		return "Confirmation code belongs to another chat."
	}
	delete(b.pending, code)
	return fmt.Sprintf("Cancelled %s", action.Description)
}

func (b *Bot) queueConfirmation(chatID int64, ttl time.Duration, description, success string, run func() error) string {
	code, err := b.storePendingAction(chatID, ttl, description, success, run)
	if err != nil {
		log.Printf("telegram: confirm code generation failed: %v", err)
		return "Failed to create confirmation code; action was not executed."
	}
	return fmt.Sprintf("Pending %s. Confirm with /confirm %s within %s, or /cancel %s.", description, code, formatConfirmTTL(ttl), code)
}

func (b *Bot) storePendingAction(chatID int64, ttl time.Duration, description, success string, run func() error) (string, error) {
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	code, err := newConfirmCodeFunc()
	if err != nil {
		return "", err
	}

	expiresAt := time.Now().Add(ttl)
	b.mu.Lock()
	b.cleanupPendingLocked(time.Now())
	b.pending[code] = pendingAction{
		ChatID:      chatID,
		Description: description,
		SuccessText: success,
		ExpiresAt:   expiresAt,
		Run:         run,
	}
	b.mu.Unlock()
	return code, nil
}

func (b *Bot) cleanupPendingLocked(now time.Time) {
	for code, action := range b.pending {
		if !action.ExpiresAt.After(now) {
			delete(b.pending, code)
		}
	}
}

func (b *Bot) pidActionTexts(pid uint32, actionLabel, verb string) (string, string) {
	target := describeProcess("", pid)
	if snap := b.store.Latest(); snap != nil {
		target = describeProcess(snap.ProcessName(pid), pid)
	}
	return fmt.Sprintf("%s %s", actionLabel, target), fmt.Sprintf("%s %s", titleWord(verb), target)
}

func (b *Bot) selectTopProcess(cfg *config.Config) (*metrics.ProcessInfo, string) {
	snap := b.store.Latest()
	if snap == nil {
		return nil, "No snapshot yet."
	}
	for _, p := range topProcessesByCPU(snap.Processes, len(snap.Processes)) {
		if p.PID == 0 || p.IsCritical || isProtectedProcess(cfg, p.Name) {
			continue
		}
		candidate := p
		return &candidate, ""
	}
	return nil, "No safe top process found."
}

func topProcessesByCPU(procs []metrics.ProcessInfo, limit int) []metrics.ProcessInfo {
	out := append([]metrics.ProcessInfo(nil), procs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUPercent == out[j].CPUPercent {
			return out[i].PID < out[j].PID
		}
		return out[i].CPUPercent > out[j].CPUPercent
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}

func (b *Bot) sendMessage(ctx context.Context, cfg *config.Config, chatID int64, text string) error {
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
	}
	req := map[string]any{
		"chat_id": chatID,
		"text":    truncate(text, 4000),
	}
	if err := b.apiCall(ctx, cfg, "sendMessage", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram %d: %s", resp.ErrorCode, resp.Description)
	}
	return nil
}

func (b *Bot) handleAlertRaised(data any) {
	alert, ok := data.(anomaly.Alert)
	if !ok {
		return
	}
	cfg := b.currentConfig()
	if !shouldNotifyTelegramAlert(cfg, alert) {
		return
	}
	text := fmt.Sprintf("[CRITICAL] %s\n%s", alert.Title, alert.Description)
	if alert.ProcessName != "" {
		text += fmt.Sprintf("\nProcess: %s", alert.ProcessName)
	}
	if alert.PID != 0 {
		text += fmt.Sprintf("\nPID: %d", alert.PID)
	}
	ctx, cancel := context.WithTimeout(b.backgroundContext(), 10*time.Second)
	defer cancel()
	for _, chatID := range cfg.Telegram.AllowedChatIDs {
		if err := b.sendMessage(ctx, cfg, chatID, text); err != nil {
			log.Printf("telegram: notify chat %d: %v", chatID, err)
		}
	}
}

func (b *Bot) aiChatText(ctx context.Context, cfg *config.Config, chatID int64, prompt string) string {
	return b.aiReply(ctx, cfg, chatID, prompt, true)
}

func (b *Bot) aiAnalyzeText(ctx context.Context, cfg *config.Config, chatID int64, prompt string) string {
	return b.aiReply(ctx, cfg, chatID, prompt, false)
}

func (b *Bot) aiReply(ctx context.Context, cfg *config.Config, chatID int64, prompt string, chatMode bool) string {
	if strings.TrimSpace(prompt) == "" {
		if chatMode {
			return "Prompt required. Example: /ask what should I investigate first?"
		}
		return "Prompt required. Example: /analyze summarize the biggest risks right now"
	}
	if b.advisor == nil || !b.advisor.Enabled() {
		return "AI advisor not configured."
	}

	var (
		resp *ai.AnalyzeResult
		err  error
	)
	if chatMode {
		resp, err = b.advisor.Chat(ctx, prompt)
	} else {
		resp, err = b.advisor.Analyze(ctx, prompt)
	}
	if err != nil {
		return fmt.Sprintf("AI error: %v", err)
	}

	lines := []string{strings.TrimSpace(resp.Answer)}
	if len(resp.Actions) > 0 {
		lines = append(lines, "", "Queued AI actions:")
		for _, suggestion := range resp.Actions[:min(len(resp.Actions), 4)] {
			lines = append(lines, "- "+b.queueAISuggestion(chatID, cfg.Telegram.ConfirmTTL, suggestion))
		}
		if len(resp.Actions) > 4 {
			lines = append(lines, fmt.Sprintf("- %d more action(s) omitted", len(resp.Actions)-4))
		}
	}
	return truncate(strings.Join(lines, "\n"), 4000)
}

func (b *Bot) queueAISuggestion(chatID int64, ttl time.Duration, suggestion ai.Suggestion) string {
	description, success := describeAISuggestion(suggestion)
	if b.executeAI == nil {
		return description + " (execution unavailable)"
	}
	code, err := b.storePendingAction(chatID, ttl, description, success, func() error {
		return b.executeAI(suggestion)
	})
	if err != nil {
		return description + " (failed to queue confirmation)"
	}
	return fmt.Sprintf("%s -> /confirm %s", description, code)
}

func shouldNotifyTelegramAlert(cfg *config.Config, alert anomaly.Alert) bool {
	if cfg == nil || !cfg.Telegram.Enabled || !cfg.Telegram.NotifyOnCritical || cfg.Telegram.BotToken == "" {
		return false
	}
	if alert.Severity != anomaly.SeverityCritical {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Telegram.NotificationMode)) {
	case "all_critical":
		return true
	case "", "high_value":
		return isHighValueTelegramAlert(cfg, alert)
	default:
		return isHighValueTelegramAlert(cfg, alert)
	}
}

func isHighValueTelegramAlert(cfg *config.Config, alert anomaly.Alert) bool {
	if strings.HasPrefix(alert.Type, "rule:") {
		return alert.Action == "kill" || alert.Action == "suspend" || matchesTelegramTypeAllowlist(cfg.Telegram.NotificationTypes, "rule:*")
	}
	switch alert.Type {
	case "runaway_cpu", "memory_leak", "port_conflict", "new_process", "network_anomaly", "network_anomaly_system":
		return matchesTelegramTypeAllowlist(cfg.Telegram.NotificationTypes, alert.Type)
	default:
		return false
	}
}

func matchesTelegramTypeAllowlist(allowlist []string, alertType string) bool {
	for _, item := range allowlist {
		token := strings.ToLower(strings.TrimSpace(item))
		if token == "" {
			continue
		}
		if token == "*" || token == strings.ToLower(alertType) {
			return true
		}
		if strings.HasSuffix(token, "*") {
			prefix := strings.TrimSuffix(token, "*")
			if strings.HasPrefix(strings.ToLower(alertType), prefix) {
				return true
			}
		}
	}
	return false
}

func describeAISuggestion(suggestion ai.Suggestion) (string, string) {
	switch strings.ToLower(strings.TrimSpace(suggestion.Type)) {
	case "kill":
		target := describeProcess(suggestion.Name, suggestion.PID)
		return "AI: kill " + target, "AI action executed: killed " + target
	case "suspend":
		target := describeProcess(suggestion.Name, suggestion.PID)
		return "AI: suspend " + target, "AI action executed: suspended " + target
	case "protect":
		name := strings.TrimSpace(suggestion.Name)
		return "AI: protect " + name, "AI action executed: protected " + name
	case "ignore":
		name := strings.TrimSpace(suggestion.Name)
		return "AI: ignore " + name, "AI action executed: ignored " + name
	case "add_rule":
		ruleName := ""
		if suggestion.Rule != nil {
			ruleName = strings.TrimSpace(suggestion.Rule.Name)
		}
		return "AI: add rule " + ruleName, "AI action executed: rule added " + ruleName
	default:
		return "AI action", "AI action executed"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isAllowedChat(allowed []int64, chatID int64) bool {
	for _, id := range allowed {
		if id == chatID {
			return true
		}
	}
	return false
}

func isProtectedProcess(cfg *config.Config, name string) bool {
	if cfg == nil || name == "" {
		return false
	}
	for _, item := range cfg.Controller.ProtectedProcesses {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func normalizeConfirmCode(args []string) string {
	if len(args) < 1 {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(args[0]))
}

func newConfirmCode() (string, error) {
	var raw [5]byte
	if _, err := randReadFunc(raw[:]); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]), nil
}

// randReadFunc is a hook for tests to simulate rand.Read failures.
var randReadFunc = rand.Read

func describeProcess(name string, pid uint32) string {
	if name != "" {
		return fmt.Sprintf("%s (PID %d)", name, pid)
	}
	return fmt.Sprintf("PID %d", pid)
}

func formatConfirmTTL(ttl time.Duration) string {
	if ttl < time.Minute {
		return fmt.Sprintf("%ds", int(ttl/time.Second))
	}
	if ttl%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(ttl/time.Minute))
	}
	return ttl.Round(time.Second).String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func titleWord(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := uint64(unit), 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	suffixes := []string{"K", "M", "G", "T", "P"}
	return fmt.Sprintf("%.1f%s", float64(n)/float64(div), suffixes[exp])
}
