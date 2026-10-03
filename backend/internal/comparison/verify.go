package comparison

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ai-compare/backend/internal/workspace"
)

// testTimeout bounds each test run.
const testTimeout = 10 * time.Minute

// verify runs after a side's agent has ended: it saves the result (files, diffs, CLI session),
// runs the tests in fresh containers and then gives the side its final status.
func (s *Service) verify(ctx context.Context, c *comparison, sd *side) {
	s.setStatus(sd, "verifying")
	start := time.Now()
	s.mu.Lock()
	containerID, outcome := sd.containerID, sd.result.Outcome
	profile := c.profile
	s.mu.Unlock()

	if containerID != "" {
		s.collect(ctx, c, sd, containerID, outcome, profile)
	}
	s.mu.Lock()
	if snap := sd.proxySnapshot(); snap != nil && sd.cfg.Mode == "interactive" && sd.result.AgentEndedAt != nil {
		w := humanWait(snap.Requests, sd.hub.Inputs(), sd.runStartedAt, *sd.result.AgentEndedAt)
		sd.result.HumanWaitSec = &w
	}
	_, err := os.Stat(filepath.Join(s.opts.Workspace.ArtifactDir(c.id, sd.key), "terminal.cast"))
	sd.result.HasRecording = err == nil
	status, reason, failure := sd.result.Outcome, sd.result.OutcomeReason, sd.result.OutcomeFailure
	s.mu.Unlock()
	s.setPhase(sd, &sd.phases.VerifySec, time.Since(start))
	s.finalize(c, sd, status, reason, failure)
}

func (s *Service) collect(ctx context.Context, c *comparison, sd *side, containerID, outcome string, profile Profile) {
	ws := s.opts.Workspace
	s.note(sd, "verify", "info", "saving the result and computing the changes")
	image, err := ws.CommitResult(ctx, containerID, c.id, sd.key)
	if err != nil {
		s.note(sd, "verify", "error", err.Error())
		return
	}
	if err := ws.CollectResult(ctx, image, c.id, sd.key); err != nil {
		s.note(sd, "verify", "error", err.Error())
	}
	dir := ws.ArtifactDir(c.id, sd.key)
	files, _ := readNumstat(filepath.Join(dir, "solution.numstat"))
	files, _ = withoutDependencies(files)
	harness, _ := readNumstat(filepath.Join(dir, "harness.numstat"))
	_, tarErr := os.Stat(filepath.Join(dir, "workspace.tar"))
	tl, tlErr := readTimeline(filepath.Join(dir, "session.json"))

	s.mu.Lock()
	sd.result.Files, sd.result.HarnessFiles = files, harness
	sd.result.HasResult = tarErr == nil
	if tlErr == nil {
		sd.result.SessionUsage, sd.result.SessionCostUSD = tl.usage, tl.cost
	}
	snap := sd.proxySnapshot()
	s.mu.Unlock()
	s.note(sd, "verify", "info", fmt.Sprintf("%d files changed%s", len(files), harnessNote(len(harness))))
	if tlErr == nil && tl.usage != nil && snap != nil {
		s.note(sd, "verify", "info", crossCheck(*tl.usage, snap.Usage.Total()))
	}

	tests := Tests{Command: strings.TrimSpace(profile.Test)}
	switch {
	case tests.Command == "":
		tests.SkippedReason = "no test command in the project profile"
	case outcome == "cancelled":
		tests.SkippedReason = "the side was cancelled"
	default:
		tests.Visible = s.runTests(ctx, c, sd, image, tests.Command, false)
		if strings.TrimSpace(profile.HiddenTestsPath) != "" {
			tests.Hidden = s.runTests(ctx, c, sd, image, tests.Command, true)
		}
	}
	s.mu.Lock()
	sd.result.Tests = tests
	s.mu.Unlock()
	s.changedSide(sd)
}

