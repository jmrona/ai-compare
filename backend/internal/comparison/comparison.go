// Package comparison runs comparisons: it prepares both sides (copy, image with the CLI, proxy
// session), starts their containers with a TTY, follows them until they end and reports their
// state and metrics.
//
// State lives in memory for now; Postgres comes with the persistence work.
package comparison

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/terminal"
	"ai-compare/backend/internal/workspace"
)

/* ── Types shared with the frontend (see frontend/src/api/types.ts) ── */

type Limits struct {
	TimeoutMin *float64 `json:"timeoutMin"`
	MaxTokensK *float64 `json:"maxTokensK"`
	MaxCostUSD *float64 `json:"maxCostUsd"`
}

type SideConfig struct {
	CLI      string `json:"cli"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Effort   string `json:"effort"`
	Mode     string `json:"mode"`
	Limits   Limits `json:"limits"`
}

type Profile struct {
	Runtime         string `json:"runtime"`
	Setup           string `json:"setup"`
	Test            string `json:"test"`
	HiddenTestsPath string `json:"hiddenTestsPath"`
}

type NewComparison struct {
	ProjectPath string                `json:"projectPath"`
	Profile     Profile               `json:"profile"`
	Prompt      string                `json:"prompt"`
	Sides       map[string]SideConfig `json:"sides"`
}

type Usage struct {
	Input      int64  `json:"input"`
	CacheRead  int64  `json:"cacheRead"`
	CacheWrite *int64 `json:"cacheWrite"`
	Output     int64  `json:"output"`
}

type Metrics struct {
	ElapsedSec   float64  `json:"elapsedSec"`
	AgentSec     float64  `json:"agentSec"`
	HumanWaitSec *float64 `json:"humanWaitSec"`
	PrepSec      float64  `json:"prepSec"`
	// Phases splits PrepSec; each phase is null until it has finished.
	Phases           Phases   `json:"phases"`
	Usage            Usage    `json:"usage"`
	CostUSD          *float64 `json:"costUsd"`
	CostConfirmedUSD *float64 `json:"costConfirmedUsd"`
	Requests         int      `json:"requests"`
	Retries          int      `json:"retries"`
	Errors           int      `json:"errors"`
	TokensPerSec     *float64 `json:"tokensPerSec"`
}

// Phases is how long each preparation step took. Both sides share the copy; their images build in parallel.
type Phases struct {
	CopySec  *float64 `json:"copySec"`
	BuildSec *float64 `json:"buildSec"`
	StartSec *float64 `json:"startSec"`
}

type PriceSnapshot struct {
	Price     *catalog.Price `json:"price"`
	FetchedAt time.Time      `json:"fetchedAt"`
}

type SideView struct {
	Key           string        `json:"key"`
	Config        SideConfig    `json:"config"`
	CLIVersion    string        `json:"cliVersion"`
	Status        string        `json:"status"`
	EndReason     string        `json:"endReason,omitempty"`
	Metrics       Metrics       `json:"metrics"`
	Files         []any         `json:"files"`
	PriceSnapshot PriceSnapshot `json:"priceSnapshot"`
}

type View struct {
	ID          string              `json:"id"`
	CreatedAt   time.Time           `json:"createdAt"`
	ProjectPath string              `json:"projectPath"`
	ProjectName string              `json:"projectName"`
	Prompt      string              `json:"prompt"`
	Harness     string              `json:"harness"`
	Sides       map[string]SideView `json:"sides"`
	Report      string              `json:"report"`
}

type LogEntry struct {
	At      string `json:"at"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

/* ── Internal state ───────────────────────────────────────── */

var terminalStatuses = map[string]bool{"finished": true, "error": true, "cancelled": true, "limit_reached": true}

type side struct {
	key       string
	cfg       SideConfig
	status    string
	endReason string

	createdAt    time.Time
	runStartedAt time.Time
	phases       Phases
	endedAt      time.Time

	containerID string
	token       string
	session     *proxy.Session
	price       PriceSnapshot
	hub         *terminal.Hub
	logs        []LogEntry
	stop        context.CancelFunc
}

type comparison struct {
	id          string
	createdAt   time.Time
	projectPath string
	prompt      string
	sides       map[string]*side
}

type Options struct {
	Docker       *client.Client
	Workspace    *workspace.Service
	Proxy        *proxy.Proxy
	Catalog      *catalog.Service
	AgentNetwork string
	ProxyPort    int
	CPUs         float64
	MemoryGB     float64
	Log          *slog.Logger
}

type Service struct {
	opts Options
	mu   sync.Mutex
	all  map[string]*comparison
}

func New(opts Options) *Service {
	if opts.CPUs == 0 {
		opts.CPUs = 2
	}
	if opts.MemoryGB == 0 {
		opts.MemoryGB = 4
	}
	return &Service{opts: opts, all: map[string]*comparison{}}
}

var ErrNotFound = errors.New("comparison not found")

/* ── Starting ─────────────────────────────────────────────── */

func (s *Service) Start(in NewComparison) (string, error) {
	if strings.TrimSpace(in.Prompt) == "" {
		return "", fmt.Errorf("the prompt is empty")
	}
	if len(in.Sides) != 2 || in.Sides["A"].Model == "" || in.Sides["B"].Model == "" {
		return "", fmt.Errorf("both sides need a model")
	}
	for k, cfg := range in.Sides {
		if cfg.CLI != "opencode" {
			return "", fmt.Errorf("side %s: %s is not supported yet; phase 1 runs opencode", k, cfg.CLI)
		}
		if cfg.Provider != "openai" {
			return "", fmt.Errorf("side %s: provider %s is not supported yet", k, cfg.Provider)
		}
	}

	b := make([]byte, 3)
	rand.Read(b)
	c := &comparison{id: "r" + hex.EncodeToString(b), createdAt: time.Now().UTC(), projectPath: in.ProjectPath, prompt: in.Prompt, sides: map[string]*side{}}
	for _, k := range []string{"A", "B"} {
		c.sides[k] = &side{key: k, cfg: in.Sides[k], status: "pending", createdAt: c.createdAt, hub: terminal.NewHub(s.opts.Log)}
	}
	s.mu.Lock()
	s.all[c.id] = c
	s.mu.Unlock()

	go s.run(c, in.Profile)
	return c.id, nil
}

func (s *Service) run(c *comparison, profile Profile) {
	ctx := context.Background()
	for _, sd := range c.sides {
		s.setStatus(sd, "copying", "")
		s.note(sd, "copy", "info", "copying the project (read-only): "+c.projectPath)
	}
	copied, err := s.opts.Workspace.CopyProject(ctx, c.projectPath, c.id)
	if err != nil {
		for _, sd := range c.sides {
			s.fail(sd, "copy", err)
		}
		return
	}
	for _, sd := range c.sides {
		s.setPhase(&sd.phases.CopySec, copied.Took)
		s.note(sd, "copy", "info", fmt.Sprintf("%d files · %d KB · %s mode · %d .env files skipped · %s", copied.Files, copied.Kilobytes, copied.Mode, copied.EnvFilesSkipped, copied.Took.Round(time.Millisecond)))
	}

	var wg sync.WaitGroup
	for _, sd := range c.sides {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runSide(ctx, c, sd, profile)
		}()
	}
	wg.Wait()
}

