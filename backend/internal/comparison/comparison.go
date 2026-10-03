// Package comparison runs comparisons: it prepares both sides (copy, image with the CLI, proxy
// session), starts their containers with a TTY, follows them until they end, verifies their
// result (diff, tests, CLI session) and reports their state, metrics and changes.
//
// State lives in memory and is saved to Postgres as it changes (store.go). Every change is also
// published to subscribers (events.go), which is how the UI follows a run.
//
// Files: comparison.go (types, starting, actions, views), run.go (preparing and following a
// side), verify.go (after a side ends), store.go (persistence and restarts), events.go,
// timeline.go (CLI sessions), human.go (human wait time), retention.go (clean-up).
package comparison

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/client"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/db"
	"ai-compare/backend/internal/presets"
	"ai-compare/backend/internal/preview"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/settings"
	"ai-compare/backend/internal/terminal"
	"ai-compare/backend/internal/workspace"
)

/* ── Types the API returns (converted to protobuf in internal/rpc) ── */

type Limits struct {
	TimeoutMin *float64 `json:"timeoutMin"`
	MaxTokensK *float64 `json:"maxTokensK"`
	MaxCostUSD *float64 `json:"maxCostUsd"`
}

type SideConfig struct {
	CLI      string  `json:"cli"`
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Effort   string  `json:"effort"`
	Mode     string  `json:"mode"`
	Limits   Limits  `json:"limits"`
	Harness  Harness `json:"harness"`
}

