package comparison

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/presets"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/terminal"
	"ai-compare/backend/internal/workspace"
)

func (s *Service) run(c *comparison) {
	ctx := context.Background()
	if c.projectPath == "" {
		// No project: both sides start from an empty folder.
		if err := s.opts.Workspace.EmptyProject(c.id); err != nil {
			for _, sd := range c.sides {
				s.fail(c, sd, "copy", err)
			}
			return
		}
		for _, sd := range c.sides {
			s.note(sd, "copy", "info", "no project: both sides start from an empty folder")
		}
	} else if !s.reuseSeriesCopy(c) && !s.copyProject(ctx, c) {
		return
	} else {
		s.snapshotProjectHarness(c)
	}
	s.copyHiddenTests(ctx, c)

	var wg sync.WaitGroup
	for _, sd := range c.sides {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runSide(ctx, c, sd)
		}()
	}
	wg.Wait()
}

// reuseSeriesCopy gives a later attempt of a series attempt 1's project copy (and hidden tests),
// so every attempt starts from exactly the same files. It reports whether it could.
func (s *Service) reuseSeriesCopy(c *comparison) bool {
	if c.seededFrom == "" {
		return false
	}
	start := time.Now()
	staging := s.opts.Workspace.StagingDir()
	src := filepath.Join(staging, c.seededFrom)
	if _, err := os.Stat(filepath.Join(src, "project")); err != nil {
		return false
	}
	for _, dir := range []string{"project", "hidden"} {
		if err := presets.CopyDir(filepath.Join(src, dir), filepath.Join(staging, c.id, dir)); err != nil {
			s.opts.Log.Warn("series: could not reuse the project copy", "error", err)
			return false
		}
	}
	for _, sd := range c.sides {
		s.setPhase(sd, &sd.phases.CopySec, time.Since(start))
		s.note(sd, "copy", "info", fmt.Sprintf("attempt %d of %d: the same project copy as attempt 1 (#%s)", c.attempt, c.seriesSize, c.seededFrom))
	}
	return true
}

// copyProject copies the project once for both sides and reports whether it worked.
func (s *Service) copyProject(ctx context.Context, c *comparison) bool {
	for _, sd := range c.sides {
		s.setStatus(sd, "copying")
		s.note(sd, "copy", "info", "copying the project (read-only): "+c.projectPath)
	}
	copied, err := s.opts.Workspace.CopyProject(ctx, c.projectPath, c.id)
	if err != nil {
		for _, sd := range c.sides {
			s.fail(c, sd, "copy", err)
		}
		return false
	}
	for _, sd := range c.sides {
		s.setPhase(sd, &sd.phases.CopySec, copied.Took)
		s.note(sd, "copy", "info", fmt.Sprintf("%d files · %d KB · %s mode · %d .env files skipped · %s", copied.Files, copied.Kilobytes, copied.Mode, copied.EnvFilesSkipped, copied.Took.Round(time.Millisecond)))
	}
	return true
}

// copyHiddenTests copies the hidden tests next to the project copy, where agents never see them.
// Without them the comparison still runs; only the hidden test run is skipped.
func (s *Service) copyHiddenTests(ctx context.Context, c *comparison) {
	hidden := strings.TrimSpace(c.profile.HiddenTestsPath)
	if hidden == "" {
		return
	}
	if err := s.opts.Workspace.CopyHiddenTests(ctx, hidden, c.id); err != nil {
		s.mu.Lock()
		c.profile.HiddenTestsPath = ""
		s.mu.Unlock()
		for _, sd := range c.sides {
			s.note(sd, "copy", "warn", "the hidden tests could not be copied, so they will not run: "+err.Error())
		}
		return
	}
	for _, sd := range c.sides {
		s.note(sd, "copy", "info", "hidden tests copied (the agents never see them)")
	}
}

// stopped reports whether the user ended a side while it was being prepared.
func (s *Service) stopped(sd *side) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return terminalStatuses[sd.status]
}

