// Package catalog keeps the model and price catalogue from models.dev.
//
// models.dev only publishes the whole catalogue (~5 MB, every provider) at a single URL,
// so the service downloads it at most once per refresh interval, asks with If-None-Match
// so an unchanged catalogue costs a 304, keeps only the providers ai-compare uses and
// stores that subset on disk. If models.dev cannot be reached, the last saved copy is
// served and flagged as stale.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Price is in USD per million tokens. A nil field means the provider does not charge it.
type Price struct {
	Input      float64  `json:"input"`
	CacheRead  *float64 `json:"cacheRead"`
	CacheWrite *float64 `json:"cacheWrite"`
	Output     float64  `json:"output"`
}

// LongContext is the price that applies once the prompt exceeds AboveTokens.
type LongContext struct {
	AboveTokens int   `json:"aboveTokens"`
	Price       Price `json:"price"`
}

type Model struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Provider    string       `json:"provider"`
	Family      string       `json:"family,omitempty"`
	ReleaseDate string       `json:"releaseDate,omitempty"`
	Deprecated  bool         `json:"deprecated"`
	ToolCall    bool         `json:"toolCall"`
	TextOutput  bool         `json:"textOutput"`
	Efforts     []string     `json:"efforts"`
	ContextK    int          `json:"contextK"`
	Price       *Price       `json:"price"`
	LongContext *LongContext `json:"longContext,omitempty"`
}

type Catalog struct {
	Source string `json:"source"`
	// FetchedAt is the last time models.dev confirmed this data (a 200 or a 304).
	FetchedAt time.Time `json:"fetchedAt"`
	// FromCache is true when the last attempt to reach models.dev failed and this is the saved copy.
	FromCache bool    `json:"fromCache"`
	Warning   string  `json:"warning,omitempty"`
	Models    []Model `json:"models"`
}

type Options struct {
	URL       string
	Providers []string
	CacheFile string
	MaxAge    time.Duration
	Client    *http.Client
	Log       *slog.Logger
}

type Service struct {
	opts Options

	mu      sync.Mutex
	current *Catalog
	etag    string
}

// cacheFile is what is stored on disk between restarts.
type cacheFile struct {
	ETag    string  `json:"etag"`
	Catalog Catalog `json:"catalog"`
}

func New(opts Options) *Service {
	if opts.MaxAge == 0 {
		opts.MaxAge = 24 * time.Hour
	}
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 30 * time.Second}
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	s := &Service{opts: opts}
	s.loadCache()
	return s
}

// Get returns the catalogue, refreshing it first when it is older than MaxAge.
func (s *Service) Get(ctx context.Context) (Catalog, error) {
	s.mu.Lock()
	fresh := s.current != nil && time.Since(s.current.FetchedAt) < s.opts.MaxAge
	s.mu.Unlock()
	if fresh {
		return s.snapshot(), nil
	}
	return s.Refresh(ctx)
}

// Refresh asks models.dev for changes now. On failure it falls back to the saved copy.
func (s *Service) Refresh(ctx context.Context) (Catalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.fetch(ctx)
	if err == nil {
		return *s.current, nil
	}
	s.opts.Log.Warn("models.dev refresh failed", "error", err)
	if s.current == nil {
		return Catalog{}, fmt.Errorf("models.dev is unreachable and there is no saved copy: %w", err)
	}
	s.current.FromCache = true
	s.current.Warning = "models.dev could not be reached; showing the copy saved on " + s.current.FetchedAt.Format(time.RFC1123)
	return *s.current, nil
}

func (s *Service) snapshot() Catalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.current
}

// fetch must be called with s.mu held.
func (s *Service) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opts.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if s.etag != "" && s.current != nil {
		req.Header.Set("If-None-Match", s.etag)
	}

	res, err := s.opts.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	now := time.Now().UTC()
	switch res.StatusCode {
	case http.StatusNotModified:
		s.current.FetchedAt = now
		s.current.FromCache = false
		s.current.Warning = ""
		s.saveCache()
		return nil
	case http.StatusOK:
	default:
		io.Copy(io.Discard, res.Body)
		return fmt.Errorf("models.dev returned %s", res.Status)
	}

	models, err := Parse(res.Body, s.opts.Providers)
	if err != nil {
		return err
	}
	s.current = &Catalog{Source: "models.dev", FetchedAt: now, Models: models}
	s.etag = res.Header.Get("ETag")
	s.saveCache()
	return nil
}

