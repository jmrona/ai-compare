package report

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ai-compare/backend/internal/comparison"
)

const maxDiffChars = 120_000

/* ── Criteria ─────────────────────────────────────────────── */

const criteriaPrompt = `You turn a task given to an AI coding agent into acceptance criteria: what the result must do to count as done.
Write from the task only; you see no result. Each criterion is one checkable statement about the result, short and specific. Mark it required when the task clearly asks for it, desirable when it is implied good practice the task hints at. Do not invent requirements the task does not support. Between 3 and 10 criteria. Write in British English.`

var criteriaSchema = obj(map[string]any{
	"criteria": arr(obj(map[string]any{"text": str(), "required": map[string]any{"type": "boolean"}})),
})

func (s *Service) writeCriteria(ctx context.Context, c *caller, prompt string) ([]comparison.Criterion, error) {
	var out struct {
		Criteria []comparison.Criterion `json:"criteria"`
	}
	if err := c.ask(ctx, criteriaPrompt, "Task:\n"+prompt, "criteria", criteriaSchema, &out); err != nil {
		return nil, err
	}
	return out.Criteria, nil
}

func (s *Service) GenerateCriteria(ctx context.Context, prompt string) ([]comparison.Criterion, error) {
	c, err := s.newCaller("criteria", s.opts.Settings.Get().JudgeModel, "medium")
	if err != nil {
		return nil, err
	}
	defer c.close()
	return s.writeCriteria(ctx, c, prompt)
}

/* ── Verifier ─────────────────────────────────────────────── */

const verifierPrompt = `You check one AI coding agent's result against acceptance criteria.
For each criterion decide: "met", "partial", "not_met" or "not_verifiable". Say how you know: "ran" when a test or command that ran shows it, "read" when the code in the diff shows it, "none" when it cannot be checked.
Evidence is specific: a file and line from the diff, a test name, or a line of the test or linter output. Never guess: when the facts cannot settle a criterion, mark it not_verifiable and say what would be needed.
Also list anything else the agent claims or the task needs that the facts cannot confirm. Write in British English.`

var verifierSchema = obj(map[string]any{
	"checks": arr(obj(map[string]any{
		"index":    map[string]any{"type": "integer"},
		"status":   enum("met", "partial", "not_met", "not_verifiable"),
		"method":   enum("ran", "read", "none"),
		"evidence": str(),
	})),
	"notVerified": arr(str()),
})

func (s *Service) verify(ctx context.Context, c *caller, v comparison.View, f *sideFacts, crit []comparison.Criterion) ([]CriterionCheck, []string, error) {
	if len(crit) == 0 {
		return []CriterionCheck{}, []string{}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\nAcceptance criteria:\n", v.Prompt)
	for i, cr := range crit {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, importance(cr), cr.Text)
	}
	b.WriteString("\n" + checksText(f))
	fmt.Fprintf(&b, "\nThe agent's last message:\n%s\n", clip(f.session.LastMessage, 3000))
	fmt.Fprintf(&b, "\nFiles changed:\n%s\n\nDiff:\n%s", filesList(f.view.Files), clip(f.diffText, maxDiffChars))
	var out struct {
		Checks []struct {
			Index    int    `json:"index"`
			Status   string `json:"status"`
			Method   string `json:"method"`
			Evidence string `json:"evidence"`
		} `json:"checks"`
		NotVerified []string `json:"notVerified"`
	}
	if err := c.ask(ctx, verifierPrompt, b.String(), "verification", verifierSchema, &out); err != nil {
		return nil, nil, err
	}
	checks := []CriterionCheck{}
	seen := map[int]bool{}
	for _, ch := range out.Checks {
		i := ch.Index - 1
		if i < 0 || i >= len(crit) || seen[i] {
			continue
		}
		seen[i] = true
		checks = append(checks, CriterionCheck{Index: i, Status: ch.Status, Method: ch.Method, Evidence: ch.Evidence})
	}
	unverified := []string{}
	for i := range crit {
		if !seen[i] {
			checks = append(checks, CriterionCheck{Index: i, Status: "not_verifiable", Method: "none", Evidence: "the verifier gave no answer"})
		}
	}
	sort.Slice(checks, func(a, b int) bool { return checks[a].Index < checks[b].Index })
	for _, ch := range checks {
		if ch.Status == "not_verifiable" {
			unverified = append(unverified, fmt.Sprintf("Criterion %d: %s", ch.Index+1, ch.Evidence))
		}
	}
	return checks, append(unverified, out.NotVerified...), nil
}

