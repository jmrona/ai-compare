package report

import (
	"fmt"
	"math"
	"strings"

	"ai-compare/backend/internal/comparison"
)

const (
	criteriaMax     = 30.0
	existingMax     = 8.0
	newTestsMax     = 4.0
	lintMax         = 3.0
	functionalMax   = 45.0
	qualityBase     = 20.0
	qualityMax      = 25.0
	strengthsMax    = 5
	processMax      = 10.0
	efficiencyMax   = 20.0
	efficiencyFloor = 27.0
)

func passed(t *comparison.TestRun) bool { return t != nil && t.Status == "passed" }

func gates(f *sideFacts, crit []comparison.Criterion, checks []CriterionCheck) []Gate {
	sv := f.view
	finished := Gate{Key: "finished", Label: "Finished", Passed: sv.Status == "finished" && !f.session.EndsWithQuestion}
	switch {
	case sv.Status != "finished":
		finished.Reason = "ended as " + strings.ReplaceAll(sv.Status, "_", " ")
	case f.session.EndsWithQuestion:
		finished.Reason = "the last message is a question"
	default:
		finished.Reason = "finished the task"
	}
	changes := Gate{Key: "changes", Label: "Made changes", Passed: len(sv.Files) > 0, Reason: fmt.Sprintf("%d files changed", len(sv.Files))}

	broke := []string{}
	t, b := sv.Tests, sv.Tests.Baseline
	if t.Visible != nil && !passed(t.Visible) && (b == nil || passed(b.Tests)) {
		broke = append(broke, "the tests fail and passed on the original project")
	}
	if t.Lint != nil && !passed(t.Lint) && (b == nil || passed(b.Lint)) {
		broke = append(broke, "the linter fails and passed on the original project")
	}
	nothing := Gate{Key: "broke-nothing", Label: "Broke nothing that worked", Passed: len(broke) == 0, Reason: "tests and linter as good as before"}
	if len(broke) > 0 {
		nothing.Reason = strings.Join(broke, "; ")
	}

	missed := []string{}
	for _, c := range checks {
		if c.Index >= 0 && c.Index < len(crit) && crit[c.Index].Required && c.Status == "not_met" {
			missed = append(missed, fmt.Sprintf("%d", c.Index+1))
		}
	}
	required := Gate{Key: "required", Label: "Every required criterion", Passed: len(missed) == 0, Reason: "no required criterion missed"}
	if len(missed) > 0 {
		required.Reason = "missed criterion " + strings.Join(missed, ", ")
	}
	return []Gate{finished, changes, nothing, required}
}

func score(facts map[string]*sideFacts, reports map[string]*SideReport, crit []comparison.Criterion) map[string]Score {
	out := map[string]Score{}
	functional := map[string]ScorePart{}
	for _, key := range sides {
		functional[key] = functionality(facts[key], reports[key].Criteria, crit)
	}
	for _, key := range sides {
		f, r := facts[key], reports[key]
		other := facts[otherSide(key)]
		parts := []ScorePart{functional[key], quality(r.Review), process(f), efficiency(f, other, functional[key].Points)}
		total := 0.0
		for _, p := range parts {
			total += p.Points
		}
		out[key] = Score{Total: round1(total), Parts: parts}
	}
	return out
}

func otherSide(key string) string {
	if key == "A" {
		return "B"
	}
	return "A"
}

func functionality(f *sideFacts, checks []CriterionCheck, crit []comparison.Criterion) ScorePart {
	t, b := f.view.Tests, f.view.Tests.Baseline
	hasExisting := t.Command != "" && b != nil && passed(b.Tests)
	critMax := criteriaMax
	if t.LintCommand == "" {
		critMax += lintMax
	}
	newMax := newTestsMax
	if !hasExisting {
		newMax += existingMax
	}

	var achieved, possible float64
	for _, c := range checks {
		if c.Index < 0 || c.Index >= len(crit) || c.Status == "not_verifiable" {
			continue
		}
		w := 1.0
		if crit[c.Index].Required {
			w = 2
		}
		possible += w
		switch c.Status {
		case "met":
			achieved += w
		case "partial":
			achieved += w / 2
		}
	}
	critLine := ScoreLine{Label: "Acceptance criteria", Max: critMax}
	if possible > 0 {
		critLine.Points = critMax * achieved / possible
		critLine.Detail = fmt.Sprintf("weighted %s of %s", trim(achieved), trim(possible))
	} else {
		critLine.Detail = "no criterion could be verified"
	}
	lines := []ScoreLine{critLine}

	if hasExisting {
		l := ScoreLine{Label: "Existing tests still pass", Max: existingMax}
		if passed(t.Visible) {
			l.Points, l.Detail = existingMax, "pass, as on the original project"
		} else {
			l.Detail = "they passed on the original project and fail now"
		}
		lines = append(lines, l)
	}
	nl := ScoreLine{Label: "New tests that pass", Max: newMax}
	switch {
	case len(f.newTests) == 0:
		nl.Detail = "no test files added or changed"
	case t.Command == "":
		nl.Detail = fmt.Sprintf("%d test files, but no test command to run them", len(f.newTests))
	case passed(t.Visible):
		nl.Points, nl.Detail = newMax, fmt.Sprintf("%d test files added or changed; the tests pass", len(f.newTests))
	default:
		nl.Detail = fmt.Sprintf("%d test files added or changed; the tests fail", len(f.newTests))
	}
	lines = append(lines, nl)
	if t.LintCommand != "" {
		l := ScoreLine{Label: "Linter clean", Max: lintMax}
		if passed(t.Lint) {
			l.Points, l.Detail = lintMax, "passes"
		} else {
			l.Detail = "fails"
		}
		lines = append(lines, l)
	}
	return part("functionality", "Functionality", functionalMax, lines)
}

