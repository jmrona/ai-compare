package workspace

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

//go:embed collect-result.sh
var collectScript string

/* ── After a side ends ──────────────────────────────────── */

// CommitResult turns a side's stopped container into an image, so its result can be inspected
// and tested in fresh containers and survives the container's removal.
func (s *Service) CommitResult(ctx context.Context, containerID, comparisonID, side string) (string, error) {
	tag := fmt.Sprintf("ai-compare/result:%s-%s", strings.ToLower(comparisonID), strings.ToLower(side))
	_, err := s.cli.ContainerCommit(ctx, containerID, client.ContainerCommitOptions{
		Reference: tag,
		Comment:   "ai-compare result of side " + side,
		Changes:   []string{"LABEL " + labelPrefix + "comparison=" + comparisonID, "LABEL " + labelPrefix + "side=" + side, "LABEL " + labelPrefix + "role=result"},
	})
	if err != nil {
		return "", fmt.Errorf("saving the side's container as an image: %w", err)
	}
	return tag, nil
}

// ArtifactDir is where api sees a side's artefacts.
func (s *Service) ArtifactDir(comparisonID, side string) string {
	return filepath.Join(s.opts.ArtifactsDir, comparisonID, side)
}

// CollectResult writes a side's files, diffs and CLI sessions to its artefact folder, running
// collect-result.sh in a container of the result image (as root, without network).
func (s *Service) CollectResult(ctx context.Context, image, comparisonID, side string) error {
	out := "/artifacts/" + comparisonID + "/" + side
	res, err := s.runOnce(ctx, oneOff{
		image:   image,
		user:    "0",
		cmd:     []string{"sh", "-c", strings.ReplaceAll(collectScript, "\r\n", "\n"), "collect", out},
		mounts:  []mount.Mount{{Type: mount.TypeVolume, Source: s.opts.ArtifactsVolume, Target: "/artifacts"}},
		labels:  map[string]string{labelPrefix + "comparison": comparisonID, labelPrefix + "side": side, labelPrefix + "role": "collect"},
		timeout: 5 * time.Minute,
	})
	if err != nil {
		return err
	}
	if res.exitCode != 0 {
		return fmt.Errorf("collecting the result failed (exit %d): %s", res.exitCode, lastLines(res.output, 5))
	}
	return nil
}

// TestOutcome is one run of a side's tests.
type TestOutcome struct {
	// Status is "passed", "failed" or "error" (could not run, or timed out).
	Status   string
	ExitCode int
	Took     time.Duration
	Output   string
}

// RunTests runs command in a fresh container of the result image, as the agent's user and
// without network. With hidden, the hidden tests copied for the comparison are added first.
func (s *Service) RunTests(ctx context.Context, image, comparisonID, side, command string, hidden bool, timeout time.Duration) TestOutcome {
	start := time.Now()
	script := command
	var mounts []mount.Mount
	if hidden {
		mounts = []mount.Mount{{Type: mount.TypeVolume, Source: s.opts.StagingVolume, Target: "/staging", ReadOnly: true}}
		script = "cp -R /staging/" + comparisonID + "/hidden/. /workspace/ && " + command
	}
	role := "test"
	if hidden {
		role = "test-hidden"
	}
	res, err := s.runOnce(ctx, oneOff{
		image:   image,
		cmd:     []string{"sh", "-c", script},
		mounts:  mounts,
		labels:  map[string]string{labelPrefix + "comparison": comparisonID, labelPrefix + "side": side, labelPrefix + "role": role},
		timeout: timeout,
	})
	out := TestOutcome{Took: time.Since(start), Output: res.output, ExitCode: res.exitCode}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		out.Status = "error"
		out.Output += fmt.Sprintf("\n[ai-compare] the tests did not finish within %s and were stopped\n", timeout)
	case err != nil:
		out.Status = "error"
		out.Output += "\n[ai-compare] " + err.Error() + "\n"
	case res.exitCode == 0:
		out.Status = "passed"
	default:
		out.Status = "failed"
	}
	return out
}

type oneOff struct {
	image   string
	user    string
	cmd     []string
	mounts  []mount.Mount
	labels  map[string]string
	timeout time.Duration
}

type oneOffResult struct {
	exitCode int
	// output is stdout and stderr interleaved.
	output string
}

// runOnce runs a container to completion without network and removes it.
func (s *Service) runOnce(ctx context.Context, o oneOff) (oneOffResult, error) {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()
	created, err := s.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      o.image,
			User:       o.user,
			Cmd:        o.cmd,
			WorkingDir: "/workspace",
			Labels:     o.labels,
		},
		HostConfig: &container.HostConfig{Mounts: o.mounts, NetworkMode: "none"},
	})
	if err != nil {
		return oneOffResult{exitCode: -1}, err
	}
	defer s.cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
	if _, err := s.cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return oneOffResult{exitCode: -1}, err
	}
	wait := s.cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	res := oneOffResult{exitCode: -1}
	var waitErr error
	select {
	case r := <-wait.Result:
		res.exitCode = int(r.StatusCode)
	case waitErr = <-wait.Error:
	case <-ctx.Done():
		waitErr = ctx.Err()
	}
	// Read what it printed even after a timeout.
	logCtx, logCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer logCancel()
	if rc, err := s.cli.ContainerLogs(logCtx, created.ID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true}); err == nil {
		var buf bytes.Buffer
		stdcopy.StdCopy(&buf, &buf, rc)
		rc.Close()
		res.output = buf.String()
	}
	return res, waitErr
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