func importance(c comparison.Criterion) string {
	if c.Required {
		return "required"
	}
	return "desirable"
}

/* ── Reviewer ─────────────────────────────────────────────── */

const reviewerPrompt = `You are a meticulous senior code reviewer. You review one change that an AI coding agent made for a task; you do not know which agent or model made it.
Look for bugs, functional failures, unhandled edge cases, security problems, practices that go against sound principles (single responsibility, duplication, leaky abstractions, error handling), code that is hard to read (names, function size, structure) and weak or missing tests.%s
Each problem needs: a short title; its impact; the location as path:line from the diff; and a severity: "high" (breaks the task or is a security problem), "medium" (a real problem in some cases) or "low" (minor).
Also list the change's real strengths: concrete things done well, each with a location. Only what you can point to; never praise in general terms.
List what you could not review and why (files too long, generated code, behaviour that needs running). Report only what you are confident about. Write in British English.`

var reviewerSchema = obj(map[string]any{
	"problems": arr(obj(map[string]any{
		"severity": enum("high", "medium", "low"),
		"title":    str(),
		"impact":   str(),
		"location": str(),
	})),
	"strengths":   arr(obj(map[string]any{"title": str(), "location": str()})),
	"notReviewed": arr(str()),
})

func (s *Service) review(ctx context.Context, c *caller, v comparison.View, f *sideFacts) (Review, error) {
	empty := Review{Problems: []Finding{}, Strengths: []Strength{}, NotReviewed: []string{}}
	if len(f.view.Files) == 0 {
		return empty, nil
	}
	extra := ""
	if f.view.Tests.LintCommand == "" {
		extra = "\nThe project has no linter, so also report inconsistent style and formatting that a linter would catch, as low problems."
	}
	user := fmt.Sprintf("Task given to the agent:\n%s\n\n%s\nFiles changed:\n%s\n\nDiff:\n%s", v.Prompt, checksText(f), filesList(f.view.Files), clip(f.diffText, maxDiffChars))
	var out Review
	if err := c.ask(ctx, fmt.Sprintf(reviewerPrompt, extra), user, "review", reviewerSchema, &out); err != nil {
		return empty, err
	}
	if out.Problems == nil {
		out.Problems = []Finding{}
	}
	if out.Strengths == nil {
		out.Strengths = []Strength{}
	}
	if out.NotReviewed == nil {
		out.NotReviewed = []string{}
	}
	return out, nil
}

/* ── Analyst ──────────────────────────────────────────────── */

const analystPrompt = `You analyse one run of an AI coding agent on a task, from the facts given: status, metrics, checks, changed files, subagents, the agent's events and the warnings in its logs.
Summarise what happened in at most four short paragraphs of plain text separated by blank lines: what the agent did, how it went, what got in its way, and the outcome.
Separate facts from inferences: state facts plainly and mark inferences ("it seems", "probably"). Do not name the model or the tool. Do not invent anything. Write in British English.`

var analystSchema = obj(map[string]any{"analysis": str()})

func (s *Service) analyse(ctx context.Context, c *caller, v comparison.View, f *sideFacts) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\n", v.Prompt)
	b.WriteString(runFacts(f.view))
	b.WriteString(checksText(f))
	for _, a := range f.session.Subagents {
		fmt.Fprintf(&b, "Subagent %s: %q, %d steps, %.0f s, %s.\n", a.Type, a.Description, a.Steps, a.DurationSec, a.Status)
	}
	if len(f.events) > 0 {
		b.WriteString("\nEvents (from the CLI session):\n")
		for i, e := range f.events {
			if i >= 150 {
				fmt.Fprintf(&b, "… %d more events\n", len(f.events)-i)
				break
			}
			fmt.Fprintf(&b, "%s %s: %s\n", e.At.Format("15:04:05"), e.Kind, e.Detail)
		}
	}
	if len(f.warnings) > 0 {
		b.WriteString("\nWarnings and errors in the logs:\n" + clip(joinLines(f.warnings), 8000) + "\n")
	}
	var out struct {
		Analysis string `json:"analysis"`
	}
	if err := c.ask(ctx, analystPrompt, b.String(), "analysis", analystSchema, &out); err != nil {
		return "", err
	}
	return out.Analysis, nil
}

/* ── Judge ────────────────────────────────────────────────── */