func quality(r Review) ScorePart {
	lines := []ScoreLine{{Label: "Base", Points: qualityBase, Max: qualityBase}}
	penalty := map[string]float64{"high": 8, "medium": 3, "low": 1}
	for _, p := range r.Problems {
		lines = append(lines, ScoreLine{Label: p.Severity + ": " + p.Title, Points: -penalty[p.Severity], Detail: p.Location})
	}
	for i, s := range r.Strengths {
		pts := 1.0
		if i >= strengthsMax {
			pts = 0
		}
		lines = append(lines, ScoreLine{Label: "strength: " + s.Title, Points: pts, Detail: s.Location})
	}
	p := part("quality", "Code quality", qualityMax, lines)
	p.Points = math.Max(0, math.Min(qualityMax, p.Points))
	return p
}

func process(f *sideFacts) ScorePart {
	lines := []ScoreLine{}
	unfixed := 0
	for _, c := range f.session.FailedCommands {
		if !c.Fixed {
			unfixed++
		}
	}
	l := ScoreLine{Label: "Failed commands left unfixed", Points: -math.Min(6, float64(2*unfixed)), Detail: fmt.Sprintf("%d of %d failed", unfixed, len(f.session.FailedCommands))}
	lines = append(lines, l)
	q := ScoreLine{Label: "Ended with a question", Detail: "no"}
	if f.session.EndsWithQuestion {
		q.Points, q.Detail = -2, "yes"
	}
	lines = append(lines, q)
	tf := ScoreLine{Label: "Tool calls that failed", Detail: fmt.Sprintf("%d of %d", f.session.ToolFailures, f.session.ToolCalls)}
	if f.session.ToolCalls > 0 && float64(f.session.ToolFailures)/float64(f.session.ToolCalls) > 0.2 {
		tf.Points = -2
	}
	lines = append(lines, tf)
	p := part("process", "Process", processMax, lines)
	p.Points = math.Max(0, processMax+p.Points)
	return p
}

func efficiency(f, other *sideFacts, functional float64) ScorePart {
	if functional < efficiencyFloor {
		return ScorePart{Key: "efficiency", Label: "Efficiency", Max: efficiencyMax, Lines: []ScoreLine{
			{Label: "Functionality threshold", Max: efficiencyMax, Detail: fmt.Sprintf("%s of %s is below %s: efficiency does not count", trim(functional), trim(functionalMax), trim(efficiencyFloor))},
		}}
	}
	ratio := func(own, best *float64) (float64, string) {
		if own == nil || best == nil || *own <= 0 {
			return 0, "not known"
		}
		if *best >= *own {
			return 10, "the better of the two"
		}
		return 10 * *best / *own, "10 × better ÷ its own"
	}
	m, om := f.view.Metrics, other.view.Metrics
	bestCost := m.CostUSD
	if om.CostUSD != nil && (bestCost == nil || *om.CostUSD < *bestCost) {
		bestCost = om.CostUSD
	}
	ownTime, otherTime := m.AgentSec, om.AgentSec
	bestTime := math.Min(ownTime, otherTime)
	if otherTime <= 0 {
		bestTime = ownTime
	}
	cp, cd := ratio(m.CostUSD, bestCost)
	tp, td := ratio(&ownTime, &bestTime)
	lines := []ScoreLine{
		{Label: "Cost", Points: cp, Max: 10, Detail: cd},
		{Label: "Agent time", Points: tp, Max: 10, Detail: td},
	}
	return part("efficiency", "Efficiency", efficiencyMax, lines)
}

func part(key, label string, max float64, lines []ScoreLine) ScorePart {
	total := 0.0
	for i := range lines {
		lines[i].Points = round1(lines[i].Points)
		total += lines[i].Points
	}
	return ScorePart{Key: key, Label: label, Points: round1(math.Min(total, max)), Max: max, Lines: lines}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func trim(v float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") }
