package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// modelsDevURL is the upstream catalog. It's a single static-ish JSON
// document that we proxy + cache so the dashboard doesn't have to deal
// with CORS or constantly hammer it.
//
// It is a var (not a const) so tests can substitute an invalid URL to
// exercise the http.NewRequest error branch in refresh.
var modelsDevURL = "https://models.dev/api.json"

const maxModelsCatalogBytes = 2 << 20

// modelInfo is the trimmed shape we hand to the dashboard. Anything we don't
// need (cost matrix, modalities, knowledge cutoff) is dropped.
type modelInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ProviderID string `json:"provider_id"`
	Provider   string `json:"provider"`
	Endpoint   string `json:"endpoint"`
	Format     string `json:"format"` // "anthropic" or "openai"
	Context    int    `json:"context,omitempty"`
	Output     int    `json:"output,omitempty"`
}

// modelsCache memoizes the result for ttl. Refresh runs lazily on the first
// request after expiry. We never block waiting for refresh: the previous
// payload is still served until the new one is ready.
type modelsCache struct {
	mu        sync.Mutex
	data      []modelInfo
	loadedAt  time.Time
	ttl       time.Duration
	inFlight  bool
	lastError string
	client    *http.Client
}

type modelsSnapshot struct {
	data     []modelInfo
	loadedAt time.Time
	lastErr  string
}

var sharedModelsCache = &modelsCache{
	ttl:    30 * time.Minute,
	client: &http.Client{Timeout: 15 * time.Second},
}

func (c *modelsCache) get() modelsSnapshot {
	c.mu.Lock()
	fresh := time.Since(c.loadedAt) < c.ttl && len(c.data) > 0
	stale := !fresh
	snap := modelsSnapshot{
		data:     append([]modelInfo(nil), c.data...),
		loadedAt: c.loadedAt,
		lastErr:  c.lastError,
	}
	if stale && !c.inFlight {
		c.inFlight = true
		go c.refresh()
	}
	c.mu.Unlock()
	return snap
}