func (s *Service) runSide(ctx context.Context, c *comparison, sd *side) {
	cfg := sd.cfg

	// Proxy session with the models.dev price of this moment.
	var price *catalog.Price
	var long *catalog.LongContext
	cat, err := s.opts.Catalog.Get(ctx)
	if err == nil {
		for _, m := range cat.Models {
			if m.Provider == cfg.Provider && m.ID == cfg.Model {
				price, long = m.Price, m.LongContext
			}
		}
	}
	limits := proxy.Limits{}
	if cfg.Limits.MaxTokensK != nil {
		limits.MaxTokens = int64(*cfg.Limits.MaxTokensK * 1000)
	}
	if cfg.Limits.MaxCostUSD != nil {
		limits.MaxCostUSD = *cfg.Limits.MaxCostUSD
	}
	token, session, err := s.opts.Proxy.NewSession(c.id+"-"+sd.key, cfg.Provider, cfg.Model, price, long, limits)
	if err != nil {
		s.fail(c, sd, "proxy", err)
		return
	}
	s.mu.Lock()
	sd.price = PriceSnapshot{Price: price, LongContext: long, FetchedAt: cat.FetchedAt}
	sd.token, sd.session = token, session
	s.mu.Unlock()

	ag, err := opencodeAgent(cfg, c.prompt, fmt.Sprintf("http://api:%d/%s/v1", s.opts.ProxyPort, cfg.Provider), token)
	if err != nil {
		s.opts.Proxy.EndSession(token)
		s.fail(c, sd, "run", err)
		return
	}

	if s.stopped(sd) {
		s.opts.Proxy.EndSession(token)
		return
	}
	s.setStatus(sd, "building")
	s.note(sd, "build", "info", fmt.Sprintf("building the image: %s + opencode %s%s · %s", runtimeOr(c.profile.Runtime), OpencodeVersion, setupNote(c.profile.Setup), harnessChoiceNote(cfg.Harness)))
	buildStart := time.Now()
	opts := workspace.SideImageOptions{
		ComparisonID: c.id, Side: sd.key, Runtime: c.profile.Runtime, Setup: c.profile.Setup,
		CLIInstall: ag.install, HomeFiles: ag.homeFiles,
		WithoutProjectHarness: cfg.Harness.Kind != "project",
	}
	if cfg.Harness.Kind == "preset" {
		opts.PresetDir = s.presetSnapshot(c.id, sd.key)
	}
	built, err := s.opts.Workspace.BuildSideImage(ctx, opts)
	if err != nil {
		s.opts.Proxy.EndSession(token)
		s.note(sd, "build", "error", lastLines(built.Log, 6))
		s.fail(c, sd, "build", err)
		return
	}
	s.setPhase(sd, &sd.phases.BuildSec, time.Since(buildStart))
	s.note(sd, "build", "info", fmt.Sprintf("image %s built in %s", built.Image, built.Took.Round(100*time.Millisecond)))

	if s.stopped(sd) {
		s.opts.Proxy.EndSession(token)
		return
	}
	s.setStatus(sd, "starting")
	startStart := time.Now()
	set := s.opts.Settings.Get()
	// Start with the size of the browser terminal, so the TUI draws for it from its first frame.
	cols, rows := sd.hub.Size(120, 40)
	created, err := s.opts.Docker.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:        built.Image,
			Cmd:          ag.command,
			WorkingDir:   "/workspace",
			Tty:          true,
			OpenStdin:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Env:          append([]string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8"}, ag.env...),
			Labels:       map[string]string{"ai-compare.comparison": c.id, "ai-compare.side": sd.key, "ai-compare.role": "agent"},
		},
		HostConfig: &container.HostConfig{
			ConsoleSize: [2]uint{rows, cols},
			NetworkMode: container.NetworkMode(s.opts.AgentNetwork),
			Resources: container.Resources{
				NanoCPUs: int64(set.CPUs * 1e9),
				Memory:   int64(set.MemoryGB * (1 << 30)),
			},
		},
	})
	if err != nil {
		s.opts.Proxy.EndSession(token)
		s.fail(c, sd, "run", err)
		return
	}
	s.mu.Lock()
	sd.containerID = created.ID
	s.mu.Unlock()

	attached, err := terminal.Attach(ctx, s.opts.Docker, created.ID)
	if err != nil {
		s.opts.Proxy.EndSession(token)
		s.fail(c, sd, "run", err)
		return
	}
	s.connectHub(sd, created.ID, attached, cols, rows)

	if _, err := s.opts.Docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		attached.Close()
		s.opts.Proxy.EndSession(token)
		s.fail(c, sd, "run", err)
		return
	}
	// A viewer may have resized while the container was starting.
	sd.hub.ApplySize()
	s.mu.Lock()
	sd.runStartedAt = time.Now()
	s.mu.Unlock()
	s.setPhase(sd, &sd.phases.StartSec, time.Since(startStart))
	s.setStatus(sd, "running")
	s.note(sd, "run", "info", fmt.Sprintf("container started in %s · %s · %g CPUs · %g GB memory · %s", time.Since(startStart).Round(time.Millisecond), cfg.Mode, set.CPUs, set.MemoryGB, s.opts.AgentNetwork))

	s.follow(ctx, c, sd, created.ID, attached.Close)
}