func (s *Service) runSide(ctx context.Context, c *comparison, sd *side, profile Profile) {
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
	sd.price = PriceSnapshot{Price: price, FetchedAt: cat.FetchedAt}
	limits := proxy.Limits{}
	if cfg.Limits.MaxTokensK != nil {
		limits.MaxTokens = int64(*cfg.Limits.MaxTokensK * 1000)
	}
	if cfg.Limits.MaxCostUSD != nil {
		limits.MaxCostUSD = *cfg.Limits.MaxCostUSD
	}
	token, session, err := s.opts.Proxy.NewSession(c.id+"-"+sd.key, cfg.Provider, cfg.Model, price, long, limits)
	if err != nil {
		s.fail(sd, "proxy", err)
		return
	}
	sd.token, sd.session = token, session

	ag, err := opencodeAgent(cfg, c.prompt, fmt.Sprintf("http://api:%d/%s/v1", s.opts.ProxyPort, cfg.Provider), token)
	if err != nil {
		s.fail(sd, "run", err)
		return
	}

	s.setStatus(sd, "building", "")
	s.note(sd, "build", "info", fmt.Sprintf("building the image: %s + opencode %s%s", runtimeOr(profile.Runtime), OpencodeVersion, setupNote(profile.Setup)))
	buildStart := time.Now()
	built, err := s.opts.Workspace.BuildSideImage(ctx, workspace.SideImageOptions{
		ComparisonID: c.id, Side: sd.key, Runtime: profile.Runtime, Setup: profile.Setup,
		CLIInstall: ag.install, HomeFiles: ag.homeFiles,
	})
	if err != nil {
		s.note(sd, "build", "error", lastLines(built.Log, 6))
		s.fail(sd, "build", err)
		return
	}
	s.setPhase(&sd.phases.BuildSec, time.Since(buildStart))
	s.note(sd, "build", "info", fmt.Sprintf("image %s built in %s", built.Image, built.Took.Round(100*time.Millisecond)))

	s.setStatus(sd, "starting", "")
	startStart := time.Now()
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
				NanoCPUs: int64(s.opts.CPUs * 1e9),
				Memory:   int64(s.opts.MemoryGB * (1 << 30)),
			},
		},
	})
	if err != nil {
		s.fail(sd, "run", err)
		return
	}
	sd.containerID = created.ID

	attached, err := terminal.Attach(ctx, s.opts.Docker, created.ID)
	if err != nil {
		s.fail(sd, "run", err)
		return
	}
	sd.hub.Connect(attached.Conn, func(cols, rows uint) {
		s.opts.Docker.ContainerResize(context.Background(), created.ID, client.ContainerResizeOptions{Width: cols, Height: rows})
	})
	go sd.hub.Pump(attached.Reader)

	var runCtx context.Context
	var stop context.CancelFunc
	if cfg.Limits.TimeoutMin != nil {
		runCtx, stop = context.WithTimeout(ctx, time.Duration(*cfg.Limits.TimeoutMin*float64(time.Minute)))
	} else {
		runCtx, stop = context.WithCancel(ctx)
	}
	sd.stop = stop
	if _, err := s.opts.Docker.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		stop()
		attached.Close()
		s.fail(sd, "run", err)
		return
	}
	// A viewer may have resized while the container was starting.
	sd.hub.ApplySize()
	sd.runStartedAt = time.Now()
	s.setPhase(&sd.phases.StartSec, time.Since(startStart))
	s.setStatus(sd, "running", "")
	s.note(sd, "run", "info", fmt.Sprintf("container started in %s · %s · %.0f CPUs · %.0f GB memory · %s", time.Since(startStart).Round(time.Millisecond), cfg.Mode, s.opts.CPUs, s.opts.MemoryGB, s.opts.AgentNetwork))

	go s.watchLimits(runCtx, sd)

	wait := s.opts.Docker.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	var exit int64 = -1
	select {
	case res := <-wait.Result:
		exit = res.StatusCode
	case err := <-wait.Error:
		s.note(sd, "run", "error", err.Error())
	case <-runCtx.Done():
		// Timeout, a limit, or Finish/Cancel: stop the container and let the status set by the caller stand.
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			s.setStatus(sd, "limit_reached", fmt.Sprintf("Timeout of %g min", *cfg.Limits.TimeoutMin))
		}
		s.opts.Docker.ContainerStop(context.Background(), created.ID, client.ContainerStopOptions{})
	}
	stop()
	attached.Close()
	s.opts.Proxy.EndSession(token)

	s.mu.Lock()
	ended := terminalStatuses[sd.status]
	s.mu.Unlock()
	if !ended {
		if exit == 0 {
			s.setStatus(sd, "finished", "The CLI exited (code 0)")
		} else {
			s.setStatus(sd, "error", fmt.Sprintf("The CLI exited with code %d", exit))
		}
	}
	sd.hub.Note("session ended: " + sd.endReason)
	sd.hub.Close()
}