// Harness is which harness files a side runs with.
type Harness struct {
	// Kind is "project" (the project's own, as copied), "preset" or "none".
	Kind string `json:"kind"`
	// Preset is the preset's slug; Title and Hash record what it was when the comparison started.
	Preset string `json:"preset,omitempty"`
	Title  string `json:"title,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

// Label describes the harness for people: "project's harness", "no harness", "preset Strict backend".
func (h Harness) Label() string {
	switch h.Kind {
	case "none":
		return "no harness"
	case "preset":
		return "preset " + h.Title
	}
	return "project's harness"
}

type Profile struct {
	Runtime         string `json:"runtime"`
	Setup           string `json:"setup"`
	Test            string `json:"test"`
	HiddenTestsPath string `json:"hiddenTestsPath"`
	// PreviewCommand starts the application for the Preview tab; empty serves the side's files.
	PreviewCommand string `json:"previewCommand,omitempty"`
	PreviewPort    int    `json:"previewPort,omitempty"`
}

type NewComparison struct {
	ProjectPath string
	Profile     Profile
	Prompt      string
	Sides       map[string]SideConfig
	// Repetitions above 1 run a series of that many comparisons, one after another.
	Repetitions int
}

type Usage struct {
	Input      int64  `json:"input"`
	CacheRead  int64  `json:"cacheRead"`
	CacheWrite *int64 `json:"cacheWrite"`
	Output     int64  `json:"output"`
}

type Metrics struct {
	ElapsedSec       float64
	AgentSec         float64
	HumanWaitSec     *float64
	PrepSec          float64
	Phases           Phases
	Usage            Usage
	CostUSD          *float64
	CostConfirmedUSD *float64
	Requests         int
	Retries          int
	Errors           int
	TokensPerSec     *float64
}

// Phases is how long each step took. Both sides share the copy; their images build in parallel.
type Phases struct {
	CopySec   *float64 `json:"copySec"`
	BuildSec  *float64 `json:"buildSec"`
	StartSec  *float64 `json:"startSec"`
	VerifySec *float64 `json:"verifySec"`
}

type PriceSnapshot struct {
	Price       *catalog.Price       `json:"price"`
	LongContext *catalog.LongContext `json:"longContext,omitempty"`
	FetchedAt   time.Time            `json:"fetchedAt"`
}

type FileChange struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

type TestRun struct {
	// Status is "passed", "failed" or "error".
	Status      string  `json:"status"`
	ExitCode    int     `json:"exitCode"`
	DurationSec float64 `json:"durationSec"`
}

type Tests struct {
	Command       string   `json:"command"`
	Visible       *TestRun `json:"visible,omitempty"`
	Hidden        *TestRun `json:"hidden,omitempty"`
	SkippedReason string   `json:"skippedReason,omitempty"`
}

// Result is what is known about a side after its agent ended. It is saved as JSON.
type Result struct {
	// Outcome is the status the side ends with once verified, decided when the agent stopped.
	Outcome        string       `json:"outcome,omitempty"`
	OutcomeReason  string       `json:"outcomeReason,omitempty"`
	OutcomeFailure string       `json:"outcomeFailure,omitempty"`
	AgentEndedAt   *time.Time   `json:"agentEndedAt,omitempty"`
	Files          []FileChange `json:"files"`
	HarnessFiles   []FileChange `json:"harnessFiles"`
	Tests          Tests        `json:"tests"`
	HasResult      bool         `json:"hasResult"`
	HasRecording   bool         `json:"hasRecording"`
	HumanWaitSec   *float64     `json:"humanWaitSec,omitempty"`
	SessionUsage   *Usage       `json:"sessionUsage,omitempty"`
	SessionCostUSD *float64     `json:"sessionCostUsd,omitempty"`
}

type SideView struct {
	Key           string
	Config        SideConfig
	CLIVersion    string
	Status        string
	EndReason     string
	Failure       string
	Metrics       Metrics
	Files         []FileChange
	HarnessFiles  []FileChange
	Tests         Tests
	PriceSnapshot PriceSnapshot
	HasResult     bool
	HasRecording  bool
}

type View struct {
	ID          string
	CreatedAt   time.Time
	ProjectPath string
	ProjectName string
	Prompt      string
	Harness     string
	Profile     Profile
	Sides       map[string]SideView
	// Report is "none", "generating", "ready" or "error".
	Report string
	// Series: empty and zero for a single comparison.
	SeriesID      string
	Attempt       int
	SeriesSize    int
	SeriesStopped bool
}

// Live reports whether a side has not reached its final status yet.
func (v View) Live() bool {
	return !terminalStatuses[v.Sides["A"].Status] || !terminalStatuses[v.Sides["B"].Status]
}

type LogEntry struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

/* ── Internal state ───────────────────────────────────────── */

var terminalStatuses = map[string]bool{"finished": true, "error": true, "cancelled": true, "limit_reached": true}

type side struct {
	key          string
	comparisonID string
	cfg          SideConfig
	status       string
	endReason    string
	failure      string

	createdAt    time.Time
	runStartedAt time.Time
	phases       Phases
	endedAt      time.Time

	containerID string
	token       string
	session     *proxy.Session
	// stored is the saved proxy session of a side loaded from the database.
	stored *proxy.Snapshot
	price  PriceSnapshot
	hub    *terminal.Hub
	logs   []LogEntry
	result Result
	stop   context.CancelFunc
	saveMu sync.Mutex
}

type comparison struct {
	id           string
	createdAt    time.Time
	projectPath  string
	prompt       string
	profile      Profile
	sides        map[string]*side
	reportStatus string
	report       []byte
	cleanedAt    *time.Time
	// Series: attempt n of seriesSize; seededFrom is attempt 1, whose project copy later attempts reuse.
	seriesID      string
	attempt       int
	seriesSize    int
	seriesStopped bool
	seededFrom    string
}

// Reporter is told when sides and comparisons end, to prepare the report in the background.
type Reporter interface {
	SideEnded(comparisonID, side string)
}

type Options struct {
	Docker       *client.Client
	Workspace    *workspace.Service
	Proxy        *proxy.Proxy
	Catalog      *catalog.Service
	Settings     *settings.Service
	Presets      *presets.Store
	AgentNetwork string
	ProxyPort    int
	// DB saves comparisons; nil keeps them in memory only.
	DB  *db.Queries
	Log *slog.Logger
}

type Service struct {
	opts     Options
	mu       sync.Mutex
	all      map[string]*comparison
	events   bus
	reporter Reporter
}

func New(opts Options) *Service {
	s := &Service{opts: opts, all: map[string]*comparison{}}
	s.events.init()
	go s.publishLoop()
	return s
}

// SetReporter connects the report service, which is created after this one.
func (s *Service) SetReporter(r Reporter) { s.reporter = r }

var ErrNotFound = errors.New("comparison not found")

/* ── Starting ─────────────────────────────────────────────── */

func (s *Service) Start(ctx context.Context, in NewComparison) (string, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return "", fmt.Errorf("the prompt is empty")
	}
	if len(in.Sides) != 2 || in.Sides["A"].Model == "" || in.Sides["B"].Model == "" {
		return "", fmt.Errorf("both sides need a model")
	}
	for k, cfg := range in.Sides {
		if cfg.CLI != "opencode" {
			return "", fmt.Errorf("side %s: %s is not supported yet; phase 1 runs opencode", k, cfg.CLI)
		}
		if _, ok := opencodeProviders[cfg.Provider]; !ok {
			return "", fmt.Errorf("side %s: provider %s is not supported yet", k, cfg.Provider)
		}
		if cfg.Mode != "autonomous" && cfg.Mode != "interactive" {
			return "", fmt.Errorf("side %s: unknown mode %q", k, cfg.Mode)
		}
		h, err := s.resolveHarness(cfg.Harness)
		if err != nil {
			return "", fmt.Errorf("side %s: %w", k, err)
		}
		cfg.Harness = h
		in.Sides[k] = cfg
	}

	if in.Repetitions < 0 || in.Repetitions > MaxRepetitions {
		return "", fmt.Errorf("repetitions must be between 1 and %d", MaxRepetitions)
	}

	c := &comparison{
		id: newID("r", 3), createdAt: time.Now().UTC(), projectPath: strings.TrimSpace(in.ProjectPath),
		prompt: in.Prompt, profile: in.Profile, sides: map[string]*side{}, reportStatus: "none",
	}
	if in.Repetitions > 1 {
		c.seriesID, c.attempt, c.seriesSize = newID("s", 3), 1, in.Repetitions
	}
	for _, k := range []string{"A", "B"} {
		c.sides[k] = &side{key: k, comparisonID: c.id, cfg: in.Sides[k], status: "pending", createdAt: c.createdAt, hub: terminal.NewHub(s.opts.Log)}
	}
	// Each side keeps its own copy of its preset: editing the preset later does not change what ran.
	for k, sd := range c.sides {
		if sd.cfg.Harness.Kind != "preset" {
			continue
		}
		if err := s.snapshotPreset(s.opts.Presets.Dir(sd.cfg.Harness.Preset), c.id, k); err != nil {
			return "", err
		}
	}
	if err := s.launch(ctx, c); err != nil {
		return "", err
	}
	return c.id, nil
}

// MaxRepetitions bounds a series.
const MaxRepetitions = 10

func newID(prefix string, n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// snapshotPreset copies a preset for one side of a comparison: into staging for the build and
// into the side's artefacts for the history.
func (s *Service) snapshotPreset(src, id, side string) error {
	for _, dst := range []string{s.presetSnapshot(id, side), filepath.Join(s.opts.Workspace.ArtifactDir(id, side), "preset")} {
		if err := presets.CopyDir(src, dst); err != nil {
			return fmt.Errorf("copying side %s's preset: %w", side, err)
		}
	}
	return nil
}

// launch saves a new comparison and starts running it.
func (s *Service) launch(ctx context.Context, c *comparison) error {
	if err := s.insert(ctx, c); err != nil {
		return err
	}
	s.mu.Lock()
	s.all[c.id] = c
	s.mu.Unlock()
	s.changed(c)
	go s.run(c)
	return nil
}

/* ── Series (repetitions) ─────────────────────────────────── */

// continueSeries starts the next attempt of a series once the current one has ended: same project
// copy (attempt 1's), prompt, profile, sides and preset snapshots.
func (s *Service) continueSeries(c *comparison) {
	s.mu.Lock()
	if c.seriesID == "" || c.attempt >= c.seriesSize || c.seriesStopped {
		s.mu.Unlock()
		return
	}
	for _, sd := range c.sides {
		if !terminalStatuses[sd.status] {
			s.mu.Unlock()
			return
		}
	}
	var first *comparison
	for _, o := range s.all {
		if o.seriesID != c.seriesID {
			continue
		}
		if o.attempt > c.attempt || o.seriesStopped {
			// The next one exists already, or the series was stopped.
			s.mu.Unlock()
			return
		}
		if o.attempt == 1 {
			first = o
		}
	}
	if first == nil {
		first = c
	}
	next := &comparison{
		id: newID("r", 3), createdAt: time.Now().UTC(), projectPath: c.projectPath, prompt: c.prompt, profile: c.profile,
		sides: map[string]*side{}, reportStatus: "none",
		seriesID: c.seriesID, attempt: c.attempt + 1, seriesSize: c.seriesSize, seededFrom: first.id,
	}
	for k, sd := range c.sides {
		next.sides[k] = &side{key: k, comparisonID: next.id, cfg: sd.cfg, status: "pending", createdAt: next.createdAt, hub: terminal.NewHub(s.opts.Log)}
	}
	s.mu.Unlock()

	for k, sd := range next.sides {
		if sd.cfg.Harness.Kind == "preset" {
			if err := s.snapshotPreset(filepath.Join(s.opts.Workspace.ArtifactDir(first.id, k), "preset"), next.id, k); err != nil {
				s.opts.Log.Warn("series: could not copy a preset snapshot", "series", c.seriesID, "error", err)
				return
			}
		}
	}
	if err := s.launch(context.Background(), next); err != nil {
		s.opts.Log.Warn("series: could not start the next attempt", "series", c.seriesID, "error", err)
		return
	}
	s.opts.Log.Info("series: next attempt started", "series", c.seriesID, "attempt", next.attempt, "comparison", next.id)
}

// StopSeries keeps the attempts that have not started from running.
func (s *Service) StopSeries(ctx context.Context, seriesID string) error {
	found := false
	var changed []*comparison
	s.mu.Lock()
	for _, c := range s.all {
		if c.seriesID == seriesID {
			found = true
			c.seriesStopped = true
			changed = append(changed, c)
		}
	}
	s.mu.Unlock()
	if !found {
		return fmt.Errorf("series %s not found", seriesID)
	}
	if s.opts.DB != nil {
		if err := s.opts.DB.StopSeries(ctx, seriesID); err != nil {
			return err
		}
	}
	for _, c := range changed {
		s.changed(c)
	}
	return nil
}

// resolveHarness checks a side's harness choice and records the preset's title and hash.
func (s *Service) resolveHarness(h Harness) (Harness, error) {
	switch h.Kind {
	case "", "project":
		return Harness{Kind: "project"}, nil
	case "none":
		return Harness{Kind: "none"}, nil
	case "preset":
		if s.opts.Presets == nil {
			return Harness{}, fmt.Errorf("presets are not available")
		}
		p, err := s.opts.Presets.Get(h.Preset)
		if err != nil {
			return Harness{}, fmt.Errorf("preset %q: %w", h.Preset, err)
		}
		return Harness{Kind: "preset", Preset: p.Slug, Title: p.Title, Hash: p.Hash}, nil
	}
	return Harness{}, fmt.Errorf("unknown harness %q", h.Kind)
}

// presetSnapshot is where a side's copy of its preset lives during the run.
func (s *Service) presetSnapshot(id, side string) string {
	return filepath.Join(s.opts.Workspace.StagingDir(), id, "preset-"+side)
}

// PresetUses counts the sides that ran with a preset.
func (s *Service) PresetUses(slug string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.all {
		for _, sd := range c.sides {
			if sd.cfg.Harness.Kind == "preset" && sd.cfg.Harness.Preset == slug {
				n++
			}
		}
	}
	return n
}

/* ── Actions ──────────────────────────────────────────────── */

// Finish ends a side as finished (the user is done with it); Cancel ends it as cancelled.
func (s *Service) Finish(id, key string) error {
	return s.end(id, key, "finished", "Finished by the user")
}
func (s *Service) Cancel(id, key string) error {
	return s.end(id, key, "cancelled", "Cancelled by the user")
}

func (s *Service) end(id, key, status, reason string) error {
	c, sd, err := s.side(id, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	preparing := sd.stop == nil && !terminalStatuses[sd.status] && sd.status != "verifying" && sd.result.Outcome == ""
	s.mu.Unlock()
	if preparing {
		// Not running yet: there is no agent to stop or result to verify.
		s.finalize(c, sd, status, reason, "")
		return nil
	}
	if s.requestEnd(sd, status, reason, "") {
		s.mu.Lock()
		stop := sd.stop
		s.mu.Unlock()
		if stop != nil {
			stop()
		}
	}
	return nil
}

func (s *Service) Hub(id, key string) (*terminal.Hub, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	return sd.hub, nil
}

func (s *Service) side(id, key string) (*comparison, *side, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.all[id]
	if c == nil {
		return nil, nil, ErrNotFound
	}
	sd := c.sides[key]
	if sd == nil {
		return nil, nil, fmt.Errorf("side %q does not exist", key)
	}
	return c, sd, nil
}

/* ── Views ────────────────────────────────────────────────── */

func (s *Service) Get(id string) (View, error) {
	s.mu.Lock()
	c := s.all[id]
	s.mu.Unlock()
	if c == nil {
		return View{}, ErrNotFound
	}
	return s.view(c), nil
}

// Active returns the newest comparison that still has a side running, if any.
func (s *Service) Active() *View {
	for _, v := range s.List() {
		if v.Live() {
			return &v
		}
	}
	return nil
}

// List returns every comparison, newest first.
func (s *Service) List() []View {
	s.mu.Lock()
	cs := make([]*comparison, 0, len(s.all))
	for _, c := range s.all {
		cs = append(cs, c)
	}
	s.mu.Unlock()
	sort.Slice(cs, func(i, j int) bool { return cs[i].createdAt.After(cs[j].createdAt) })
	out := make([]View, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.view(c))
	}
	return out
}

func (s *Service) Logs(id, key string) ([]LogEntry, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	logs := append([]LogEntry(nil), sd.logs...)
	snap := sd.proxySnapshot()
	s.mu.Unlock()
	// Proxy requests read like log lines too.
	if snap != nil {
		for _, r := range snap.Requests {
			level, msg := "info", fmt.Sprintf("%s %s %s %d · %.1f s · in %d · out %d", r.Method, r.Path, r.Model, r.Status, r.Duration, r.Usage.PromptTokens(), r.Usage.Output)
			if r.Error != "" {
				level, msg = "warn", msg+" · "+r.Error
			} else if r.Cancelled {
				msg += " · cancelled by the CLI"
			}
			logs = append(logs, LogEntry{At: r.At, Level: level, Source: "proxy", Message: msg})
		}
	}
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].At.Before(logs[j].At) })
	return logs, nil
}

func (s *Service) view(c *comparison) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := View{
		ID: c.id, CreatedAt: c.createdAt, ProjectPath: c.projectPath, ProjectName: projectName(c.projectPath),
		Prompt: c.prompt, Harness: harnessLabel(c), Profile: c.profile, Sides: map[string]SideView{}, Report: c.reportStatus,
		SeriesID: c.seriesID, Attempt: c.attempt, SeriesSize: c.seriesSize, SeriesStopped: c.seriesStopped,
	}
	now := time.Now()
	for k, sd := range c.sides {
		end := now
		if !sd.endedAt.IsZero() {
			end = sd.endedAt
		}
		m := Metrics{ElapsedSec: end.Sub(sd.createdAt).Seconds(), Phases: sd.phases}
		snap := sd.proxySnapshot()
		if !sd.runStartedAt.IsZero() {
			agentEnd := end
			if sd.result.AgentEndedAt != nil {
				agentEnd = *sd.result.AgentEndedAt
			}
			m.PrepSec = sd.runStartedAt.Sub(sd.createdAt).Seconds()
			agentSec := agentEnd.Sub(sd.runStartedAt).Seconds()
			if sd.cfg.Mode == "interactive" {
				wait := sd.result.HumanWaitSec
				if wait == nil && snap != nil {
					w := humanWait(snap.Requests, sd.hub.Inputs(), sd.runStartedAt, agentEnd)
					wait = &w
				}
				m.HumanWaitSec = wait
				if wait != nil {
					agentSec -= *wait
				}
			}
			m.AgentSec = max(agentSec, 0)
		}
		if snap != nil {
			cw := snap.Usage.CacheWrite
			m.Usage = Usage{Input: snap.Usage.Input, CacheRead: snap.Usage.CacheRead, CacheWrite: &cw, Output: snap.Usage.Output}
			m.CostUSD = snap.CostUSD
			m.Requests = snap.RequestCnt
			var out int64
			var secs float64
			for _, r := range snap.Requests {
				if r.Error != "" || r.Status >= 400 {
					m.Errors++
				}
				if r.Usage.Reported && r.Streamed {
					out += r.Usage.Output
					secs += r.Duration
				}
			}
			if secs > 1 {
				tps := float64(out) / secs
				m.TokensPerSec = &tps
			}
		}
		v.Sides[k] = SideView{
			Key: k, Config: sd.cfg, CLIVersion: OpencodeVersion, Status: sd.status, EndReason: sd.endReason, Failure: sd.failure,
			Metrics: m, Files: agentFiles(sd.result.Files), HarnessFiles: sd.result.HarnessFiles, Tests: sd.result.Tests,
			PriceSnapshot: sd.price, HasResult: sd.result.HasResult, HasRecording: sd.result.HasRecording,
		}
	}
	return v
}

/* ── Status helpers ───────────────────────────────────────── */

// setStatus moves a side to a non-final status.
func (s *Service) setStatus(sd *side, status string) {
	s.mu.Lock()
	if terminalStatuses[sd.status] {
		s.mu.Unlock()
		return
	}
	sd.status = status
	s.mu.Unlock()
	s.save(sd, false)
	s.changedSide(sd)
}

// requestEnd decides how a running side ends (the first decision wins) and reports whether this
// call decided it.
func (s *Service) requestEnd(sd *side, status, reason, failure string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sd.result.Outcome != "" || terminalStatuses[sd.status] {
		return false
	}
	sd.result.Outcome, sd.result.OutcomeReason, sd.result.OutcomeFailure = status, reason, failure
	return true
}

// finalize gives a side its final status, saves it with its terminal output and tells the reporter.
func (s *Service) finalize(c *comparison, sd *side, status, reason, failure string) {
	s.mu.Lock()
	if terminalStatuses[sd.status] {
		s.mu.Unlock()
		return
	}
	sd.status, sd.endReason, sd.failure = status, reason, failure
	sd.endedAt = time.Now()
	sd.token = ""
	s.mu.Unlock()
	sd.hub.Close()
	s.save(sd, true)
	s.changed(c)
	if s.reporter != nil {
		s.reporter.SideEnded(c.id, sd.key)
	}
	go s.continueSeries(c)
}

func (s *Service) setPhase(sd *side, dst **float64, d time.Duration) {
	sec := d.Seconds()
	s.mu.Lock()
	*dst = &sec
	s.mu.Unlock()
	s.changedSide(sd)
}

func (s *Service) note(sd *side, source, level, msg string) {
	s.mu.Lock()
	sd.logs = append(sd.logs, LogEntry{At: time.Now().UTC(), Level: level, Source: source, Message: msg})
	s.mu.Unlock()
	if level != "error" {
		sd.hub.Note(msg)
	}
	s.changedSide(sd)
}

// fail ends a side that broke before or around its agent's run: an infrastructure error.
func (s *Service) fail(c *comparison, sd *side, source string, err error) {
	s.note(sd, source, "error", err.Error())
	sd.hub.Write([]byte("\r\n\x1b[91m[ai-compare] " + source + " failed: " + strings.ReplaceAll(err.Error(), "\n", "\r\n") + "\x1b[0m\r\n"))
	s.opts.Log.Warn("side failed", "comparison", c.id, "side", sd.key, "step", source, "error", err)
	s.finalize(c, sd, "error", source+" failed: "+firstLine(err.Error()), "infrastructure")
}

/* ── Reports ──────────────────────────────────────────────── */

// SetReport records the report's status and content (JSON from the report service).
func (s *Service) SetReport(ctx context.Context, id, status string, data []byte) error {
	s.mu.Lock()
	c := s.all[id]
	if c != nil {
		c.reportStatus = status
		if data != nil {
			c.report = data
		}
	}
	s.mu.Unlock()
	if c == nil {
		return ErrNotFound
	}
	if s.opts.DB != nil {
		if err := s.opts.DB.SaveReport(ctx, db.SaveReportParams{ID: id, ReportStatus: status, Report: data}); err != nil {
			return err
		}
	}
	s.changed(c)
	return nil
}

// ReportData returns the saved report JSON, nil if there is none.
func (s *Service) ReportData(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.all[id]
	if c == nil {
		return nil, ErrNotFound
	}
	return c.report, nil
}

/* ── Files ────────────────────────────────────────────────── */

// Download describes a side's files for download.
type Download struct {
	// Filename is the zip name, after the model that produced the work, e.g. gpt-5.4-mini-r1c5860-A.zip.
	Filename string
	// Tar is the side's /workspace as a tar stream; the caller closes it.
	Tar io.ReadCloser
}

// Workspace exports a side's working folder: the saved result once the side has ended (it
// survives retention), or a snapshot of the running container.
func (s *Service) Workspace(ctx context.Context, id, key string) (Download, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return Download{}, err
	}
	s.mu.Lock()
	containerID, model := sd.containerID, sd.cfg.Model
	s.mu.Unlock()
	name := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
	d := Download{Filename: fmt.Sprintf("%s-%s-%s.zip", name, id, key)}
	if f, err := os.Open(filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "workspace.tar")); err == nil {
		d.Tar = f
		return d, nil
	}
	if containerID == "" {
		return Download{}, fmt.Errorf("side %s has no files yet", key)
	}
	res, err := s.opts.Docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: "/workspace/."})
	if err != nil {
		return Download{}, fmt.Errorf("reading side %s's files: %w", key, err)
	}
	d.Tar = res.Content
	return d, nil
}

// PreviewTarget says what a side's preview runs from: its saved files, its result image and the
// profile's preview command. The side must have ended with a saved result.
func (s *Service) PreviewTarget(id, key string) (preview.Target, error) {
	c, sd, err := s.side(id, key)
	if err != nil {
		return preview.Target{}, err
	}
	s.mu.Lock()
	ended, saved := terminalStatuses[sd.status], sd.result.HasResult
	profile := c.profile
	s.mu.Unlock()
	if !ended || !saved {
		return preview.Target{}, fmt.Errorf("side %s has no saved result yet: the preview is available once it has ended", key)
	}
	return preview.Target{
		Tar:     filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "workspace.tar"),
		Image:   fmt.Sprintf("ai-compare/result:%s-%s", strings.ToLower(id), strings.ToLower(key)),
		Command: profile.PreviewCommand,
		Port:    profile.PreviewPort,
	}, nil
}

// Recording returns the path of a side's asciicast recording.
func (s *Service) Recording(id, key string) (string, error) {
	if _, _, err := s.side(id, key); err != nil {
		return "", err
	}
	p := filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "terminal.cast")
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("side %s has no recording", key)
	}
	return p, nil
}

/* ── Small helpers ────────────────────────────────────────── */

// agentFiles leaves out dependency folders, which results collected before they were excluded
// still list.
func agentFiles(files []FileChange) []FileChange {
	out, _ := withoutDependencies(files)
	return out
}

// projectName is the folder's name, or "empty project" for comparisons without one.
func projectName(path string) string {
	if path == "" {
		return "empty project"
	}
	return filepath.Base(strings.ReplaceAll(path, `\`, "/"))
}

// harnessLabel describes what both sides ran with, e.g. "project's harness" or "A: preset X · B: no harness".
func harnessLabel(c *comparison) string {
	a, b := c.sides["A"].cfg.Harness, c.sides["B"].cfg.Harness
	label := func(h Harness) string {
		if h.Kind == "project" && c.projectPath == "" {
			return "none (empty project)"
		}
		return h.Label()
	}
	if label(a) == label(b) {
		return label(a)
	}
	return "A: " + label(a) + " · B: " + label(b)
}

func runtimeOr(r string) string {
	if r == "" {
		return "node:22-bookworm-slim"
	}
	return r
}

func setupNote(setup string) string {
	if strings.TrimSpace(setup) == "" {
		return ""
	}
	return " + " + setup
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
