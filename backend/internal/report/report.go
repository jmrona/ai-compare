// Package report writes the comparison report with the report model (settings): a blind code
// review of each side's diff, an analysis of each side's run and a comparative judgement.
//
// The per-side stages run as soon as a side ends when automatic reports are on, so only the
// judgement is left when the second side ends. Model calls go through the inference proxy with
// a session of their own, so the report's cost is measured apart from the comparison's.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/settings"
)

type Report struct {
	ComparisonID string `json:"comparisonId"`
	// Status is "generating", "ready" or "error".
	Status      string            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Model       string            `json:"model"`
	CostUSD     *float64          `json:"costUsd"`
	Verdicts    []Verdict         `json:"verdicts"`
	Conclusions []string          `json:"conclusions"`
	Analysis    map[string]string `json:"analysis"`
	Findings    []Finding         `json:"findings"`
	Warnings    []string          `json:"warnings"`
}

type Verdict struct {
	Label string `json:"label"`
	// Side is "A", "B" or empty for a tie.
	Side string `json:"side"`
}

type Finding struct {
	Severity string `json:"severity"`
	Side     string `json:"side"`
	Title    string `json:"title"`
	Impact   string `json:"impact"`
	Location string `json:"location"`
}

// sidePart is the result of the per-side stages.
type sidePart struct {
	findings []Finding
	analysis string
	cost     float64
	costSeen bool
}

type Options struct {
	Comparisons *comparison.Service
	Proxy       *proxy.Proxy
	Catalog     *catalog.Service
	Settings    *settings.Service
	// ProxyURL is the proxy as api reaches it, e.g. http://127.0.0.1:4701.
	ProxyURL string
	Log      *slog.Logger
}

type Service struct {
	opts Options

	mu sync.Mutex
	// parts holds per-side stages done ahead of the judgement, by comparison and side.
	parts map[string]map[string]*sidePart
	// running marks comparisons whose report is being generated.
	running map[string]bool
}

func New(opts Options) *Service {
	return &Service{opts: opts, parts: map[string]map[string]*sidePart{}, running: map[string]bool{}}
}

// Recover marks reports that were being generated when api stopped as failed.
func (s *Service) Recover(ctx context.Context) {
	for _, v := range s.opts.Comparisons.List() {
		if v.Report == "generating" {
			s.save(ctx, Report{ComparisonID: v.ID, Status: "error", Error: "ai-compare restarted while the report was being generated; generate it again"})
		}
	}
}

// SideEnded starts the per-side stages early when automatic reports are on, and the whole
// report once both sides have ended.
func (s *Service) SideEnded(id, side string) {
	if !s.opts.Settings.Get().AutoReport {
		return
	}
	go func() {
		ctx := context.Background()
		v, err := s.opts.Comparisons.Get(id)
		if err != nil {
			return
		}
		if v.Live() {
			s.prepareSide(ctx, v, side)
			return
		}
		if err := s.Generate(ctx, id); err != nil && !errors.Is(err, errBusy) {
			s.opts.Log.Warn("automatic report failed", "comparison", id, "error", err)
		}
	}()
}

var errBusy = errors.New("the report is already being generated")

// Generate writes the report in the background and returns once it has started.
func (s *Service) Generate(ctx context.Context, id string) error {
	v, err := s.opts.Comparisons.Get(id)
	if err != nil {
		return err
	}
	if v.Live() {
		return fmt.Errorf("both sides must end before the report")
	}
	s.mu.Lock()
	if s.running[id] {
		s.mu.Unlock()
		return errBusy
	}
	s.running[id] = true
	s.mu.Unlock()

	model := s.opts.Settings.Get().ReportModel
	if err := s.save(ctx, Report{ComparisonID: id, Status: "generating", Model: model}); err != nil {
		s.done(id)
		return err
	}
	go func() {
		defer s.done(id)
		r, err := s.generate(context.Background(), v, model)
		if err != nil {
			s.opts.Log.Warn("report failed", "comparison", id, "error", err)
			r = Report{ComparisonID: id, Status: "error", Error: err.Error(), Model: model}
		}
		s.save(context.Background(), r)
	}()
	return nil
}

