// Package proxy is the inference proxy every agent container talks to instead of the provider.
//
// Each side of a comparison gets a session with a random token, which the container uses as
// its API key and the proxy swaps for the real one. Requests are forwarded unchanged and
// responses are copied back unchanged (streaming included) while the proxy reads the token
// usage from them. That gives per-side tokens and cost in interactive mode too, keeps API keys
// out of the containers, and lets token and cost limits be enforced.
//
// URLs: http://api:4701/<provider>/<provider path>, e.g. /openai/v1/responses.
package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"ai-compare/backend/internal/catalog"
)

// Provider is an upstream the proxy can forward to.
type Provider struct {
	Name    string
	BaseURL string // origin without the API version, e.g. https://api.openai.com
	APIKey  string
	// AuthHeader is "Authorization" (Bearer) or "x-api-key".
	AuthHeader string
}

func (p Provider) setAuth(h http.Header) {
	h.Del("Authorization")
	h.Del("x-api-key")
	if p.APIKey == "" {
		return // e.g. a local server
	}
	if p.AuthHeader == "x-api-key" {
		h.Set("x-api-key", p.APIKey)
	} else {
		h.Set("Authorization", "Bearer "+p.APIKey)
	}
}

type Limits struct {
	MaxTokens  int64   `json:"maxTokens,omitempty"`  // 0 = no limit
	MaxCostUSD float64 `json:"maxCostUsd,omitempty"` // 0 = no limit
}