const judgePrompt = `You judge which of two runs of AI coding agents did a task better. The runs are called Side 1 and Side 2; you are not told which agent or model ran each.
You receive, per side: the gates, the acceptance criteria checked with evidence, the score computed by fixed rules with its breakdown, the code review, an analysis of the run and the metrics.
Rules: a side that fails a gate cannot win against one that passes them all. Do not recompute the score. Cite evidence for every claim. Being cheaper or faster never outweighs not doing the task.
If you think the score is wrong, say so as a disagreement with the reason. Give: the winner ("1", "2" or "tie"), your confidence, three to five reasons, whether you would ship each side's change and why, and verdict labels for "Works", "Code quality", "Cheaper", "Faster" and "Overall" (each "1", "2" or "tie"). Write in British English.`

var judgeSchema = obj(map[string]any{
	"winner":        enum("1", "2", "tie"),
	"confidence":    enum("high", "medium", "low"),
	"reasons":       arr(str()),
	"ship":          arr(obj(map[string]any{"side": enum("1", "2"), "yes": map[string]any{"type": "boolean"}, "reason": str()})),
	"labels":        arr(obj(map[string]any{"label": str(), "side": enum("1", "2", "tie")})),
	"disagreements": arr(str()),
})

type judgePass struct {
	Winner     string   `json:"winner"`
	Confidence string   `json:"confidence"`
	Reasons    []string `json:"reasons"`
	Ship       []struct {
		Side   string `json:"side"`
		Yes    bool   `json:"yes"`
		Reason string `json:"reason"`
	} `json:"ship"`
	Labels []struct {
		Label string `json:"label"`
		Side  string `json:"side"`
	} `json:"labels"`
	Disagreements []string `json:"disagreements"`
}

func (s *Service) judge(ctx context.Context, c *caller, v comparison.View, facts map[string]*sideFacts, r Report) (Judgement, error) {
	orders := [][2]string{{"A", "B"}, {"B", "A"}}
	passes := make([]judgePass, 2)
	errs := make([]error, 2)
	done := make(chan int, 2)
	for i, order := range orders {
		go func() {
			var b strings.Builder
			fmt.Fprintf(&b, "Task:\n%s\n\nAcceptance criteria:\n", v.Prompt)
			for k, cr := range r.Criteria {
				fmt.Fprintf(&b, "%d. [%s] %s\n", k+1, importance(cr), cr.Text)
			}
			for n, key := range order {
				fmt.Fprintf(&b, "\n## Side %d\n%s", n+1, judgeFacts(facts[key], r.Sides[key]))
			}
			errs[i] = c.ask(ctx, judgePrompt, b.String(), "judgement", judgeSchema, &passes[i])
			done <- i
		}()
	}
	<-done
	<-done
	if errs[0] != nil {
		return Judgement{}, errs[0]
	}
	if errs[1] != nil {
		return Judgement{}, errs[1]
	}
	toSide := func(order [2]string, s string) string {
		switch s {
		case "1":
			return order[0]
		case "2":
			return order[1]
		}
		return ""
	}
	p := passes[0]
	names := strings.NewReplacer("Side 1", "Side "+orders[0][0], "Side 2", "Side "+orders[0][1], "side 1", "side "+orders[0][0], "side 2", "side "+orders[0][1])
	rename := func(in []string) []string {
		out := make([]string, 0, len(in))
		for _, s := range in {
			out = append(out, names.Replace(s))
		}
		return out
	}
	j := Judgement{
		Winner: toSide(orders[0], p.Winner), Confidence: p.Confidence, Reasons: rename(p.Reasons), Ship: map[string]Ship{},
		Disagreements: rename(p.Disagreements), Labels: []Verdict{},
		Passes: []string{toSide(orders[0], passes[0].Winner), toSide(orders[1], passes[1].Winner)},
	}
	if j.Reasons == nil {
		j.Reasons = []string{}
	}
	j.PassesAgree = j.Passes[0] == j.Passes[1]
	if !j.PassesAgree {
		j.Confidence = "low"
	}
	for _, sh := range p.Ship {
		j.Ship[toSide(orders[0], sh.Side)] = Ship{Yes: sh.Yes, Reason: names.Replace(sh.Reason)}
	}
	for _, l := range p.Labels {
		j.Labels = append(j.Labels, Verdict{Label: l.Label, Side: toSide(orders[0], l.Side)})
	}
	return j, nil
}