/* ── Clean-up ───────────────────────────────────────────── */

// Removed counts what a clean-up removed.
type Removed struct {
	Containers int
	Images     int
	Staging    int
	Artifacts  int
}

type Targets struct {
	Containers bool
	Images     bool
	Staging    bool
	Artifacts  bool
}

// RemoveDockerObjects removes the containers and images ai-compare created for a comparison,
// found by its labels and image names, and its staging copy. Nothing else on the user's Docker
// is touched. Artefacts are kept.
func (s *Service) RemoveDockerObjects(ctx context.Context, comparisonID string) (Removed, error) {
	return s.Remove(ctx, comparisonID, Targets{Containers: true, Images: true, Staging: true})
}

// Remove deletes what targets selects of a comparison. An image still used by a kept container
// is left in place.
func (s *Service) Remove(ctx context.Context, comparisonID string, targets Targets) (Removed, error) {
	var removed Removed
	var errs []error
	if comparisonID == "" || strings.ContainsAny(comparisonID, `/\.`) {
		return removed, fmt.Errorf("invalid comparison id %q", comparisonID)
	}
	if targets.Containers {
		cs, err := s.cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", labelPrefix+"comparison="+comparisonID)})
		if err != nil {
			return removed, err
		}
		for _, c := range cs.Items {
			if _, err := s.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
				errs = append(errs, err)
			} else {
				removed.Containers++
			}
		}
	}
	if targets.Images {
		id := strings.ToLower(comparisonID)
		for _, repo := range []string{"ai-compare/result", "ai-compare/side"} {
			for _, side := range []string{"a", "b"} {
				_, err := s.cli.ImageRemove(ctx, repo+":"+id+"-"+side, client.ImageRemoveOptions{PruneChildren: true})
				switch msg := strings.ToLower(fmt.Sprint(err)); {
				case err == nil:
					removed.Images++
				case strings.Contains(msg, "no such image"), strings.Contains(msg, "being used"), strings.Contains(msg, "conflict"):
				default:
					errs = append(errs, err)
				}
			}
		}
	}
	if targets.Staging {
		n, err := removeDir(filepath.Join(s.opts.StagingDir, comparisonID))
		removed.Staging += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	if targets.Artifacts {
		n, err := removeDir(filepath.Join(s.opts.ArtifactsDir, comparisonID))
		removed.Artifacts += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return removed, errors.Join(errs...)
}

// leftoverAge keeps folders that a comparison or an import being set up may still be filling.
const leftoverAge = 10 * time.Minute

// RemoveLeftovers removes what ai-compare created that belongs to no known comparison: objects of
// deleted comparisons, stopped helper containers and copy helper images of earlier versions.
func (s *Service) RemoveLeftovers(ctx context.Context, known func(id string) bool, targets Targets) (Removed, error) {
	var removed Removed
	var errs []error
	if targets.Containers {
		cs, err := s.cli.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", labelPrefix+"role")})
		if err != nil {
			return removed, err
		}
		for _, c := range cs.Items {
			id := c.Labels[labelPrefix+"comparison"]
			orphan := id != "" && !known(id)
			idle := id == "" && string(c.State) != "running"
			if !orphan && !idle {
				continue
			}
			if _, err := s.cli.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
				errs = append(errs, err)
			} else {
				removed.Containers++
			}
		}
	}
	if targets.Images {
		current, _ := s.ensureCopierImage(ctx)
		imgs, err := s.cli.ImageList(ctx, client.ImageListOptions{})
		if err != nil {
			return removed, err
		}
		for _, img := range imgs.Items {
			for _, tag := range img.RepoTags {
				repo, version, _ := strings.Cut(tag, ":")
				stale := false
				switch repo {
				case "ai-compare/side", "ai-compare/result":
					if i := strings.LastIndex(version, "-"); i > 0 {
						stale = !known(version[:i])
					}
				case "ai-compare/copier":
					stale = current != "" && tag != current
				}
				if !stale {
					continue
				}
				_, err := s.cli.ImageRemove(ctx, tag, client.ImageRemoveOptions{PruneChildren: true})
				switch msg := strings.ToLower(fmt.Sprint(err)); {
				case err == nil:
					removed.Images++
				case strings.Contains(msg, "no such image"), strings.Contains(msg, "being used"), strings.Contains(msg, "conflict"):
				default:
					errs = append(errs, err)
				}
			}
		}
	}
	sweep := func(dir string, skip func(name string) bool) int {
		entries, _ := os.ReadDir(dir)
		n := 0
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || !e.IsDir() || skip(e.Name()) || time.Since(info.ModTime()) < leftoverAge {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				errs = append(errs, err)
			} else {
				n++
			}
		}
		return n
	}
	if targets.Staging {
		removed.Staging += sweep(s.opts.StagingDir, func(name string) bool { return name == "previews" || known(name) })
	}
	if targets.Artifacts {
		removed.Artifacts += sweep(s.opts.ArtifactsDir, known)
	}
	return removed, errors.Join(errs...)
}