func (s *Service) loadCache() {
	if s.opts.CacheFile == "" {
		return
	}
	data, err := os.ReadFile(s.opts.CacheFile)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.opts.Log.Warn("could not read the catalogue cache", "error", err)
		return
	}
	var c cacheFile
	if err := json.Unmarshal(data, &c); err != nil {
		s.opts.Log.Warn("ignoring an unreadable catalogue cache", "error", err)
		return
	}
	s.current = &c.Catalog
	s.etag = c.ETag
}

// saveCache must be called with s.mu held. A failed write is logged, not fatal.
func (s *Service) saveCache() {
	if s.opts.CacheFile == "" || s.current == nil {
		return
	}
	data, err := json.Marshal(cacheFile{ETag: s.etag, Catalog: *s.current})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.opts.CacheFile), 0o755)
	}
	if err == nil {
		tmp := s.opts.CacheFile + ".tmp"
		if err = os.WriteFile(tmp, data, 0o644); err == nil {
			err = os.Rename(tmp, s.opts.CacheFile)
		}
	}
	if err != nil {
		s.opts.Log.Warn("could not save the catalogue cache", "error", err)
	}
}

/* ── Parsing the models.dev format ───────────────────────── */

type rawProvider struct {
	Models map[string]rawModel `json:"models"`
}

type rawCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Tiers      []struct {
		rawCostRates
		Tier struct {
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
}

type rawCostRates struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

type rawModel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Family           string `json:"family"`
	ReleaseDate      string `json:"release_date"`
	Status           string `json:"status"`
	ToolCall         bool   `json:"tool_call"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
	Modalities struct {
		Output []string `json:"output"`
	} `json:"modalities"`
	Limit struct {
		Context int `json:"context"`
	} `json:"limit"`
	Cost *rawCost `json:"cost"`
}

// Parse reads the full models.dev catalogue and returns the models of the given providers,
// newest release first.
func Parse(r io.Reader, providers []string) ([]Model, error) {
	var all map[string]json.RawMessage
	if err := json.NewDecoder(r).Decode(&all); err != nil {
		return nil, fmt.Errorf("models.dev returned invalid JSON: %w", err)
	}

	var models []Model
	for _, provider := range providers {
		raw, ok := all[provider]
		if !ok {
			continue
		}
		var p rawProvider
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("could not read provider %q: %w", provider, err)
		}
		for key, m := range p.Models {
			models = append(models, convert(provider, key, m))
		}
	}

	slices.SortFunc(models, func(a, b Model) int {
		if c := strings.Compare(b.ReleaseDate, a.ReleaseDate); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return models, nil
}

func convert(provider, key string, m rawModel) Model {
	id := m.ID
	if id == "" {
		id = key
	}
	out := Model{
		ID:          id,
		Name:        m.Name,
		Provider:    provider,
		Family:      m.Family,
		ReleaseDate: m.ReleaseDate,
		Deprecated:  m.Status == "deprecated",
		ToolCall:    m.ToolCall,
		TextOutput:  slices.Contains(m.Modalities.Output, "text"),
		Efforts:     []string{},
		ContextK:    m.Limit.Context / 1000,
	}
	for _, o := range m.ReasoningOptions {
		if o.Type == "effort" {
			out.Efforts = o.Values
		}
	}
	if m.Cost != nil && m.Cost.Input != nil && m.Cost.Output != nil {
		out.Price = &Price{Input: *m.Cost.Input, Output: *m.Cost.Output, CacheRead: m.Cost.CacheRead, CacheWrite: m.Cost.CacheWrite}
		for _, t := range m.Cost.Tiers {
			if t.Tier.Type == "context" && t.Input != nil && t.Output != nil {
				out.LongContext = &LongContext{
					AboveTokens: t.Tier.Size,
					Price:       Price{Input: *t.Input, Output: *t.Output, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite},
				}
				break
			}
		}
	}
	return out
}
