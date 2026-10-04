// Package report writes the comparison report described in doc/20-reports.md: acceptance
// criteria, a verifier, a blind reviewer and an analyst per side, a score computed here, a judge
// that runs twice with the sides swapped, a harness auditor per side and a short headline.
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

const Version = 2

type Report struct {
	Version      int                    `json:"version"`
	ComparisonID string                 `json:"comparisonId"`
	Status       string                 `json:"status"`
	Error        string                 `json:"error,omitempty"`
	Model        string                 `json:"model"`
	JudgeModel   string                 `json:"judgeModel"`
	CostUSD      *float64               `json:"costUsd"`
	Headline     string                 `json:"headline"`
	Criteria     []comparison.Criterion `json:"criteria"`
	CriteriaBy   string                 `json:"criteriaBy"`
	Sides        map[string]*SideReport `json:"sides"`
	Judge        *Judgement             `json:"judge,omitempty"`
	Warnings     []string               `json:"warnings"`
}

type SideReport struct {
	Gates       []Gate           `json:"gates"`
	Criteria    []CriterionCheck `json:"criteria"`
	Review      Review           `json:"review"`
	Analysis    string           `json:"analysis"`
	Score       Score            `json:"score"`
	NotVerified []string         `json:"notVerified"`
	Harness     *HarnessCost     `json:"harness,omitempty"`
	Audit       *Audit           `json:"audit,omitempty"`
	Subagents   []SubagentInfo   `json:"subagents"`
	Session     SessionSummary   `json:"session"`
}

type Gate struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason"`
}

type CriterionCheck struct {
	Index    int    `json:"index"`
	Status   string `json:"status"`
	Method   string `json:"method"`
	Evidence string `json:"evidence"`
}

type Review struct {
	Problems    []Finding  `json:"problems"`
	Strengths   []Strength `json:"strengths"`
	NotReviewed []string   `json:"notReviewed"`
}

type Finding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Impact   string `json:"impact"`
	Location string `json:"location"`
}

type Strength struct {
	Title    string `json:"title"`
	Location string `json:"location"`
}

type Score struct {
	Total float64     `json:"total"`
	Parts []ScorePart `json:"parts"`
}

type ScorePart struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Points float64     `json:"points"`
	Max    float64     `json:"max"`
	Lines  []ScoreLine `json:"lines"`
}

type ScoreLine struct {
	Label  string  `json:"label"`
	Points float64 `json:"points"`
	Max    float64 `json:"max"`
	Detail string  `json:"detail"`
}

type Judgement struct {
	Winner        string          `json:"winner"`
	Confidence    string          `json:"confidence"`
	Reasons       []string        `json:"reasons"`
	Ship          map[string]Ship `json:"ship"`
	Labels        []Verdict       `json:"labels"`
	Disagreements []string        `json:"disagreements"`
	PassesAgree   bool            `json:"passesAgree"`
	Passes        []string        `json:"passes"`
}

type Ship struct {
	Yes    bool   `json:"yes"`
	Reason string `json:"reason"`
}

type Verdict struct {
	Label string `json:"label"`
	Side  string `json:"side"`
}

type HarnessCost struct {
	FirstRequestTokens int64      `json:"firstRequestTokens"`
	Parts              []CostPart `json:"parts"`
	PerRequest         int64      `json:"perRequest"`
	Requests           int        `json:"requests"`
	Total              int64      `json:"total"`
	CacheShare         float64    `json:"cacheShare"`
	CostUSD            *float64   `json:"costUsd"`
	ShareOfSide        *float64   `json:"shareOfSide"`
	Files              []CostPart `json:"files"`
	Skills             []CostPart `json:"skills"`
	SkillsLoaded       []CostPart `json:"skillsLoaded"`
}

type CostPart struct {
	Label  string `json:"label"`
	Tokens int64  `json:"tokens"`
}

type Audit struct {
	Strengths   []AuditItem  `json:"strengths"`
	Gaps        []AuditItem  `json:"gaps"`
	Suggestions []Suggestion `json:"suggestions"`
}

type AuditItem struct {
	Title    string `json:"title"`
	Evidence string `json:"evidence"`
}

type Suggestion struct {
	Kind        string `json:"kind"`
	File        string `json:"file"`
	Change      string `json:"change"`
	Evidence    string `json:"evidence"`
	TokensSaved int64  `json:"tokensSaved"`
}

type SubagentInfo struct {
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Model       string         `json:"model"`
	Status      string         `json:"status"`
	DurationSec float64        `json:"durationSec"`
	Tokens      int64          `json:"tokens"`
	CostUSD     float64        `json:"costUsd"`
	Tools       map[string]int `json:"tools"`
}