func removeDir(dir string) (int, error) {
	if _, err := os.Stat(dir); err != nil {
		return 0, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return 1, nil
}

// RemoveArtifacts deletes a comparison's artefacts (used when the comparison itself is deleted).
func (s *Service) RemoveArtifacts(comparisonID string) error {
	if comparisonID == "" || strings.ContainsAny(comparisonID, `/\.`) {
		return fmt.Errorf("invalid comparison id %q", comparisonID)
	}
	return os.RemoveAll(filepath.Join(s.opts.ArtifactsDir, comparisonID))
}

// DiskUse is how much disk what ai-compare created takes.
type DiskUse struct {
	Images    int64
	Artifacts int64
	Staging   int64
}

func (s *Service) DiskUsage(ctx context.Context) DiskUse {
	var d DiskUse
	if imgs, err := s.cli.ImageList(ctx, client.ImageListOptions{SharedSize: true, Filters: client.Filters{}.Add("label", labelPrefix+"role")}); err == nil {
		// Each image's own layers, plus the largest shared part (the base image) once: an estimate
		// that does not count the runtime and the CLI layer again for every side.
		var shared int64
		for _, i := range imgs.Items {
			own := i.Size
			if i.SharedSize > 0 {
				own -= i.SharedSize
				shared = max(shared, i.SharedSize)
			}
			d.Images += own
		}
		d.Images += shared
	}
	d.Artifacts = dirSize(s.opts.ArtifactsDir)
	d.Staging = dirSize(s.opts.StagingDir)
	return d
}

func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

/* ── While a side runs ──────────────────────────────────── */

// harnessNames are the harness files at a project root, the same list as the copy, the
// inspection and collect-result.sh.
var harnessNames = []string{"AGENTS.md", "CLAUDE.md", "CLAUDE.local.md", "GEMINI.md", ".claude", ".agents", ".codex", ".opencode", "opencode.json", "opencode.jsonc", ".mcp.json", ".cursor", ".cursorrules"}

// DependencyDirs are folders of installed dependencies and caches (npm install, pip install…).
// They are not the agent's work, so diffs leave them out and only count their files. The same
// list is in collect-result.sh.
var DependencyDirs = []string{"node_modules", "bower_components", ".venv", "venv", "__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".next", ".nuxt", ".turbo", ".parcel-cache", ".cache", ".gradle", ".pnpm-store"}

// InDependencyDir reports whether a repository path is inside one of DependencyDirs.
func InDependencyDir(path string) bool {
	for _, part := range strings.Split(path, "/") {
		if slices.Contains(DependencyDirs, part) {
			return true
		}
	}
	return false
}

// LiveDiff returns the current changes of a running side against its baseline, as unified diff
// and numstat, and how many changed files in dependency folders were left out. It uses a
// temporary Git index, so the agent's own index is not touched.
func (s *Service) LiveDiff(ctx context.Context, containerID string, harness bool) (diff, numstat string, dependencies int, err error) {
	var paths, deps []string
	for _, h := range harnessNames {
		if harness {
			paths = append(paths, ":(top,literal)"+h)
		} else {
			paths = append(paths, ":(top,exclude,literal)"+h)
		}
	}
	if !harness {
		paths = append([]string{"."}, paths...)
		for _, d := range DependencyDirs {
			paths = append(paths, ":(glob,exclude)**/"+d+"/**")
			deps = append(deps, ":(glob)**/"+d+"/**")
		}
	}
	spec := strings.Join(paths, " ")
	count := "echo 0"
	if len(deps) > 0 {
		count = "g diff --cached --name-only baseline -- " + strings.Join(deps, " ") + " | wc -l"
	}
	const sep = "@@ai-compare@@"
	script := `export GIT_INDEX_FILE=/tmp/ai-compare-index
g() { git -c safe.directory='*' -c core.quotepath=off "$@"; }
g read-tree baseline && g add -A >/dev/null 2>&1 &&
g diff --cached --numstat baseline -- ` + spec + ` && echo '` + sep + `' &&
` + count + ` && echo '` + sep + `' &&
g diff --cached baseline -- ` + spec
	out, err := s.exec(ctx, containerID, []string{"sh", "-c", script})
	if err != nil {
		return "", "", 0, err
	}
	parts := strings.SplitN(out, sep+"\n", 3)
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("could not read the changes: %s", lastLines(out, 3))
	}
	dependencies, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	return parts[2], parts[0], dependencies, nil
}

// exec runs a command in a running container and returns its output.
func (s *Service) exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	created, err := s.cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{Cmd: cmd, AttachStdout: true, AttachStderr: true, WorkingDir: "/workspace"})
	if err != nil {
		return "", err
	}
	attached, err := s.cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", err
	}
	defer attached.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, attached.Reader); err != nil {
		return "", err
	}
	return buf.String(), nil
}
