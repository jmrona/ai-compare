// Package settings keeps the user's editable settings, saved in Postgres (or in memory without
// a database).
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"ai-compare/backend/internal/db"
)

// Limits are optional per-side limits; nil means no limit.
type Limits struct {
	TimeoutMin *float64 `json:"timeoutMin"`
	MaxTokensK *float64 `json:"maxTokensK"`
	MaxCostUSD *float64 `json:"maxCostUsd"`
}

type Settings struct {
	// DefaultLimits are pre-filled on both sides of a new comparison.
	DefaultLimits Limits  `json:"defaultLimits"`
	ReportModel   string  `json:"reportModel"`
	JudgeModel    string  `json:"judgeModel"`
	AutoReport    bool    `json:"autoReport"`
	CPUs          float64 `json:"cpus"`
	MemoryGB      float64 `json:"memoryGb"`
	// RetentionDays after which what Retention selects is removed from an ended comparison; 0
	// removes it from every comparison that is not running.
	RetentionDays  int       `json:"retentionDays"`
	RetentionHours int       `json:"retentionHours"`
	Retention      Retention `json:"retention"`
}

// Retention selects what the clean-up removes. Reports and the history are always kept.
type Retention struct {
	Containers bool `json:"containers"`
	Images     bool `json:"images"`
	Staging    bool `json:"staging"`
	Artifacts  bool `json:"artifacts"`
}

func ptr(f float64) *float64 { return &f }

// Suggested are the values offered when a limit is switched on.
var Suggested = Limits{TimeoutMin: ptr(30), MaxTokensK: ptr(2000), MaxCostUSD: ptr(2)}

// Defaults applies until the user changes something.
func Defaults() Settings {
	return Settings{ReportModel: "openai/gpt-6-luna", JudgeModel: "openai/gpt-6.1-sol", CPUs: 2, MemoryGB: 4, RetentionDays: 2, Retention: Retention{Containers: true, Images: true, Staging: true}}
}

// RetentionAge is how long after a comparison ends retention removes what it selects.
func (s Settings) RetentionAge() time.Duration {
	return time.Duration(s.RetentionDays)*24*time.Hour + time.Duration(s.RetentionHours)*time.Hour
}

func ModelRef(ref string) (provider, model string) {
	if p, m, ok := strings.Cut(ref, "/"); ok {
		return p, m
	}
	return "openai", ref
}

type Service struct {
	q   *db.Queries
	log *slog.Logger

	mu  sync.Mutex
	cur Settings
}

// New loads the saved settings over the defaults. q may be nil (settings then live in memory).
func New(ctx context.Context, q *db.Queries, log *slog.Logger) *Service {
	s := &Service{q: q, log: log, cur: Defaults()}
	if q == nil {
		return s
	}
	data, err := q.GetSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return s
	}
	if err != nil {
		log.Warn("could not read the settings; using the defaults", "error", err)
		return s
	}
	if err := json.Unmarshal(data, &s.cur); err != nil {
		log.Warn("ignoring unreadable settings", "error", err)
		s.cur = Defaults()
	}
	return s
}

func (s *Service) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Update validates and saves new settings.
func (s *Service) Update(ctx context.Context, n Settings) (Settings, error) {
	if n.JudgeModel == "" {
		return Settings{}, fmt.Errorf("choose a judge model")
	}
	if n.ReportModel == "" {
		return Settings{}, fmt.Errorf("choose a report model")
	}
	if n.CPUs < 0.5 || n.CPUs > 64 {
		return Settings{}, fmt.Errorf("CPUs per side must be between 0.5 and 64")
	}
	if n.MemoryGB < 1 || n.MemoryGB > 256 {
		return Settings{}, fmt.Errorf("memory per side must be between 1 and 256 GB")
	}
	if n.RetentionDays < 0 || n.RetentionDays > 365 {
		return Settings{}, fmt.Errorf("retention must be between 0 and 365 days")
	}
	if n.RetentionHours < 0 || n.RetentionHours > 23 {
		return Settings{}, fmt.Errorf("retention hours must be between 0 and 23")
	}
	for _, l := range []*float64{n.DefaultLimits.TimeoutMin, n.DefaultLimits.MaxTokensK, n.DefaultLimits.MaxCostUSD} {
		if l != nil && *l <= 0 {
			return Settings{}, fmt.Errorf("limits must be greater than zero")
		}
	}
	if s.q != nil {
		data, _ := json.Marshal(n)
		if err := s.q.SaveSettings(ctx, data); err != nil {
			return Settings{}, fmt.Errorf("saving the settings: %w", err)
		}
	}
	s.mu.Lock()
	s.cur = n
	s.mu.Unlock()
	return n, nil
}