type SessionSummary struct {
	Requests            int             `json:"requests"`
	CacheShare          float64         `json:"cacheShare"`
	ReasoningSteps      int             `json:"reasoningSteps"`
	ReasoningTokens     int64           `json:"reasoningTokens"`
	FirstEditSec        *float64        `json:"firstEditSec"`
	Tools               map[string]int  `json:"tools"`
	ToolCalls           int             `json:"toolCalls"`
	ToolFailures        int             `json:"toolFailures"`
	FailedCommands      []FailedCommand `json:"failedCommands"`
	EndsWithQuestion    bool            `json:"endsWithQuestion"`
	LongContextRequests int             `json:"longContextRequests"`
	ProviderErrors      int             `json:"providerErrors"`
	RateLimited         int             `json:"rateLimited"`
	RequestPoints       []RequestPoint  `json:"requestPoints"`
	ReasoningPoints     []int64         `json:"reasoningPoints"`
}

type FailedCommand struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
	Fixed    bool   `json:"fixed"`
	Agent    string `json:"agent"`
}

type RequestPoint struct {
	AtSec   float64  `json:"atSec"`
	Context int64    `json:"context"`
	CostUSD *float64 `json:"costUsd"`
}

type Options struct {
	Comparisons *comparison.Service
	Proxy       *proxy.Proxy
	Catalog     *catalog.Service
	Settings    *settings.Service
	ProxyURL    string
	Log         *slog.Logger
}

type Service struct {
	opts Options

	mu      sync.Mutex
	early   map[string]*early
	running map[string]bool
}

type early struct {
	once     sync.Once
	criteria []comparison.Criterion
	by       string
	err      error
	sides    map[string]*sideStages
	cost     costSum
}

type sideStages struct {
	once     sync.Once
	checks   []CriterionCheck
	unverif  []string
	review   Review
	analysis string
	err      error
}

type costSum struct {
	mu    sync.Mutex
	total float64
	known bool
}

func (c *costSum) add(v *float64) {
	if v == nil {
		return
	}
	c.mu.Lock()
	c.total += *v
	c.known = true
	c.mu.Unlock()
}

func (c *costSum) value() *float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.known {
		return nil
	}
	t := c.total
	return &t
}

func New(opts Options) *Service {
	return &Service{opts: opts, early: map[string]*early{}, running: map[string]bool{}}
}

func (s *Service) Recover(ctx context.Context) {
	for _, v := range s.opts.Comparisons.List() {
		if v.Report == "generating" {
			s.save(ctx, Report{Version: Version, ComparisonID: v.ID, Status: "error", Error: "ai-compare restarted while the report was being generated; generate it again"})
		}
	}
}

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
			s.stages(ctx, v, side)
			return
		}
		if err := s.Generate(ctx, id); err != nil && !errors.Is(err, errBusy) {
			s.opts.Log.Warn("automatic report failed", "comparison", id, "error", err)
		}
	}()
}

var errBusy = errors.New("the report is already being generated")

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

	st := s.opts.Settings.Get()
	if err := s.save(ctx, Report{Version: Version, ComparisonID: id, Status: "generating", Model: st.ReportModel, JudgeModel: st.JudgeModel}); err != nil {
		s.done(id)
		return err
	}
	go func() {
		defer s.done(id)
		r, err := s.generate(context.Background(), v)
		if err != nil {
			s.opts.Log.Warn("report failed", "comparison", id, "error", err)
			r = Report{Version: Version, ComparisonID: id, Status: "error", Error: err.Error(), Model: st.ReportModel, JudgeModel: st.JudgeModel}
		}
		s.save(context.Background(), r)
	}()
	return nil
}

func (s *Service) done(id string) {
	s.mu.Lock()
	delete(s.running, id)
	delete(s.early, id)
	s.mu.Unlock()
}

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

func (s *Service) state(id string) *early {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.early[id]
	if e == nil {
		e = &early{sides: map[string]*sideStages{"A": {}, "B": {}}}
		s.early[id] = e
	}
	return e
}

func (s *Service) criteria(ctx context.Context, v comparison.View) ([]comparison.Criterion, string, error) {
	e := s.state(v.ID)
	e.once.Do(func() {
		if len(v.Criteria) > 0 {
			e.criteria, e.by = v.Criteria, "user"
			return
		}
		c, err := s.newCaller(v.ID+"-criteria", s.opts.Settings.Get().JudgeModel, "medium")
		if err != nil {
			e.err = err
			return
		}
		defer c.close()
		e.criteria, e.err = s.writeCriteria(ctx, c, v.Prompt)
		e.by = "judge"
		e.cost.add(c.cost())
	})
	return e.criteria, e.by, e.err
}