type Request struct {
	At     time.Time `json:"at"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	// Model is the model named in the request body, when there is one.
	Model    string   `json:"model,omitempty"`
	Status   int      `json:"status"`
	Streamed bool     `json:"streamed"`
	Duration float64  `json:"durationSec"`
	Usage    Usage    `json:"usage"`
	CostUSD  *float64 `json:"costUsd"`
	Error    string   `json:"error,omitempty"`
	// Cancelled is true when the client stopped reading the response before it ended.
	Cancelled bool `json:"cancelled,omitempty"`
}

type Session struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Price    *catalog.Price `json:"price"`
	Long     *catalog.LongContext
	Limits   Limits `json:"limits"`

	mu        sync.Mutex
	usage     Usage
	cost      float64
	costKnown bool
	requests  []Request
	limitHit  string

	firstRequest []byte
}

// Snapshot is what the API returns about a session.
type Snapshot struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	Limits     Limits    `json:"limits"`
	Usage      Usage     `json:"usage"`
	CostUSD    *float64  `json:"costUsd"`
	LimitHit   string    `json:"limitHit,omitempty"`
	Requests   []Request `json:"requests"`
	RequestCnt int       `json:"requestCount"`
}

func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{ID: s.ID, Provider: s.Provider, Model: s.Model, Limits: s.Limits, Usage: s.usage, LimitHit: s.limitHit,
		Requests: append([]Request(nil), s.requests...), RequestCnt: len(s.requests)}
	if s.costKnown {
		c := s.cost
		snap.CostUSD = &c
	}
	return snap
}

func (s *Session) record(r Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usage.add(r.Usage)
	if r.CostUSD != nil {
		s.cost += *r.CostUSD
		s.costKnown = true
	}
	s.requests = append(s.requests, r)
	if s.Limits.MaxTokens > 0 && s.usage.Total() >= s.Limits.MaxTokens {
		s.limitHit = "token limit"
	}
	if s.Limits.MaxCostUSD > 0 && s.cost >= s.Limits.MaxCostUSD {
		s.limitHit = "cost limit"
	}
}

func (s *Session) limitReached() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limitHit
}

// Cost prices one request with the session's snapshot, applying the long-context tier when the
// prompt exceeds it. nil when the model has no price.
func Cost(u Usage, price *catalog.Price, long *catalog.LongContext) *float64 {
	if price == nil || !u.Reported {
		return nil
	}
	p := *price
	if long != nil && long.AboveTokens > 0 && u.PromptTokens() > int64(long.AboveTokens) {
		p = long.Price
	}
	rate := func(r *float64, fallback float64) float64 {
		if r == nil {
			return fallback
		}
		return *r
	}
	// No cache-read price means cached input is billed as normal input; no cache-write price means it is not charged.
	c := (float64(u.Input)*p.Input + float64(u.CacheRead)*rate(p.CacheRead, p.Input) +
		float64(u.CacheWrite)*rate(p.CacheWrite, 0) + float64(u.Output)*p.Output) / 1_000_000
	return &c
}

type Proxy struct {
	providers map[string]Provider
	log       *slog.Logger
	client    *http.Transport

	mu       sync.RWMutex
	sessions map[string]*Session // by token
}

func New(providers []Provider, log *slog.Logger) *Proxy {
	p := &Proxy{providers: map[string]Provider{}, log: log, sessions: map[string]*Session{},
		client: http.DefaultTransport.(*http.Transport).Clone()}
	for _, pr := range providers {
		p.providers[pr.Name] = pr
	}
	return p
}

// NewSession registers a side and returns its token.
func (p *Proxy) NewSession(id, provider, model string, price *catalog.Price, long *catalog.LongContext, limits Limits) (string, *Session, error) {
	if _, ok := p.providers[provider]; !ok {
		return "", nil, fmt.Errorf("unknown provider %q", provider)
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token := "aic_" + hex.EncodeToString(b)
	s := &Session{ID: id, Provider: provider, Model: model, Price: price, Long: long, Limits: limits}
	p.mu.Lock()
	p.sessions[token] = s
	p.mu.Unlock()
	return token, s, nil
}

// RestoreSession registers a side again with the token it already has, keeping what was
// recorded before. It lets a container that kept running while api restarted carry on.
func (p *Proxy) RestoreSession(token string, snap Snapshot, price *catalog.Price, long *catalog.LongContext) (*Session, error) {
	if _, ok := p.providers[snap.Provider]; !ok {
		return nil, fmt.Errorf("unknown provider %q", snap.Provider)
	}
	if !strings.HasPrefix(token, "aic_") {
		return nil, fmt.Errorf("invalid session token")
	}
	s := &Session{ID: snap.ID, Provider: snap.Provider, Model: snap.Model, Price: price, Long: long, Limits: snap.Limits,
		usage: snap.Usage, requests: append([]Request(nil), snap.Requests...), limitHit: snap.LimitHit}
	if snap.CostUSD != nil {
		s.cost, s.costKnown = *snap.CostUSD, true
	}
	p.mu.Lock()
	p.sessions[token] = s
	p.mu.Unlock()
	return s, nil
}

// EndSession revokes a token.
func (p *Proxy) EndSession(token string) {
	p.mu.Lock()
	delete(p.sessions, token)
	p.mu.Unlock()
}

// Session finds a session by its id (not its token).
func (p *Proxy) Session(id string) *Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func tokenFrom(r *http.Request) string {
	if t := r.Header.Get("x-api-key"); t != "" {
		return t
	}
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
}

// apiError answers in the shape both OpenAI and Anthropic clients print.
func apiError(w http.ResponseWriter, status int, kind, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": kind, "code": kind, "message": "ai-compare proxy: " + msg},
	})
}

// peekBody reads a JSON request body and its "model" field, and puts the body back untouched.
func peekBody(r *http.Request) (string, []byte) {
	if r.Body == nil || !strings.Contains(r.Header.Get("Content-Type"), "json") {
		return "", nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return "", nil
	}
	var m struct {
		Model string `json:"model"`
	}
	json.Unmarshal(body, &m)
	return m.Model, body
}

const maxFirstRequest = 4 << 20

func (s *Session) keepFirst(body []byte) {
	if len(body) == 0 || len(body) > maxFirstRequest || !bytes.Contains(body, []byte(`"tools"`)) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firstRequest == nil {
		s.firstRequest = body
	}
}

func (s *Session) FirstRequest() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.firstRequest
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.Write([]byte("ok\n"))
		return
	}
	name, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	provider, ok := p.providers[name]
	if !ok {
		apiError(w, http.StatusNotFound, "not_found", fmt.Sprintf("unknown provider %q; use /openai, /anthropic or /local", name))
		return
	}

	p.mu.RLock()
	session := p.sessions[tokenFrom(r)]
	p.mu.RUnlock()
	if session == nil {
		apiError(w, http.StatusUnauthorized, "invalid_api_key", "unknown or expired session token")
		return
	}
	if session.Provider != name {
		apiError(w, http.StatusForbidden, "wrong_provider", fmt.Sprintf("this session is for %s, not %s", session.Provider, name))
		return
	}
	if hit := session.limitReached(); hit != "" {
		// 403 rather than 429, so CLIs stop instead of retrying.
		apiError(w, http.StatusForbidden, "limit_reached", "the "+hit+" for this side has been reached")
		return
	}

	target, err := url.Parse(provider.BaseURL)
	if err != nil {
		apiError(w, http.StatusBadGateway, "bad_upstream", "invalid base URL for "+name)
		return
	}

	start := time.Now()
	model, body := peekBody(r)
	session.keepFirst(body)
	req := Request{At: start.UTC(), Method: r.Method, Path: "/" + rest, Model: model}
	rp := &httputil.ReverseProxy{
		Transport:     p.client,
		FlushInterval: -1, // stream every chunk as soon as it arrives
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.URL.Path = strings.TrimSuffix(target.Path, "/") + "/" + rest
			pr.Out.URL.RawPath = ""
			pr.Out.Host = target.Host
			provider.setAuth(pr.Out.Header)
			// Plain bodies, so usage can be read without decompressing.
			pr.Out.Header.Del("Accept-Encoding")
		},
		ModifyResponse: func(res *http.Response) error {
			req.Status = res.StatusCode
			req.Streamed = strings.Contains(res.Header.Get("Content-Type"), "text/event-stream")
			res.Body = newMeter(res.Body, res.Header.Get("Content-Type"), func(u Usage, apiError string, readErr error) {
				req.Duration = time.Since(start).Seconds()
				req.Usage = u
				req.CostUSD = Cost(u, session.Price, session.Long)
				req.Error = apiError
				switch {
				case req.Error != "" || readErr == nil || errors.Is(readErr, http.ErrBodyReadAfterClose):
				case errors.Is(readErr, context.Canceled):
					// The CLI stopped reading (e.g. opencode drops its title request when it ends):
					// not a provider failure.
					req.Cancelled = true
				default:
					req.Error = readErr.Error()
				}
				session.record(req)
				p.log.Info("proxied request", "session", session.ID, "path", req.Path, "status", req.Status,
					"streamed", req.Streamed, "tokens", u.Total(), "seconds", fmt.Sprintf("%.2f", req.Duration), "error", req.Error)
			})
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			req.Status = http.StatusBadGateway
			req.Duration = time.Since(start).Seconds()
			req.Error = err.Error()
			session.record(req)
			apiError(w, http.StatusBadGateway, "upstream_unreachable", name+" could not be reached: "+err.Error())
		},
	}
	rp.ServeHTTP(w, r)
}