func judgeFacts(f *sideFacts, r *SideReport) string {
	var b strings.Builder
	b.WriteString("Gates:\n")
	for _, g := range r.Gates {
		state := "passed"
		if !g.Passed {
			state = "FAILED"
		}
		fmt.Fprintf(&b, "- %s: %s (%s)\n", g.Label, state, g.Reason)
	}
	b.WriteString("Criteria:\n")
	for _, ch := range r.Criteria {
		fmt.Fprintf(&b, "- %d %s via %s: %s\n", ch.Index+1, ch.Status, ch.Method, ch.Evidence)
	}
	fmt.Fprintf(&b, "Score: %s of 100\n", trim(r.Score.Total))
	for _, p := range r.Score.Parts {
		fmt.Fprintf(&b, "- %s %s of %s:", p.Label, trim(p.Points), trim(p.Max))
		for _, l := range p.Lines {
			fmt.Fprintf(&b, " %s %s (%s);", l.Label, trim(l.Points), l.Detail)
		}
		b.WriteString("\n")
	}
	b.WriteString("Review problems:\n")
	for _, p := range r.Review.Problems {
		fmt.Fprintf(&b, "- [%s] %s (%s): %s\n", p.Severity, p.Title, p.Location, p.Impact)
	}
	b.WriteString("Review strengths:\n")
	for _, s := range r.Review.Strengths {
		fmt.Fprintf(&b, "- %s (%s)\n", s.Title, s.Location)
	}
	m := f.view.Metrics
	cost := "unknown"
	if m.CostUSD != nil {
		cost = fmt.Sprintf("$%.4f", *m.CostUSD)
	}
	fmt.Fprintf(&b, "Metrics: cost %s, agent time %.0f s, %d model requests.\n", cost, m.AgentSec, m.Requests)
	fmt.Fprintf(&b, "Analysis:\n%s\n", r.Analysis)
	return b.String()
}

/* ── Harness auditor ──────────────────────────────────────── */

const auditorPrompt = `You audit the harness (the instruction files: AGENTS.md, rules, skills, agents, MCP config…) one AI coding agent ran with, against what happened in the run.
You receive the harness files, what each costs in tokens on every request, the skills the agent loaded, the run's gates, criteria checks, review problems, failed commands and events.
Return:
- strengths: what in the harness helped this run, with evidence from the run (an instruction the agent followed or quoted, a skill it used well);
- gaps: what went wrong that a harness instruction could have prevented, with evidence;
- suggestions: concrete changes, each with a kind, the file to create or change, the change itself, the evidence, and the tokens per request it would save (0 when it adds or saves nothing; never negative).
Kinds: "add_rule" (a new rules file or section), "add_skill", "compact" (same meaning in fewer tokens), "split" (part of a skill into rules, the rest kept as a skill), "move_to_skill" (rules that rarely apply, loaded on demand), "remove" (instructions this kind of task never uses), "other".
With no harness at all, suggest what to create. Base everything on the facts; mark inferences as such. Write in British English.`

var auditorSchema = obj(map[string]any{
	"strengths": arr(obj(map[string]any{"title": str(), "evidence": str()})),
	"gaps":      arr(obj(map[string]any{"title": str(), "evidence": str()})),
	"suggestions": arr(obj(map[string]any{
		"kind":        enum("add_rule", "add_skill", "compact", "split", "move_to_skill", "remove", "other"),
		"file":        str(),
		"change":      str(),
		"evidence":    str(),
		"tokensSaved": map[string]any{"type": "integer"},
	})),
})