// watchLimits stops a side once the proxy reports a token or cost limit.
func (s *Service) watchLimits(ctx context.Context, sd *side) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if hit := sd.session.Snapshot().LimitHit; hit != "" {
				s.setStatus(sd, "limit_reached", "The "+hit+" was reached")
				sd.stop()
				return
			}
		}
	}
}

/* ── Actions ──────────────────────────────────────────────── */

// Finish ends a side as finished (the user is done with it); Cancel ends it as cancelled.
func (s *Service) Finish(id, key string) error {
	return s.end(id, key, "finished", "Finished by the user")
}
func (s *Service) Cancel(id, key string) error {
	return s.end(id, key, "cancelled", "Cancelled by the user")
}

func (s *Service) end(id, key, status, reason string) error {
	sd, err := s.side(id, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	done := terminalStatuses[sd.status]
	stop := sd.stop
	s.mu.Unlock()
	if done {
		return nil
	}
	s.setStatus(sd, status, reason)
	if stop != nil {
		stop()
	}
	return nil
}

func (s *Service) Hub(id, key string) (*terminal.Hub, error) {
	sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	return sd.hub, nil
}

func (s *Service) side(id, key string) (*side, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.all[id]
	if c == nil {
		return nil, ErrNotFound
	}
	sd := c.sides[key]
	if sd == nil {
		return nil, fmt.Errorf("side %q does not exist", key)
	}
	return sd, nil
}

/* ── Views ────────────────────────────────────────────────── */

func (s *Service) Get(id string) (View, error) {
	s.mu.Lock()
	c := s.all[id]
	s.mu.Unlock()
	if c == nil {
		return View{}, ErrNotFound
	}
	return s.view(c), nil
}

// Active returns the newest comparison that still has a side running, if any.
func (s *Service) Active() *View {
	for _, v := range s.List() {
		if !terminalStatuses[v.Sides["A"].Status] || !terminalStatuses[v.Sides["B"].Status] {
			return &v
		}
	}
	return nil
}

// List returns every comparison, newest first.
func (s *Service) List() []View {
	s.mu.Lock()
	cs := make([]*comparison, 0, len(s.all))
	for _, c := range s.all {
		cs = append(cs, c)
	}
	s.mu.Unlock()
	sort.Slice(cs, func(i, j int) bool { return cs[i].createdAt.After(cs[j].createdAt) })
	out := make([]View, 0, len(cs))
	for _, c := range cs {
		out = append(out, s.view(c))
	}
	return out
}

func (s *Service) Logs(id, key string) ([]LogEntry, error) {
	sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	logs := append([]LogEntry(nil), sd.logs...)
	s.mu.Unlock()
	// Proxy requests read like log lines too.
	if sd.session != nil {
		for _, r := range sd.session.Snapshot().Requests {
			level, msg := "info", fmt.Sprintf("%s %s %s %d · %.1f s · in %d · out %d", r.Method, r.Path, r.Model, r.Status, r.Duration, r.Usage.PromptTokens(), r.Usage.Output)
			if r.Error != "" {
				level, msg = "warn", msg+" · "+r.Error
			}
			logs = append(logs, LogEntry{At: r.At.Local().Format("15:04:05"), Level: level, Source: "proxy", Message: msg})
		}
	}
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].At < logs[j].At })
	return logs, nil
}

