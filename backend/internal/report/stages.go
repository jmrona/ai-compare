package report

import (
	"context"
	"fmt"
	"strings"

	"ai-compare/backend/internal/comparison"
)

const maxDiffChars = 120_000

/* ── Blind reviewer ───────────────────────────────────────── */

const reviewerPrompt = `You are a meticulous senior code reviewer. You review one change that an AI coding agent made for a task.
Look for bugs, functional failures, unhandled edge cases, security problems and problems in the tests. Do not comment on style, formatting or naming.
Each finding needs: a short title; its impact on the user or the code; the location as path:line from the diff; and a severity: "high" (breaks the task or is a security problem), "medium" (a real problem in some cases) or "low" (minor).
Report only problems you can point to in the diff and are confident about; the test results are given to help you judge. If you find none, return an empty list. Write in British English.`

var reviewerSchema = obj(map[string]any{
	"findings": arr(obj(map[string]any{
		"severity": map[string]any{"type": "string", "enum": []string{"high", "medium", "low"}},
		"title":    str(),
		"impact":   str(),
		"location": str(),
	})),
})

func (s *Service) review(ctx context.Context, c *caller, v comparison.View, key string) ([]Finding, error) {
	d, err := s.opts.Comparisons.Diff(ctx, v.ID, key, "solution")
	if err != nil {
		return nil, err
	}
	if len(d.Files) == 0 {
		return []Finding{}, nil
	}
	var text strings.Builder
	for _, l := range d.Lines {
		if l.Kind == "file" {
			text.WriteString("\n### " + l.Text + "\n")
			continue
		}
		text.WriteString(l.Text + "\n")
	}
	// The reviewer is blind: nothing about the model, CLI or side.
	user := fmt.Sprintf("Task given to the agent:\n%s\n\nTests: %s\n\nFiles changed:\n%s\n\nDiff:\n%s", v.Prompt, testsLine(v.Sides[key].Tests), filesList(d.Files), clip(text.String(), maxDiffChars))
	var out struct {
		Findings []Finding `json:"findings"`
	}
	if err := c.ask(ctx, reviewerPrompt, user, "review", reviewerSchema, &out); err != nil {
		return nil, err
	}
	for i := range out.Findings {
		out.Findings[i].Side = key
	}
	return out.Findings, nil
}

/* ── Per-side analyst ─────────────────────────────────────── */

const analystPrompt = `You analyse one run of an AI coding agent on a task, from the facts given: status, metrics, tests, changed files, the agent's events and the warnings in its logs.
Summarise what happened in at most five short paragraphs of plain text separated by blank lines: what the agent did, how it went, what got in its way, and the outcome.
Separate facts from inferences: state facts plainly and mark inferences ("it seems", "probably"). Do not invent anything. Write in British English.`

var analystSchema = obj(map[string]any{"analysis": str()})

func (s *Service) analyse(ctx context.Context, c *caller, v comparison.View, key string) (string, error) {
	sv := v.Sides[key]
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\n", v.Prompt)
	fmt.Fprintf(&b, "Agent: %s %s, model %s, effort %q, %s mode.\n", sv.Config.CLI, sv.CLIVersion, sv.Config.Model, sv.Config.Effort, sv.Config.Mode)
	b.WriteString(sideFacts(sv))
	if tl, err := s.opts.Comparisons.Timeline(v.ID, key); err == nil && len(tl.Events) > 0 {
		b.WriteString("\nEvents (from the CLI session):\n")
		for i, e := range tl.Events {
			if i >= 150 {
				fmt.Fprintf(&b, "… %d more events\n", len(tl.Events)-i)
				break
			}
			fmt.Fprintf(&b, "%s %s: %s\n", e.At.Format("15:04:05"), e.Kind, e.Detail)
		}
	}
	if logs, err := s.opts.Comparisons.Logs(v.ID, key); err == nil {
		var warns []string
		for _, l := range logs {
			if l.Level != "info" {
				warns = append(warns, fmt.Sprintf("%s [%s] %s", l.Source, l.Level, l.Message))
			}
		}
		if len(warns) > 0 {
			b.WriteString("\nWarnings and errors in the logs:\n" + clip(joinLines(warns), 8000) + "\n")
		}
	}
	if _, visible, hidden, err := s.opts.Comparisons.TestOutput(v.ID, key); err == nil {
		if visible != "" {
			b.WriteString("\nEnd of the test output:\n" + tail(visible, 3000) + "\n")
		}
		if hidden != "" {
			b.WriteString("\nEnd of the hidden test output:\n" + tail(hidden, 2000) + "\n")
		}
	}
	var out struct {
		Analysis string `json:"analysis"`
	}
	if err := c.ask(ctx, analystPrompt, b.String(), "analysis", analystSchema, &out); err != nil {
		return "", err
	}
	return out.Analysis, nil
}

/* ── Comparative judge ────────────────────────────────────── */

const judgePrompt = `You compare two runs, A and B, of AI coding agents given the same task on the same project.
You receive each side's configuration, metrics, tests, changed files, the findings of a blind code review and an analysis of each run.
Explain which side performed better and where the trade-offs are. Do not force a single winner: a side can be cheaper while the other is faster or has fewer problems.
Give verdicts as short labels with the side that wins each: always include "Cheaper", "Faster", "Fewer problems" and "Overall", and "Tests" when tests ran. Use "tie" when neither is clearly better.
Then write the conclusions in two to five short paragraphs. Base everything on the facts given. Write in British English.`