func (c *modelsCache) refresh() {
	defer func() {
		c.mu.Lock()
		c.inFlight = false
		c.mu.Unlock()
	}()

	req, err := http.NewRequest(http.MethodGet, modelsDevURL, nil)
	if err != nil {
		c.setError(err)
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WindowsTaskManager/1.0 (+models.dev sync)")
	resp, err := c.client.Do(req)
	if err != nil {
		c.setError(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		c.setError(fmt.Errorf("models.dev: status %d", resp.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsCatalogBytes+1))
	if err != nil {
		c.setError(err)
		return
	}
	if len(body) > maxModelsCatalogBytes {
		c.setError(fmt.Errorf("models.dev response exceeds %d bytes", maxModelsCatalogBytes))
		return
	}

	parsed, err := parseModelsDev(body)
	if err != nil {
		c.setError(err)
		return
	}

	c.mu.Lock()
	c.data = parsed
	c.loadedAt = time.Now()
	c.lastError = ""
	c.mu.Unlock()
}

func (c *modelsCache) setError(err error) {
	c.mu.Lock()
	c.lastError = err.Error()
	c.mu.Unlock()
}

// parseModelsDev decodes the models.dev document and flattens it into a sorted
// list of modelInfo. Provider format is inferred from the npm SDK package the
// upstream lists, since there's no explicit "anthropic vs openai" field.
func parseModelsDev(body []byte) ([]modelInfo, error) {
	type rawModel struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Limit struct {
			Context int `json:"context"`
			Output  int `json:"output"`
		} `json:"limit"`
	}
	type rawProvider struct {
		ID     string              `json:"id"`
		Name   string              `json:"name"`
		API    string              `json:"api"`
		NPM    string              `json:"npm"`
		Models map[string]rawModel `json:"models"`
	}

	var doc map[string]rawProvider
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	out := make([]modelInfo, 0, 256)
	for pid, p := range doc {
		format := inferFormat(p.NPM, p.API, pid)
		endpoint := normalizedEndpoint(p.API, format)
		for mid, m := range p.Models {
			name := m.Name
			if name == "" {
				name = mid
			}
			out = append(out, modelInfo{
				ID:         mid,
				Name:       name,
				ProviderID: pid,
				Provider:   p.Name,
				Endpoint:   endpoint,
				Format:     format,
				Context:    m.Limit.Context,
				Output:     m.Limit.Output,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return strings.ToLower(out[i].Provider) < strings.ToLower(out[j].Provider)
		}
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out, nil
}

// inferFormat picks the closest provider type WTM speaks based on the
// upstream's SDK hint. Anthropic SDK → Anthropic Messages format; everything
// else (openai, openai-compatible, mistral, google, ...) routes through the
// OpenAI chat-completions handler since that's the lingua franca.
func inferFormat(npm, api, providerID string) string {
	low := strings.ToLower(npm + " " + api + " " + providerID)
	switch {
	case strings.Contains(low, "anthropic"):
		return "anthropic"
	case strings.Contains(low, "z.ai") || strings.Contains(low, "zhipu"):
		return "anthropic"
	default:
		return "openai"
	}
}

// normalizedEndpoint converts a provider's documented base URL into the
// fully-qualified path WTM needs.
func normalizedEndpoint(api, format string) string {
	api = strings.TrimRight(api, "/")
	if api == "" {
		return ""
	}
	switch format {
	case "anthropic":
		if strings.HasSuffix(api, "/v1/messages") || strings.HasSuffix(api, "/messages") {
			return api
		}
		// Anthropic-compatible upstreams sometimes publish ".../anthropic" or
		// ".../v1" — append /messages either way.
		if strings.HasSuffix(api, "/anthropic") {
			return api + "/v1/messages"
		}
		if strings.HasSuffix(api, "/v1") {
			return api + "/messages"
		}
		return api + "/v1/messages"
	case "openai":
		if strings.HasSuffix(api, "/chat/completions") {
			return api
		}
		if strings.HasSuffix(api, "/v1") {
			return api + "/chat/completions"
		}
		return api + "/v1/chat/completions"
	}
	return api
}

func (s *Server) handleAIModels(w http.ResponseWriter, r *http.Request) {
	snap := sharedModelsCache.get()
	data := snap.data
	q := strings.ToLower(r.URL.Query().Get("provider"))
	if q != "" {
		filtered := make([]modelInfo, 0, len(data)/4)
		for _, m := range data {
			if strings.Contains(strings.ToLower(m.ProviderID), q) ||
				strings.Contains(strings.ToLower(m.Provider), q) ||
				strings.Contains(strings.ToLower(m.Format), q) {
				filtered = append(filtered, m)
			}
		}
		data = filtered
	}
	resp := map[string]any{
		"models":   data,
		"count":    len(data),
		"source":   modelsDevURL,
		"updated":  snap.loadedAt.Format(time.RFC3339),
		"endpoint": s.effectiveAIEndpoint(),
	}
	if snap.lastErr != "" {
		resp["error"] = snap.lastErr
	}
	if len(data) == 0 && snap.lastErr == "" {
		w.Header().Set("Retry-After", "2")
	}
	writeJSON(w, http.StatusOK, resp)
}

// effectiveAIEndpoint returns the AI endpoint URL actually in use.
func (s *Server) effectiveAIEndpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.cfg == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(s.cfg.AI.Provider)) {
	case "anthropic":
		if s.cfg.AI.Endpoint != "" {
			return strings.TrimRight(s.cfg.AI.Endpoint, "/") + "/v1/messages"
		}
		return "https://api.anthropic.com/v1/messages"
	default:
		if s.cfg.AI.Endpoint != "" {
			ep := strings.TrimRight(s.cfg.AI.Endpoint, "/")
			if strings.HasSuffix(ep, "/chat/completions") {
				return ep
			}
			return ep + "/v1/chat/completions"
		}
		return "https://api.openai.com/v1/chat/completions"
	}
}