func (s *Service) view(c *comparison) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := View{
		ID: c.id, CreatedAt: c.createdAt, ProjectPath: c.projectPath, ProjectName: filepath.Base(strings.ReplaceAll(c.projectPath, `\`, "/")),
		Prompt: c.prompt, Harness: "project's harness", Sides: map[string]SideView{}, Report: "none",
	}
	now := time.Now()
	for k, sd := range c.sides {
		end := now
		if !sd.endedAt.IsZero() {
			end = sd.endedAt
		}
		m := Metrics{ElapsedSec: end.Sub(sd.createdAt).Seconds()}
		if !sd.runStartedAt.IsZero() {
			m.AgentSec = end.Sub(sd.runStartedAt).Seconds()
			m.PrepSec = sd.runStartedAt.Sub(sd.createdAt).Seconds()
		}
		m.Phases = sd.phases
		if sd.session != nil {
			snap := sd.session.Snapshot()
			cw := snap.Usage.CacheWrite
			m.Usage = Usage{Input: snap.Usage.Input, CacheRead: snap.Usage.CacheRead, CacheWrite: &cw, Output: snap.Usage.Output}
			m.CostUSD = snap.CostUSD
			m.Requests = snap.RequestCnt
			var out int64
			var secs float64
			for _, r := range snap.Requests {
				if r.Error != "" || r.Status >= 400 {
					m.Errors++
				}
				if r.Usage.Reported && r.Streamed {
					out += r.Usage.Output
					secs += r.Duration
				}
			}
			if secs > 1 {
				tps := float64(out) / secs
				m.TokensPerSec = &tps
			}
		}
		v.Sides[k] = SideView{
			Key: k, Config: sd.cfg, CLIVersion: OpencodeVersion, Status: sd.status, EndReason: sd.endReason,
			Metrics: m, Files: []any{}, PriceSnapshot: sd.price,
		}
	}
	return v
}

/* ── Helpers ──────────────────────────────────────────────── */

func (s *Service) setStatus(sd *side, status, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if terminalStatuses[sd.status] {
		return // the first ending wins (e.g. Finish before the container stops)
	}
	sd.status = status
	if reason != "" {
		sd.endReason = reason
	}
	if terminalStatuses[status] {
		sd.endedAt = time.Now()
	}
}

func (s *Service) setPhase(dst **float64, d time.Duration) {
	sec := d.Seconds()
	s.mu.Lock()
	*dst = &sec
	s.mu.Unlock()
}

func (s *Service) note(sd *side, source, level, msg string) {
	s.mu.Lock()
	sd.logs = append(sd.logs, LogEntry{At: time.Now().Format("15:04:05"), Level: level, Source: source, Message: msg})
	s.mu.Unlock()
	if level != "error" {
		sd.hub.Note(msg)
	}
}

func (s *Service) fail(sd *side, source string, err error) {
	s.note(sd, source, "error", err.Error())
	sd.hub.Write([]byte("\r\n\x1b[91m[ai-compare] " + source + " failed: " + strings.ReplaceAll(err.Error(), "\n", "\r\n") + "\x1b[0m\r\n"))
	s.setStatus(sd, "error", source+" failed: "+firstLine(err.Error()))
	sd.hub.Close()
	s.opts.Log.Warn("side failed", "side", sd.key, "step", source, "error", err)
}

func runtimeOr(r string) string {
	if r == "" {
		return "node:22-bookworm-slim"
	}
	return r
}

func setupNote(setup string) string {
	if strings.TrimSpace(setup) == "" {
		return ""
	}
	return " + " + setup
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// StopOrphans stops agent containers left running by a previous api process. Comparisons
// live in memory for now, so after a restart nothing can follow those containers any more.
func (s *Service) StopOrphans(ctx context.Context) {
	res, err := s.opts.Docker.ContainerList(ctx, client.ContainerListOptions{
		Filters: client.Filters{}.Add("label", "ai-compare.role=agent").Add("status", "running"),
	})
	if err != nil {
		s.opts.Log.Warn("could not list leftover agent containers", "error", err)
		return
	}
	for _, c := range res.Items {
		s.opts.Log.Info("stopping a leftover agent container", "container", c.ID[:12], "comparison", c.Labels["ai-compare.comparison"])
		s.opts.Docker.ContainerStop(ctx, c.ID, client.ContainerStopOptions{})
	}
}

// Download describes a side's workspace export.
type Download struct {
	// Filename is the zip name, after the model that produced the work, e.g. gpt-5.4-mini-r1c5860-A.zip.
	Filename string
	// Tar is the container's /workspace as a tar stream; the caller closes it.
	Tar io.ReadCloser
}

// Workspace exports a side's working folder. It works while the side runs (a snapshot) and after
// it ends, as long as its container exists.
func (s *Service) Workspace(ctx context.Context, id, key string) (Download, error) {
	sd, err := s.side(id, key)
	if err != nil {
		return Download{}, err
	}
	s.mu.Lock()
	containerID, model := sd.containerID, sd.cfg.Model
	s.mu.Unlock()
	if containerID == "" {
		return Download{}, fmt.Errorf("side %s has no container yet", key)
	}
	res, err := s.opts.Docker.CopyFromContainer(ctx, containerID, client.CopyFromContainerOptions{SourcePath: "/workspace/."})
	if err != nil {
		return Download{}, fmt.Errorf("reading side %s's workspace: %w", key, err)
	}
	name := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(model)
	return Download{Filename: fmt.Sprintf("%s-%s-%s.zip", name, id, key), Tar: res.Content}, nil
}
