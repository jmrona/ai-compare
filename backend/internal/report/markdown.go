package report

import (
	"fmt"
	"strings"

	"ai-compare/backend/internal/comparison"
)

var statusLabel = map[string]string{"met": "met", "partial": "partial", "not_met": "not met", "not_verifiable": "not verifiable"}

func Markdown(v comparison.View, r *Report) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# Comparison %s\n\n", v.ID)
	if r.Headline != "" {
		w("**%s**\n\n", r.Headline)
	}
	w("## Task\n\n%s\n\n", quote(v.Prompt))
	w("| | A | B |\n|---|---|---|\n")
	w("| Agent | %s | %s |\n", sideLabel(v.Sides["A"]), sideLabel(v.Sides["B"]))
	w("| Harness | %s | %s |\n", v.Sides["A"].Config.Harness.Label(), v.Sides["B"].Config.Harness.Label())
	w("| Score | %s / 100 | %s / 100 |\n", trim(r.Sides["A"].Score.Total), trim(r.Sides["B"].Score.Total))
	w("| Cost | %s | %s |\n", usd(v.Sides["A"].Metrics.CostUSD), usd(v.Sides["B"].Metrics.CostUSD))
	w("| Agent time | %.0f s | %.0f s |\n\n", v.Sides["A"].Metrics.AgentSec, v.Sides["B"].Metrics.AgentSec)

	if j := r.Judge; j != nil {
		w("## Judge's verdict\n\n")
		w("Winner: **%s** · confidence %s", winnerLabel(j.Winner), j.Confidence)
		if !j.PassesAgree {
			w(" · the two passes disagreed (%s, then %s)", winnerLabel(j.Passes[0]), winnerLabel(j.Passes[1]))
		}
		w("\n\n")
		for _, reason := range j.Reasons {
			w("- %s\n", reason)
		}
		for _, k := range sides {
			if sh, ok := j.Ship[k]; ok {
				w("- Would ship %s: %s. %s\n", k, yesNo(sh.Yes), sh.Reason)
			}
		}
		for _, d := range j.Disagreements {
			w("- Disagreement with the score: %s\n", d)
		}
		w("\n")
	}

	w("## Gates\n\n| Gate | A | B |\n|---|---|---|\n")
	for i, g := range r.Sides["A"].Gates {
		gb := r.Sides["B"].Gates[i]
		w("| %s | %s | %s |\n", g.Label, gateCell(g), gateCell(gb))
	}

	w("\n## Score\n\n")
	for i, p := range r.Sides["A"].Score.Parts {
		pb := r.Sides["B"].Score.Parts[i]
		w("### %s · A %s · B %s of %s\n\n| | A | B |\n|---|---|---|\n", p.Label, trim(p.Points), trim(pb.Points), trim(p.Max))
		for li, l := range p.Lines {
			other := ""
			if li < len(pb.Lines) && pb.Lines[li].Label == l.Label {
				other = fmt.Sprintf("%s · %s", trim(pb.Lines[li].Points), pb.Lines[li].Detail)
			}
			w("| %s | %s · %s | %s |\n", l.Label, trim(l.Points), l.Detail, other)
		}
		w("\n")
	}

	if len(r.Criteria) > 0 {
		w("## Acceptance criteria (written by %s)\n\n| # | Criterion | A | B |\n|---|---|---|---|\n", map[string]string{"user": "you", "judge": "the judge model"}[r.CriteriaBy])
		for i, c := range r.Criteria {
			w("| %d | %s (%s) | %s | %s |\n", i+1, cell(c.Text), importance(c), checkCell(r.Sides["A"].Criteria, i), checkCell(r.Sides["B"].Criteria, i))
		}
		w("\n")
	}

	for _, k := range sides {
		s := r.Sides[k]
		w("## Side %s\n\n### Review\n\n", k)
		for _, p := range s.Review.Problems {
			w("- **%s** %s (`%s`): %s\n", p.Severity, p.Title, p.Location, p.Impact)
		}
		for _, st := range s.Review.Strengths {
			w("- **strength** %s (`%s`)\n", st.Title, st.Location)
		}
		if len(s.NotVerified) > 0 {
			w("\n### Not verified\n\n")
			for _, n := range s.NotVerified {
				w("- %s\n", n)
			}
		}
		if h := s.Harness; h != nil {
			w("\n### Harness cost\n\n%d tokens per request × %d requests = %d tokens, %.0f %% from the cache, %s.\n", h.PerRequest, h.Requests, h.Total, h.CacheShare*100, usd(h.CostUSD))
		}
		if a := s.Audit; a != nil {
			w("\n### Harness audit\n\n")
			for _, it := range a.Strengths {
				w("- Strength: %s (%s)\n", it.Title, it.Evidence)
			}
			for _, it := range a.Gaps {
				w("- Gap: %s (%s)\n", it.Title, it.Evidence)
			}
			for _, sg := range a.Suggestions {
				w("- %s `%s`: %s", strings.ReplaceAll(sg.Kind, "_", " "), sg.File, sg.Change)
				if sg.TokensSaved > 0 {
					w(" (saves about %d tokens per request)", sg.TokensSaved)
				}
				w("\n")
			}
		}
		if len(s.Subagents) > 0 {
			w("\n### Subagents\n\n")
			for _, a := range s.Subagents {
				w("- %s · %s · %s · %d tokens · $%.4f · %.0f s\n", a.Type, a.Description, a.Model, a.Tokens, a.CostUSD, a.DurationSec)
			}
		}
		if s.Analysis != "" {
			w("\n### Analysis\n\n%s\n", s.Analysis)
		}
		w("\n")
	}
	for _, warn := range r.Warnings {
		w("> %s\n", warn)
	}
	return b.String()
}

func sideLabel(s comparison.SideView) string {
	l := s.Config.CLI + " · " + s.Config.Model
	if s.Config.Effort != "" {
		l += " · " + s.Config.Effort
	}
	return l
}

func winnerLabel(w string) string {
	if w == "" {
		return "tie"
	}
	return "side " + w
}

func gateCell(g Gate) string {
	if g.Passed {
		return "passed"
	}
	return "failed: " + cell(g.Reason)
}

func checkCell(checks []CriterionCheck, i int) string {
	for _, c := range checks {
		if c.Index == i {
			return statusLabel[c.Status] + " · " + cell(c.Evidence)
		}
	}
	return ""
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }

func quote(s string) string { return "> " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n> ") }

func usd(v *float64) string {
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("$%.4f", *v)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