func harnessChoiceNote(h Harness) string {
	switch h.Kind {
	case "none":
		return "no harness: the project's harness files are left out"
	case "preset":
		return fmt.Sprintf("preset %s (%s) instead of the project's harness files", h.Title, h.Hash)
	}
	return "the project's own harness"
}

// connectHub wires a side's terminal hub to an attached container and starts recording it.
func (s *Service) connectHub(sd *side, containerID string, attached client.ContainerAttachResult, cols, rows uint) {
	sd.hub.Connect(attached.Conn, func(cols, rows uint) {
		s.opts.Docker.ContainerResize(context.Background(), containerID, client.ContainerResizeOptions{Width: cols, Height: rows})
	})
	if rec, err := terminal.NewRecorder(filepath.Join(s.opts.Workspace.ArtifactDir(sd.comparisonID, sd.key), "terminal.cast"), cols, rows, time.Now()); err == nil {
		sd.hub.Record(rec)
	} else {
		s.opts.Log.Warn("terminal recording disabled", "side", sd.key, "error", err)
	}
	go sd.hub.Pump(attached.Reader)
}

// follow waits for a running side's agent to end, enforcing its timeout and limits, then
// verifies the result. It is also where a side reattached after a restart continues.
func (s *Service) follow(ctx context.Context, c *comparison, sd *side, containerID string, detach func()) {
	var runCtx context.Context
	var stop context.CancelFunc
	if t := sd.cfg.Limits.TimeoutMin; t != nil {
		s.mu.Lock()
		deadline := sd.runStartedAt.Add(time.Duration(*t * float64(time.Minute)))
		s.mu.Unlock()
		runCtx, stop = context.WithDeadline(ctx, deadline)
	} else {
		runCtx, stop = context.WithCancel(ctx)
	}
	s.mu.Lock()
	sd.stop = stop
	token := sd.token
	s.mu.Unlock()

	go s.watchLimits(runCtx, sd)

	wait := s.opts.Docker.ContainerWait(ctx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var exit int64 = -1
	var waitErr error
	select {
	case res := <-wait.Result:
		exit = res.StatusCode
	case waitErr = <-wait.Error:
	case <-runCtx.Done():
		// Timeout, a limit, or Finish/Cancel: stop the container and keep the decided outcome.
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			s.requestEnd(sd, "limit_reached", fmt.Sprintf("Timeout of %g min", *sd.cfg.Limits.TimeoutMin), "")
		}
		s.opts.Docker.ContainerStop(context.Background(), containerID, client.ContainerStopOptions{})
	}
	stop()
	detach()
	s.opts.Proxy.EndSession(token)

	switch {
	case waitErr != nil:
		s.requestEnd(sd, "error", "lost track of the container: "+waitErr.Error(), "infrastructure")
	case exit == 0:
		s.requestEnd(sd, "finished", "The CLI exited (code 0)", "")
	default:
		s.requestEnd(sd, "error", fmt.Sprintf("The CLI exited with code %d", exit), "agent")
	}
	now := time.Now()
	s.mu.Lock()
	sd.result.AgentEndedAt = &now
	reason := sd.result.OutcomeReason
	s.mu.Unlock()
	sd.hub.Note("agent ended: " + reason)
	sd.hub.Close()

	s.verify(ctx, c, sd)
}