func (s *Service) runTests(ctx context.Context, c *comparison, sd *side, image, command string, hidden bool) *TestRun {
	which, file := "tests", "tests-visible.log"
	if hidden {
		which, file = "hidden tests", "tests-hidden.log"
	}
	s.note(sd, "verify", "info", "running the "+which+" in a fresh container: "+command)
	out := s.opts.Workspace.RunTests(ctx, image, c.id, sd.key, command, hidden, testTimeout)
	path := filepath.Join(s.opts.Workspace.ArtifactDir(c.id, sd.key), file)
	if err := os.WriteFile(path, []byte(out.Output), 0o644); err != nil {
		s.opts.Log.Warn("could not save the test output", "error", err)
	}
	level := "info"
	if out.Status != "passed" {
		level = "warn"
	}
	s.note(sd, "verify", level, fmt.Sprintf("%s %s (exit %d, %s)", which, out.Status, out.ExitCode, out.Took.Round(100*time.Millisecond)))
	return &TestRun{Status: out.Status, ExitCode: out.ExitCode, DurationSec: out.Took.Seconds()}
}

func harnessNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(" · %d harness files changed", n)
}

// crossCheck compares the CLI's own token count with the proxy's. The proxy also sees requests
// the CLI does not count in its session (such as titles), so it is usually a little higher.
func crossCheck(session Usage, proxyTotal int64) string {
	cw := int64(0)
	if session.CacheWrite != nil {
		cw = *session.CacheWrite
	}
	total := session.Input + session.CacheRead + cw + session.Output
	return fmt.Sprintf("cross-check: the CLI session counts %d tokens, the proxy %d (the proxy figure is used)", total, proxyTotal)
}

// readNumstat parses `git diff --numstat` output. Binary files count as 0 lines.
func readNumstat(path string) ([]FileChange, error) {
	f, err := os.Open(path)
	if err != nil {
		return []FileChange{}, err
	}
	defer f.Close()
	out := []FileChange{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		removed, _ := strconv.Atoi(parts[1])
		out = append(out, FileChange{Path: parts[2], Added: added, Removed: removed})
	}
	return out, sc.Err()
}

/* ── Reading a side's result ─────────────────────────────── */

type DiffLine struct {
	// Kind is "+", "-", " ", "@@" or "file".
	Kind string
	Text string
}

type Diff struct {
	Files     []FileChange
	Lines     []DiffLine
	Truncated bool
	// Dependencies counts changed files in dependency folders (node_modules…), left out.
	Dependencies int
	// Ready is false while the side runs and its live diff could not be read.
	Ready bool
}

const maxDiffLines = 20000

// Diff returns a side's changes: the saved diff once it has ended, the live one while it runs.
// kind is "solution" (harness files excluded) or "harness".
func (s *Service) Diff(ctx context.Context, id, key, kind string) (Diff, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return Diff{}, err
	}
	if kind != "harness" {
		kind = "solution"
	}
	dir := s.opts.Workspace.ArtifactDir(id, key)
	if text, err := os.ReadFile(filepath.Join(dir, kind+".diff")); err == nil {
		files, _ := readNumstat(filepath.Join(dir, kind+".numstat"))
		d := parseDiff(string(text))
		// Results collected before dependency folders were left out still contain them.
		d.Files, d.Dependencies = withoutDependencies(files)
		if n, err := os.ReadFile(filepath.Join(dir, "dependencies.count")); err == nil && kind == "solution" {
			d.Dependencies, _ = strconv.Atoi(strings.TrimSpace(string(n)))
		}
		d.Ready = true
		return d, nil
	}
	s.mu.Lock()
	containerID, status := sd.containerID, sd.status
	s.mu.Unlock()
	if containerID == "" || status != "running" {
		return Diff{Files: []FileChange{}, Lines: []DiffLine{}}, nil
	}
	text, numstat, deps, err := s.opts.Workspace.LiveDiff(ctx, containerID, kind == "harness")
	if err != nil {
		return Diff{}, err
	}
	d := parseDiff(text)
	d.Files = parseNumstat(numstat)
	d.Dependencies = deps
	d.Ready = true
	return d, nil
}

// withoutDependencies drops files in dependency folders and says how many there were.
func withoutDependencies(files []FileChange) ([]FileChange, int) {
	out := make([]FileChange, 0, len(files))
	for _, f := range files {
		if !workspace.InDependencyDir(f.Path) {
			out = append(out, f)
		}
	}
	return out, len(files) - len(out)
}