func (s *Service) audit(ctx context.Context, c *caller, v comparison.View, f *sideFacts, r *SideReport) (Audit, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\nHarness: %s\n", v.Prompt, f.view.Config.Harness.Label())
	if h := f.harness; h != nil {
		fmt.Fprintf(&b, "Harness tokens per request: %d (sent on %d requests, %.0f %% from the cache).\nFiles by tokens per request:\n", h.PerRequest, h.Requests, h.CacheShare*100)
		for _, p := range h.Files {
			fmt.Fprintf(&b, "- %s: %d\n", p.Label, p.Tokens)
		}
		b.WriteString("Skills listed, by tokens per request:\n")
		for _, p := range h.Skills {
			fmt.Fprintf(&b, "- %s: %d\n", p.Label, p.Tokens)
		}
		b.WriteString("Skills loaded during the run:\n")
		for _, p := range h.SkillsLoaded {
			fmt.Fprintf(&b, "- %s: %d tokens\n", p.Label, p.Tokens)
		}
	}
	b.WriteString("\nHarness files:\n")
	if len(f.harnessFiles) == 0 {
		b.WriteString("(none)\n")
	}
	budget := 40000
	keys := make([]string, 0, len(f.harnessFiles))
	for k := range f.harnessFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, p := range keys {
		text := clip(f.harnessFiles[p], 8000)
		if budget -= len(text); budget < 0 {
			b.WriteString("[… more files left out]\n")
			break
		}
		fmt.Fprintf(&b, "### %s\n%s\n", p, text)
	}
	b.WriteString("\nRun:\n" + judgeFacts(f, r))
	for _, fc := range f.session.FailedCommands {
		fmt.Fprintf(&b, "Failed command (exit %d, fixed later: %t): %s\n", fc.ExitCode, fc.Fixed, clip(fc.Command, 200))
	}
	if len(f.events) > 0 {
		b.WriteString("\nEvents:\n")
		for i, e := range f.events {
			if i >= 120 {
				break
			}
			fmt.Fprintf(&b, "%s: %s\n", e.Kind, e.Detail)
		}
	}
	var out Audit
	if err := c.ask(ctx, auditorPrompt, b.String(), "harness_audit", auditorSchema, &out); err != nil {
		return Audit{}, err
	}
	if out.Strengths == nil {
		out.Strengths = []AuditItem{}
	}
	if out.Gaps == nil {
		out.Gaps = []AuditItem{}
	}
	if out.Suggestions == nil {
		out.Suggestions = []Suggestion{}
	}
	for i := range out.Suggestions {
		out.Suggestions[i].TokensSaved = max(0, out.Suggestions[i].TokensSaved)
	}
	return out, nil
}

/* ── Writer ───────────────────────────────────────────────── */

const writerPrompt = `You write the one-sentence headline of a report comparing two runs of AI coding agents, A and B, from the judgement and scores given. Say who won and the main reason, plainly, in at most 25 words. Write in British English.`

var writerSchema = obj(map[string]any{"headline": str()})

func (s *Service) headline(ctx context.Context, c *caller, v comparison.View, r Report) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n", clip(v.Prompt, 600))
	if j := r.Judge; j != nil {
		fmt.Fprintf(&b, "Winner: %s (confidence %s)\nReasons:\n- %s\n", firstNonEmpty(j.Winner, "tie"), j.Confidence, strings.Join(j.Reasons, "\n- "))
	}
	for _, k := range sides {
		fmt.Fprintf(&b, "Side %s score: %s of 100\n", k, trim(r.Sides[k].Score.Total))
	}
	var out struct {
		Headline string `json:"headline"`
	}
	if err := c.ask(ctx, writerPrompt, b.String(), "headline", writerSchema, &out); err != nil {
		return "", err
	}
	return out.Headline, nil
}

/* ── Facts as text ────────────────────────────────────────── */

func checksText(f *sideFacts) string {
	var b strings.Builder
	t := f.view.Tests
	run := func(label, cmd string, r *comparison.TestRun, out string) {
		if cmd == "" {
			fmt.Fprintf(&b, "%s: none in the project profile.\n", label)
			return
		}
		if r == nil {
			fmt.Fprintf(&b, "%s (%s): did not run.\n", label, cmd)
			return
		}
		fmt.Fprintf(&b, "%s (%s): %s, exit %d.\n", label, cmd, r.Status, r.ExitCode)
		if out != "" {
			fmt.Fprintf(&b, "End of its output:\n%s\n", tail(out, 2500))
		}
	}
	run("Tests", t.Command, t.Visible, f.testOut)
	if t.Hidden != nil {
		run("Tests the agent never saw", t.Command, t.Hidden, f.hiddenOut)
	}
	run("Linter", t.LintCommand, t.Lint, f.lintOut)
	if base := t.Baseline; base != nil {
		if base.Tests != nil {
			fmt.Fprintf(&b, "On the original project, before the agent, the tests %s.\n", base.Tests.Status)
		}
		if base.Lint != nil {
			fmt.Fprintf(&b, "On the original project, before the agent, the linter %s.\n", base.Lint.Status)
		}
	}
	if len(f.newTests) > 0 {
		fmt.Fprintf(&b, "Test files added or changed: %s.\n", strings.Join(f.newTests, ", "))
	}
	return b.String()
}

func runFacts(sv comparison.SideView) string {
	var b strings.Builder
	m := sv.Metrics
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

/* ── JSON schema helpers (strict structured outputs) ─────── */

func str() map[string]any { return map[string]any{"type": "string"} }

func enum(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func arr(items map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": items}
}

func obj(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	sort.Strings(required)
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