// watchLimits stops a side once the proxy reports a token or cost limit. It also saves the
// running side every 10 seconds, so a restart loses at most that much of its proxy requests.
func (s *Service) watchLimits(ctx context.Context, sd *side) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for tick := 1; ; tick++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if tick%5 == 0 {
				s.save(sd, false)
			}
			s.mu.Lock()
			session, stop := sd.session, sd.stop
			s.mu.Unlock()
			if session == nil {
				continue
			}
			if hit := session.Snapshot().LimitHit; hit != "" {
				if s.requestEnd(sd, "limit_reached", "The "+hit+" was reached", "") && stop != nil {
					stop()
				}
				return
			}
		}
	}
}

// reattach picks up a side whose container kept running while api restarted: same proxy token,
// the terminal rebuilt from the container's logs, and the recording completed with the output
// produced while api was down.
func (s *Service) reattach(c *comparison, sd *side, since time.Time) {
	ctx := context.Background()
	s.mu.Lock()
	snap, token, containerID, price := sd.stored, sd.token, sd.containerID, sd.price
	s.mu.Unlock()
	if snap == nil {
		snap = &proxy.Snapshot{ID: c.id + "-" + sd.key, Provider: sd.cfg.Provider, Model: sd.cfg.Model}
	}
	session, err := s.opts.Proxy.RestoreSession(token, *snap, price.Price, price.LongContext)
	if err != nil {
		s.fail(c, sd, "run", fmt.Errorf("reattaching after a restart: %w", err))
		return
	}
	s.mu.Lock()
	sd.session, sd.stored = session, nil
	s.mu.Unlock()

	if logs, err := s.containerOutput(ctx, containerID, time.Time{}); err == nil {
		sd.hub.Preload(logs)
	}
	attached, err := terminal.Attach(ctx, s.opts.Docker, containerID)
	if err != nil {
		s.opts.Proxy.EndSession(token)
		s.fail(c, sd, "run", fmt.Errorf("reattaching after a restart: %w", err))
		return
	}
	cols, rows := sd.hub.Size(120, 40)
	s.connectHub(sd, containerID, attached, cols, rows)
	// What the agent printed while api was down goes into the recording in one piece.
	if missed, err := s.containerOutput(ctx, containerID, since); err == nil && len(missed) > 0 {
		sd.hub.Write(missed)
	}
	s.note(sd, "run", "info", "ai-compare restarted; reattached to the running container")
	s.follow(ctx, c, sd, containerID, attached.Close)
}

// containerOutput reads a TTY container's output from Docker's logs, since a moment or from the start.
func (s *Service) containerOutput(ctx context.Context, containerID string, since time.Time) ([]byte, error) {
	opts := client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true}
	if !since.IsZero() {
		opts.Since = fmt.Sprintf("%d", since.Unix())
	}
	rc, err := s.opts.Docker.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// With a TTY the logs are the raw stream, not multiplexed.
	return io.ReadAll(io.LimitReader(rc, 8<<20))
}