var judgeSchema = obj(map[string]any{
	"verdicts": arr(obj(map[string]any{
		"label": str(),
		"side":  map[string]any{"type": "string", "enum": []string{"A", "B", "tie"}},
	})),
	"conclusions": arr(str()),
})

type judgement struct {
	Verdicts    []Verdict `json:"verdicts"`
	Conclusions []string  `json:"conclusions"`
}

func (s *Service) judge(ctx context.Context, c *caller, v comparison.View, parts map[string]*sidePart) (judgement, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n", v.Prompt)
	for _, key := range []string{"A", "B"} {
		sv := v.Sides[key]
		fmt.Fprintf(&b, "\n## Side %s\nAgent: %s %s, model %s, effort %q, %s mode.\n", key, sv.Config.CLI, sv.CLIVersion, sv.Config.Model, sv.Config.Effort, sv.Config.Mode)
		b.WriteString(sideFacts(sv))
		p := parts[key]
		if len(p.findings) == 0 {
			b.WriteString("Blind review: no findings.\n")
		} else {
			b.WriteString("Blind review findings:\n")
			for _, f := range p.findings {
				fmt.Fprintf(&b, "- [%s] %s (%s): %s\n", f.Severity, f.Title, f.Location, f.Impact)
			}
		}
		b.WriteString("Analysis:\n" + p.analysis + "\n")
	}
	var out judgement
	if err := c.ask(ctx, judgePrompt, b.String(), "judgement", judgeSchema, &out); err != nil {
		return out, err
	}
	for i := range out.Verdicts {
		if out.Verdicts[i].Side == "tie" {
			out.Verdicts[i].Side = ""
		}
	}
	return out, nil
}

/* ── Facts about a side ───────────────────────────────────── */

func sideFacts(sv comparison.SideView) string {
	var b strings.Builder
	m := sv.Metrics
	fmt.Fprintf(&b, "Harness: %s.\n", sv.Config.Harness.Label())
	fmt.Fprintf(&b, "Status: %s", sv.Status)
	if sv.EndReason != "" {
		fmt.Fprintf(&b, " (%s)", sv.EndReason)
	}
	if sv.Failure != "" {
		fmt.Fprintf(&b, ", %s failure", sv.Failure)
	}
	b.WriteString(".\n")
	fmt.Fprintf(&b, "Agent time: %.0f s", m.AgentSec)
	if m.HumanWaitSec != nil {
		fmt.Fprintf(&b, " (plus %.0f s waiting for the user)", *m.HumanWaitSec)
	}
	fmt.Fprintf(&b, ". Model requests: %d, of which %d failed.\n", m.Requests, m.Errors)
	fmt.Fprintf(&b, "Tokens: %d input, %d cached input, %d output.", m.Usage.Input, m.Usage.CacheRead, m.Usage.Output)
	if m.CostUSD != nil {
		fmt.Fprintf(&b, " Cost: $%.4f.", *m.CostUSD)
	}
	b.WriteString("\n")
	t := sv.Tests
	switch {
	case t.SkippedReason != "":
		fmt.Fprintf(&b, "Tests: not run (%s).\n", t.SkippedReason)
	case t.Visible != nil:
		fmt.Fprintf(&b, "Tests (%s): %s, exit %d.", t.Command, t.Visible.Status, t.Visible.ExitCode)
		if t.Hidden != nil {
			fmt.Fprintf(&b, " Hidden tests: %s, exit %d.", t.Hidden.Status, t.Hidden.ExitCode)
		}
		b.WriteString("\n")
	}
	if len(sv.Files) == 0 {
		b.WriteString("Files changed: none.\n")
	} else {
		b.WriteString("Files changed:\n" + filesList(sv.Files) + "\n")
	}
	if len(sv.HarnessFiles) > 0 {
		b.WriteString("Harness files changed:\n" + filesList(sv.HarnessFiles) + "\n")
	}
	return b.String()
}

// testsLine describes test results without anything that identifies the side.
func testsLine(t comparison.Tests) string {
	switch {
	case t.SkippedReason != "":
		return "not run (" + t.SkippedReason + ")"
	case t.Visible == nil:
		return "not run"
	}
	s := fmt.Sprintf("%s → %s", t.Command, t.Visible.Status)
	if t.Hidden != nil {
		s += fmt.Sprintf("; with tests the agent never saw → %s", t.Hidden.Status)
	}
	return s
}

func filesList(files []comparison.FileChange) string {
	lines := make([]string, 0, len(files))
	for i, f := range files {
		if i >= 200 {
			lines = append(lines, fmt.Sprintf("… %d more files", len(files)-i))
			break
		}
		lines = append(lines, fmt.Sprintf("%s +%d −%d", f.Path, f.Added, f.Removed))
	}
	return joinLines(lines)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

/* ── JSON schema helpers (strict structured outputs) ─────── */

func str() map[string]any { return map[string]any{"type": "string"} }

func arr(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func obj(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
