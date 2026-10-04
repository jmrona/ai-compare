package report

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/proxy"
)

type sideFacts struct {
	key          string
	view         comparison.SideView
	diff         comparison.Diff
	diffText     string
	newTests     []string
	testOut      string
	hiddenOut    string
	lintOut      string
	session      comparison.SessionFacts
	requests     []proxy.Request
	harness      *HarnessCost
	harnessFiles map[string]string
	warnings     []string
	events       []comparison.TimelineEvent
}

var testFile = regexp.MustCompile(`(^|/)(tests?|__tests__|spec)/|[._-](test|spec)\.[a-z]+$|(^|/)test_[^/]+\.py$|_test\.go$`)

func (s *Service) facts(ctx context.Context, v comparison.View, key string) *sideFacts {
	cs := s.opts.Comparisons
	sv := v.Sides[key]
	f := &sideFacts{key: key, view: sv}
	if d, err := cs.Diff(ctx, v.ID, key, "solution"); err == nil {
		f.diff = d
		var b strings.Builder
		for _, l := range d.Lines {
			if l.Kind == "file" {
				b.WriteString("\n### " + l.Text + "\n")
				continue
			}
			b.WriteString(l.Text + "\n")
		}
		f.diffText = b.String()
	}
	for _, file := range sv.Files {
		if testFile.MatchString(file.Path) {
			f.newTests = append(f.newTests, file.Path)
		}
	}
	if _, visible, hidden, err := cs.TestOutput(v.ID, key); err == nil {
		f.testOut, f.hiddenOut = visible, hidden
	}
	f.lintOut = cs.ArtifactText(v.ID, key, "lint.log")
	f.session, _ = cs.SessionFacts(v.ID, key)
	f.requests, _ = cs.Requests(v.ID, key)
	f.harnessFiles, _ = cs.HarnessText(v.ID, key)
	if tl, err := cs.Timeline(v.ID, key); err == nil {
		f.events = tl.Events
	}
	if logs, err := cs.Logs(v.ID, key); err == nil {
		for _, l := range logs {
			if l.Level != "info" {
				f.warnings = append(f.warnings, fmt.Sprintf("%s [%s] %s", l.Source, l.Level, l.Message))
			}
		}
	}
	f.harness = harnessCost(cs.FirstRequest(v.ID, key), f.requests, f.session, sv)
	return f
}

func subagents(f *sideFacts) []SubagentInfo {
	out := []SubagentInfo{}
	for _, a := range f.session.Subagents {
		out = append(out, SubagentInfo{Type: a.Type, Description: a.Description, Model: a.Model, Status: a.Status,
			DurationSec: a.DurationSec, Tokens: a.Tokens, CostUSD: a.CostUSD, Tools: a.Tools})
	}
	return out
}

func summary(f *sideFacts) SessionSummary {
	sf := f.session
	out := SessionSummary{
		Requests: len(f.requests), ReasoningSteps: sf.ReasoningSteps, ReasoningTokens: sf.ReasoningTokens,
		FirstEditSec: sf.FirstEditSec, Tools: sf.Tools, ToolCalls: sf.ToolCalls, ToolFailures: sf.ToolFailures,
		EndsWithQuestion: sf.EndsWithQuestion, FailedCommands: []FailedCommand{}, RequestPoints: []RequestPoint{}, ReasoningPoints: []int64{},
	}
	if out.Tools == nil {
		out.Tools = map[string]int{}
	}
	for _, c := range sf.FailedCommands {
		out.FailedCommands = append(out.FailedCommands, FailedCommand{Command: clip(c.Command, 200), ExitCode: c.ExitCode, Fixed: c.Fixed, Agent: c.Agent})
	}
	for _, st := range sf.Steps {
		out.ReasoningPoints = append(out.ReasoningPoints, st.Reasoning)
	}
	var cached, prompt int64
	var start time.Time
	long := f.view.PriceSnapshot.LongContext
	requests := append([]proxy.Request(nil), f.requests...)
	sort.SliceStable(requests, func(i, j int) bool { return requests[i].At.Before(requests[j].At) })
	for _, r := range requests {
		if start.IsZero() {
			start = r.At
		}
		u := r.Usage
		cached += u.CacheRead
		prompt += u.PromptTokens()
		if long != nil && long.AboveTokens > 0 && u.PromptTokens() > int64(long.AboveTokens) {
			out.LongContextRequests++
		}
		switch {
		case r.Status == 429:
			out.RateLimited++
		case r.Status >= 500 || (r.Error != "" && !r.Cancelled):
			out.ProviderErrors++
		}
		if u.Reported {
			out.RequestPoints = append(out.RequestPoints, RequestPoint{AtSec: r.At.Sub(start).Seconds(), Context: u.PromptTokens(), CostUSD: r.CostUSD})
		}
	}
	if prompt > 0 {
		out.CacheShare = float64(cached) / float64(prompt)
	}
	return out
}

func notVerified(f *sideFacts, sg *sideStages) []string {
	out := append([]string{}, sg.unverif...)
	for _, n := range sg.review.NotReviewed {
		out = append(out, "Review: "+n)
	}
	t := f.view.Tests
	if t.Command == "" {
		out = append(out, "Tests: the project profile has no test command, so no test ran.")
	}
	if t.LintCommand == "" {
		out = append(out, "Linter: the project profile has no lint command; consistency and style were judged by reading the code.")
	}
	if f.diff.Truncated {
		out = append(out, "The diff was too long to read in full; only its first part was checked.")
	}
	return out
}