func (s *Service) stages(ctx context.Context, v comparison.View, key string) (*sideStages, error) {
	crit, _, err := s.criteria(ctx, v)
	if err != nil {
		return nil, fmt.Errorf("acceptance criteria: %w", err)
	}
	e := s.state(v.ID)
	st := e.sides[key]
	st.once.Do(func() {
		f := s.facts(ctx, v, key)
		c, err := s.newCaller(v.ID+"-"+key, s.opts.Settings.Get().JudgeModel, "medium")
		if err != nil {
			st.err = err
			return
		}
		defer c.close()
		defer func() { e.cost.add(c.cost()) }()
		var wg sync.WaitGroup
		var errs [3]error
		wg.Add(3)
		go func() {
			defer wg.Done()
			st.checks, st.unverif, errs[0] = s.verify(ctx, c, v, f, crit)
		}()
		go func() {
			defer wg.Done()
			st.review, errs[1] = s.review(ctx, c, v, f)
		}()
		go func() {
			defer wg.Done()
			st.analysis, errs[2] = s.analyse(ctx, c, v, f)
		}()
		wg.Wait()
		if errs[0] != nil {
			errs[0] = fmt.Errorf("verifier of side %s: %w", key, errs[0])
		}
		if errs[1] != nil {
			errs[1] = fmt.Errorf("reviewer of side %s: %w", key, errs[1])
		}
		if errs[2] != nil {
			errs[2] = fmt.Errorf("analyst of side %s: %w", key, errs[2])
		}
		st.err = errors.Join(errs[:]...)
	})
	return st, st.err
}

func (s *Service) generate(ctx context.Context, v comparison.View) (Report, error) {
	st := s.opts.Settings.Get()
	r := Report{Version: Version, ComparisonID: v.ID, Status: "ready", Model: st.ReportModel, JudgeModel: st.JudgeModel,
		Sides: map[string]*SideReport{}, Warnings: warnings(v, st)}
	crit, by, err := s.criteria(ctx, v)
	if err != nil {
		return r, fmt.Errorf("acceptance criteria: %w", err)
	}
	r.Criteria, r.CriteriaBy = crit, by

	facts := map[string]*sideFacts{}
	stages := map[string]*sideStages{}
	var wg sync.WaitGroup
	var errs [2]error
	for i, key := range sides {
		facts[key] = s.facts(ctx, v, key)
		wg.Add(1)
		go func() {
			defer wg.Done()
			stages[key], errs[i] = s.stages(ctx, v, key)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs[:]...); err != nil {
		return r, err
	}

	for _, key := range sides {
		f, sg := facts[key], stages[key]
		r.Sides[key] = &SideReport{
			Criteria:    sg.checks,
			Review:      sg.review,
			Analysis:    sg.analysis,
			NotVerified: notVerified(f, sg),
			Harness:     f.harness,
			Subagents:   subagents(f),
			Session:     summary(f),
		}
		r.Sides[key].Gates = gates(f, crit, sg.checks)
	}
	scores := score(facts, r.Sides, crit)
	for _, key := range sides {
		r.Sides[key].Score = scores[key]
	}

	e := s.state(v.ID)
	judge, err := s.newCaller(v.ID+"-judge", st.JudgeModel, "high")
	if err != nil {
		return r, err
	}
	defer judge.close()
	j, err := s.judge(ctx, judge, v, facts, r)
	if err != nil {
		return r, fmt.Errorf("judge: %w", err)
	}
	r.Judge = &j
	e.cost.add(judge.cost())

	var awg sync.WaitGroup
	var aerrs [2]error
	for i, key := range sides {
		awg.Add(1)
		go func() {
			defer awg.Done()
			c, err := s.newCaller(v.ID+"-audit-"+key, st.JudgeModel, "medium")
			if err != nil {
				aerrs[i] = err
				return
			}
			defer c.close()
			a, err := s.audit(ctx, c, v, facts[key], r.Sides[key])
			e.cost.add(c.cost())
			if err != nil {
				aerrs[i] = fmt.Errorf("harness auditor of side %s: %w", key, err)
				return
			}
			r.Sides[key].Audit = &a
		}()
	}
	awg.Wait()
	if err := errors.Join(aerrs[:]...); err != nil {
		return r, err
	}

	w, err := s.newCaller(v.ID+"-writer", st.ReportModel, "low")
	if err != nil {
		return r, err
	}
	defer w.close()
	if r.Headline, err = s.headline(ctx, w, v, r); err != nil {
		return r, fmt.Errorf("writer: %w", err)
	}
	e.cost.add(w.cost())
	r.CostUSD = e.cost.value()
	return r, nil
}

var sides = []string{"A", "B"}

func warnings(v comparison.View, st settings.Settings) []string {
	out := []string{"One run per side: results vary from run to run, so treat small differences with care."}
	_, judge := settings.ModelRef(st.JudgeModel)
	for _, k := range sides {
		if v.Sides[k].Config.Model == judge {
			out = append(out, fmt.Sprintf("The judge model (%s) is also side %s's model; it may favour its own work.", judge, k))
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