func parseNumstat(s string) []FileChange {
	out := []FileChange{}
	for _, line := range strings.Split(s, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		a, _ := strconv.Atoi(parts[0])
		r, _ := strconv.Atoi(parts[1])
		out = append(out, FileChange{Path: parts[2], Added: a, Removed: r})
	}
	return out
}

// parseDiff turns a unified diff into display lines, dropping Git's metadata lines.
func parseDiff(text string) Diff {
	d := Diff{Lines: []DiffLine{}}
	skipping := false
	for _, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if len(d.Lines) >= maxDiffLines {
			d.Truncated = true
			break
		}
		if skipping && !strings.HasPrefix(l, "diff --git ") {
			continue
		}
		switch {
		case l == "":
		case strings.HasPrefix(l, "diff --git "):
			// "diff --git a/path b/path" → "path"
			name := l[len("diff --git "):]
			if i := strings.Index(name, " b/"); i >= 0 {
				name = name[i+3:]
			}
			// Dependency folders are not the agent's work (old results may still contain them).
			if skipping = workspace.InDependencyDir(name); skipping {
				continue
			}
			d.Lines = append(d.Lines, DiffLine{Kind: "file", Text: name})
		case strings.HasPrefix(l, "@@"):
			d.Lines = append(d.Lines, DiffLine{Kind: "@@", Text: l})
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"), strings.HasPrefix(l, "index "),
			strings.HasPrefix(l, "new file mode"), strings.HasPrefix(l, "deleted file mode"),
			strings.HasPrefix(l, "similarity index"), strings.HasPrefix(l, "rename "), strings.HasPrefix(l, "old mode"), strings.HasPrefix(l, "new mode"):
		case strings.HasPrefix(l, "Binary files"):
			d.Lines = append(d.Lines, DiffLine{Kind: " ", Text: l})
		case l[0] == '+' || l[0] == '-' || l[0] == ' ':
			d.Lines = append(d.Lines, DiffLine{Kind: l[:1], Text: l})
		default:
			d.Lines = append(d.Lines, DiffLine{Kind: " ", Text: l})
		}
	}
	return d
}

// TestOutput returns a side's test results and their output.
func (s *Service) TestOutput(id, key string) (Tests, string, string, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return Tests{}, "", "", err
	}
	s.mu.Lock()
	tests := sd.result.Tests
	s.mu.Unlock()
	dir := s.opts.Workspace.ArtifactDir(id, key)
	visible, _ := os.ReadFile(filepath.Join(dir, "tests-visible.log"))
	hidden, _ := os.ReadFile(filepath.Join(dir, "tests-hidden.log"))
	return tests, string(visible), string(hidden), nil
}

// Timeline returns the agent's events from its CLI session, once the side has ended.
// recollect saves a side's artefacts again from its result image, once per side and process: it
// repairs a CLI session cut short while it was being collected. It reports whether it ran.
func (s *Service) recollect(id, key string) bool {
	if _, done := s.recollected.LoadOrStore(id+"/"+key, true); done || s.opts.Workspace == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	image := "ai-compare/result:" + strings.ToLower(id) + "-" + strings.ToLower(key)
	if err := s.opts.Workspace.CollectResult(ctx, image, id, key); err != nil {
		s.opts.Log.Warn("could not collect the side's result again", "comparison", id, "side", key, "error", err)
		return false
	}
	s.opts.Log.Info("side's result collected again to repair its CLI session", "comparison", id, "side", key)
	return true
}

func (s *Service) Timeline(id, key string) (Timeline, error) {
	if _, _, err := s.side(id, key); err != nil {
		return Timeline{}, err
	}
	path := filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "session.json")
	tl, err := readTimeline(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) && s.recollect(id, key) {
		tl, err = readTimeline(path)
	}
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.opts.Log.Warn("the CLI session cannot be read", "comparison", id, "side", key, "error", err)
		}
		return Timeline{Events: []TimelineEvent{}}, nil
	}
	tl.Ready = true
	return tl, nil
}
