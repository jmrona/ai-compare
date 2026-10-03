package catalog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sample = `{
  "openai": {"models": {
    "gpt-old":  {"id": "gpt-old", "name": "GPT Old", "release_date": "2025-01-10", "status": "deprecated", "tool_call": true,
                 "modalities": {"output": ["text"]}, "limit": {"context": 128000}, "cost": {"input": 10, "output": 30}},
    "gpt-new":  {"id": "gpt-new", "name": "GPT New", "release_date": "2026-09-29", "tool_call": true,
                 "reasoning_options": [{"type": "effort", "values": ["low", "medium", "high"]}],
                 "modalities": {"output": ["text"]}, "limit": {"context": 400000},
                 "cost": {"input": 2.5, "output": 15, "cache_read": 0.25,
                          "tiers": [{"input": 5, "output": 22.5, "tier": {"type": "context", "size": 272000}}]}},
    "gpt-image": {"id": "gpt-image", "name": "GPT Image", "release_date": "2026-05-01",
                  "modalities": {"output": ["image"]}, "limit": {"context": 32000}}
  }},
  "anthropic": {"models": {
    "claude-x": {"id": "claude-x", "name": "Claude X", "release_date": "2026-09-28", "tool_call": true,
                 "modalities": {"output": ["text"]}, "limit": {"context": 200000},
                 "cost": {"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75}}
  }},
  "some-other-provider": {"models": {"x": {"id": "x", "release_date": "2026-12-01"}}}
}`

func TestParseFiltersProvidersAndSortsNewestFirst(t *testing.T) {
	models, err := Parse(strings.NewReader(sample), []string{"openai", "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	want := "gpt-new,claude-x,gpt-image,gpt-old"
	if got := strings.Join(ids, ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}

	m := models[0]
	if m.Price == nil || m.Price.Input != 2.5 || *m.Price.CacheRead != 0.25 || m.Price.CacheWrite != nil {
		t.Errorf("price = %+v", m.Price)
	}
	if m.LongContext == nil || m.LongContext.AboveTokens != 272000 || m.LongContext.Price.Input != 5 {
		t.Errorf("long context = %+v", m.LongContext)
	}
	if strings.Join(m.Efforts, ",") != "low,medium,high" || m.ContextK != 400 || !m.TextOutput || !m.ToolCall {
		t.Errorf("model = %+v", m)
	}
	if !models[3].Deprecated {
		t.Error("gpt-old should be deprecated")
	}
	if models[2].Price != nil || models[2].TextOutput {
		t.Errorf("image model = %+v", models[2])
	}
	if c := models[1].Price.CacheWrite; c == nil || *c != 3.75 {
		t.Errorf("anthropic cache write = %v", c)
	}
}

func TestRefreshUsesETagAndFallsBackToCache(t *testing.T) {
	var calls, conditional atomic.Int32
	var down atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if down.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, sample)
	}))
	defer srv.Close()

	cacheFile := filepath.Join(t.TempDir(), "catalog.json")
	opts := Options{URL: srv.URL, Providers: []string{"openai"}, CacheFile: cacheFile, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx := context.Background()

	s := New(opts)
	first, err := s.Get(ctx)
	if err != nil || len(first.Models) != 3 || first.FromCache {
		t.Fatalf("first get: %+v, %v", first, err)
	}

	// Within MaxAge, Get must not call models.dev again.
	if _, err := s.Get(ctx); err != nil || calls.Load() != 1 {
		t.Fatalf("second get made %d calls, err %v", calls.Load(), err)
	}

	// A forced refresh sends If-None-Match and accepts the 304.
	if _, err := s.Refresh(ctx); err != nil || conditional.Load() != 1 {
		t.Fatalf("refresh: conditional=%d err=%v", conditional.Load(), err)
	}

	// A new service starts from the saved copy, and serves it flagged when models.dev is down.
	down.Store(true)
	restarted := New(Options{URL: srv.URL, Providers: []string{"openai"}, CacheFile: cacheFile, MaxAge: time.Nanosecond, Log: opts.Log})
	stale, err := restarted.Get(ctx)
	if err != nil || !stale.FromCache || stale.Warning == "" || len(stale.Models) != 3 {
		t.Fatalf("stale get: %+v, %v", stale, err)
	}
}

func TestRefreshWithoutCacheReportsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	s := New(Options{URL: srv.URL, Providers: []string{"openai"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := s.Get(context.Background()); err == nil {
		t.Fatal("expected an error with no cache and models.dev down")
	}
}