func (s *Service) done(id string) {
	s.mu.Lock()
	delete(s.running, id)
	s.mu.Unlock()
}

// Get returns the saved report, nil when there is none.
func (s *Service) Get(id string) (*Report, error) {
	data, err := s.opts.Comparisons.ReportData(id)
	if err != nil || data == nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) save(ctx context.Context, r Report) error {
	data, _ := json.Marshal(r)
	return s.opts.Comparisons.SetReport(ctx, r.ComparisonID, r.Status, data)
}

func (s *Service) generate(ctx context.Context, v comparison.View, model string) (Report, error) {
	r := Report{ComparisonID: v.ID, Status: "ready", Model: model, Analysis: map[string]string{}, Findings: []Finding{}, Warnings: warnings(v, model)}
	var total float64
	costKnown := false

	// Per-side stages that did not run early, in parallel.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, key := range []string{"A", "B"} {
		if s.part(v.ID, key) != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.prepareSide(ctx, v, key)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return r, err
	}
	parts := map[string]*sidePart{}
	for _, key := range []string{"A", "B"} {
		p := s.part(v.ID, key)
		if p == nil {
			return r, fmt.Errorf("the analysis of side %s is missing", key)
		}
		parts[key] = p
		r.Findings = append(r.Findings, p.findings...)
		r.Analysis[key] = p.analysis
		total += p.cost
		costKnown = costKnown || p.costSeen
	}

	c, err := s.newCaller(v.ID+"-judge", model)
	if err != nil {
		return r, err
	}
	defer c.close()
	j, err := s.judge(ctx, c, v, parts)
	if err != nil {
		return r, fmt.Errorf("comparative judgement: %w", err)
	}
	r.Verdicts, r.Conclusions = j.Verdicts, j.Conclusions
	if cost := c.cost(); cost != nil {
		total += *cost
		costKnown = true
	}
	if costKnown {
		r.CostUSD = &total
	}
	s.mu.Lock()
	delete(s.parts, v.ID)
	s.mu.Unlock()
	return r, nil
}

func (s *Service) part(id, side string) *sidePart {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parts[id][side]
}

// prepareSide runs the blind review and the analysis of one side.
func (s *Service) prepareSide(ctx context.Context, v comparison.View, key string) error {
	if s.part(v.ID, key) != nil {
		return nil
	}
	model := s.opts.Settings.Get().ReportModel
	c, err := s.newCaller(v.ID+"-"+key, model)
	if err != nil {
		return err
	}
	defer c.close()
	findings, err := s.review(ctx, c, v, key)
	if err != nil {
		return fmt.Errorf("blind review of side %s: %w", key, err)
	}
	analysis, err := s.analyse(ctx, c, v, key)
	if err != nil {
		return fmt.Errorf("analysis of side %s: %w", key, err)
	}
	p := &sidePart{findings: findings, analysis: analysis}
	if cost := c.cost(); cost != nil {
		p.cost, p.costSeen = *cost, true
	}
	s.mu.Lock()
	if s.parts[v.ID] == nil {
		s.parts[v.ID] = map[string]*sidePart{}
	}
	s.parts[v.ID][key] = p
	s.mu.Unlock()
	return nil
}

func warnings(v comparison.View, model string) []string {
	out := []string{"One run per side: results vary from run to run, so treat small differences with care."}
	for _, k := range []string{"A", "B"} {
		if v.Sides[k].Config.Model == model {
			out = append(out, fmt.Sprintf("The report model (%s) is also side %s's model; it may favour its own work.", model, k))
			break
		}
	}
	if v.Sides["A"].Config.Mode == "interactive" || v.Sides["B"].Config.Mode == "interactive" {
		out = append(out, "At least one side was interactive: human input makes the comparison less even.")
	}
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n[… cut: " + fmt.Sprint(len(s)-n) + " more characters]"
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }
