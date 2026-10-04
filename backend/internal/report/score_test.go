package report

import (
	"testing"

	"ai-compare/backend/internal/comparison"
)

func ptr(v float64) *float64 { return &v }

func run(status string) *comparison.TestRun { return &comparison.TestRun{Status: status} }

func side(cost, sec float64, tests comparison.Tests, files int, newTests []string) *sideFacts {
	sv := comparison.SideView{Status: "finished", Tests: tests}
	sv.Metrics.CostUSD, sv.Metrics.AgentSec = ptr(cost), sec
	for range files {
		sv.Files = append(sv.Files, comparison.FileChange{Path: "src/a.ts", Added: 1})
	}
	return &sideFacts{view: sv, newTests: newTests}
}

var crit = []comparison.Criterion{
	{Text: "Starts at 0", Required: true},
	{Text: "Saves to the database", Required: true},
	{Text: "No new dependencies"},
	{Text: "Fast enough"},
}

func TestScoreFavoursTheSideThatWorks(t *testing.T) {
	base := &comparison.Baseline{Tests: run("passed"), Lint: run("passed")}
	good := side(0.084, 212, comparison.Tests{Command: "pnpm test", LintCommand: "pnpm lint", Visible: run("passed"), Lint: run("passed"), Baseline: base}, 7, []string{"tests/a.test.ts"})
	bad := side(0.021, 96, comparison.Tests{Command: "pnpm test", LintCommand: "pnpm lint", Visible: run("failed"), Lint: run("passed"), Baseline: base}, 3, nil)
	bad.session.FailedCommands = []comparison.FailedCommand{{Command: "pnpm test", ExitCode: 1}}

	reports := map[string]*SideReport{
		"A": {
			Criteria: []CriterionCheck{{0, "met", "ran", ""}, {1, "met", "ran", ""}, {2, "met", "read", ""}, {3, "not_verifiable", "none", ""}},
			Review:   Review{Problems: []Finding{{Severity: "medium"}, {Severity: "low"}}, Strengths: []Strength{{}, {}, {}}},
		},
		"B": {
			Criteria: []CriterionCheck{{0, "met", "read", ""}, {1, "not_met", "read", ""}, {2, "met", "read", ""}, {3, "not_verifiable", "none", ""}},
			Review:   Review{Problems: []Finding{{Severity: "high"}}},
		},
	}
	facts := map[string]*sideFacts{"A": good, "B": bad}
	scores := score(facts, reports, crit)

	a, b := scores["A"], scores["B"]
	if a.Parts[0].Points != 45 {
		t.Errorf("A functionality = %v, want 45", a.Parts[0].Points)
	}
	if b.Parts[0].Points != 21 {
		t.Errorf("B functionality = %v, want 21 (criteria 18 + lint 3)", b.Parts[0].Points)
	}
	if b.Parts[3].Points != 0 {
		t.Errorf("B efficiency = %v, want 0 below the functionality threshold", b.Parts[3].Points)
	}
	if got := a.Parts[3].Points; got != 7 {
		t.Errorf("A efficiency = %v, want 7 (2.5 + 4.5)", got)
	}
	if a.Parts[1].Points != 19 || b.Parts[1].Points != 12 {
		t.Errorf("quality A %v, B %v; want 19 and 12", a.Parts[1].Points, b.Parts[1].Points)
	}
	if b.Parts[2].Points != 8 {
		t.Errorf("B process = %v, want 8", b.Parts[2].Points)
	}
	if a.Total <= b.Total {
		t.Errorf("A %v should beat B %v", a.Total, b.Total)
	}

	g := gates(bad, crit, reports["B"].Criteria)
	if g[2].Passed || g[3].Passed {
		t.Errorf("B should fail 'broke nothing' and 'required': %+v", g)
	}
	if ga := gates(good, crit, reports["A"].Criteria); !ga[0].Passed || !ga[1].Passed || !ga[2].Passed || !ga[3].Passed {
		t.Errorf("A should pass every gate: %+v", ga)
	}
}

func TestPointsMoveWhenChecksAreMissing(t *testing.T) {
	f := side(0.01, 10, comparison.Tests{}, 1, []string{"a.test.ts"})
	p := functionality(f, []CriterionCheck{{0, "met", "read", ""}, {1, "met", "read", ""}}, crit)
	if p.Lines[0].Max != 33 {
		t.Errorf("criteria max without a linter = %v, want 33", p.Lines[0].Max)
	}
	if p.Lines[1].Max != 12 || p.Lines[1].Points != 0 {
		t.Errorf("new tests line = %+v, want 0 of 12 with no test command", p.Lines[1])
	}
}
