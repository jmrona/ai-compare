package comparison

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/moby/moby/client"

	"ai-compare/backend/internal/db"
	"ai-compare/backend/internal/proxy"
	"ai-compare/backend/internal/terminal"
)

// Comparisons and their sides are saved to Postgres as they change, so the history survives
// restarts. Without a database (Options.DB nil) they live in memory only.

func (s *Service) insert(ctx context.Context, c *comparison) error {
	if s.opts.DB == nil {
		return nil
	}
	err := s.opts.DB.InsertComparison(ctx, db.InsertComparisonParams{
		ID: c.id, CreatedAt: c.createdAt, ProjectPath: c.projectPath, Prompt: c.prompt, Profile: mustJSON(c.profile),
		SeriesID: c.seriesID, Attempt: int32(c.attempt), SeriesSize: int32(c.seriesSize), Criteria: mustJSON(c.criteria),
	})
	if err != nil {
		return fmt.Errorf("saving the comparison: %w", err)
	}
	for _, k := range []string{"A", "B"} {
		sd := c.sides[k]
		err := s.opts.DB.InsertSide(ctx, db.InsertSideParams{
			ComparisonID: c.id, Side: k, Config: mustJSON(sd.cfg), CliVersion: OpencodeVersion, Status: sd.status, UpdatedAt: c.createdAt,
		})
		if err != nil {
			return fmt.Errorf("saving side %s: %w", k, err)
		}
	}
	return nil
}

// save writes a side's current state. withTerminal also stores its terminal output, which can
// reach 2 MB, so it is only done when the side ends.
func (s *Service) save(sd *side, withTerminal bool) {
	if s.opts.DB == nil {
		return
	}
	// One save at a time per side, so an older state never overwrites a newer one.
	sd.saveMu.Lock()
	defer sd.saveMu.Unlock()

	s.mu.Lock()
	p := db.SaveSideParams{
		ComparisonID: sd.comparisonID, Side: sd.key, Status: sd.status, EndReason: sd.endReason,
		RunStartedAt: timePtr(sd.runStartedAt), EndedAt: timePtr(sd.endedAt), UpdatedAt: time.Now().UTC(),
		Phases: mustJSON(sd.phases), PriceSnapshot: mustJSON(sd.price), ContainerID: sd.containerID, Logs: mustJSON(sd.logs),
		Token: sd.token, Failure: sd.failure, Result: mustJSON(sd.result), Inputs: mustJSON(sd.hub.Inputs()),
	}
	if snap := sd.proxySnapshot(); snap != nil {
		p.Proxy = mustJSON(snap)
	}
	s.mu.Unlock()
	if withTerminal {
		p.Terminal = sd.hub.Output()
	}
	if err := s.opts.DB.SaveSide(context.Background(), p); err != nil {
		s.opts.Log.Warn("could not save a side", "comparison", sd.comparisonID, "side", sd.key, "error", err)
	}
}

// Load reads the saved comparisons after a restart. Sides that were running continue: their
// containers are reattached (or verified, if they ended while api was down). Sides that were
// being prepared cannot resume and end as infrastructure errors. Agent containers that belong to
// no side being followed are stopped.
func (s *Service) Load(ctx context.Context) error {
	if s.opts.DB == nil {
		s.StopOrphans(ctx, nil)
		return nil
	}
	rows, err := s.opts.DB.ListComparisons(ctx)
	if err != nil {
		return fmt.Errorf("reading comparisons: %w", err)
	}
	sideRows, err := s.opts.DB.ListSides(ctx)
	if err != nil {
		return fmt.Errorf("reading comparison sides: %w", err)
	}

	loaded := map[string]*comparison{}
	for _, r := range rows {
		c := &comparison{id: r.ID, createdAt: r.CreatedAt.UTC(), projectPath: r.ProjectPath, prompt: r.Prompt, sides: map[string]*side{},
			reportStatus: r.ReportStatus, report: r.Report, userVerdict: r.UserVerdict, cleanedAt: r.CleanedAt,
			seriesID: r.SeriesID, attempt: int(r.Attempt), seriesSize: int(r.SeriesSize), seriesStopped: r.SeriesStopped}
		json.Unmarshal(r.Profile, &c.profile)
		json.Unmarshal(r.Criteria, &c.criteria)
		loaded[c.id] = c
	}
	type resume struct {
		c     *comparison
		sd    *side
		since time.Time
	}
	var interrupted []*side
	var toResume []resume
	for _, r := range sideRows {
		c := loaded[r.ComparisonID]
		if c == nil {
			continue
		}
		sd := &side{
			key: r.Side, comparisonID: c.id, status: r.Status, endReason: r.EndReason, failure: r.Failure, createdAt: c.createdAt,
			containerID: r.ContainerID, token: r.Token, hub: terminal.NewHub(s.opts.Log),
		}
		json.Unmarshal(r.Config, &sd.cfg)
		json.Unmarshal(r.Phases, &sd.phases)
		json.Unmarshal(r.PriceSnapshot, &sd.price)
		json.Unmarshal(r.Logs, &sd.logs)
		json.Unmarshal(r.Result, &sd.result)
		var inputs []time.Time
		if json.Unmarshal(r.Inputs, &inputs) == nil {
			sd.hub.SetInputs(inputs)
		}
		if r.Proxy != nil {
			var snap proxy.Snapshot
			if json.Unmarshal(r.Proxy, &snap) == nil {
				sd.stored = &snap
			}
		}
		if r.RunStartedAt != nil {
			sd.runStartedAt = *r.RunStartedAt
		}
		if r.EndedAt != nil {
			sd.endedAt = *r.EndedAt
		}
		c.sides[sd.key] = sd

		switch {
		case terminalStatuses[sd.status]:
			sd.hub.Preload(r.Terminal)
			sd.hub.Close()
		case (sd.status == "running" || sd.status == "verifying") && sd.containerID != "":
			sd.hub.Preload(r.Terminal)
			toResume = append(toResume, resume{c, sd, r.UpdatedAt})
		default:
			sd.status, sd.endReason, sd.failure, sd.endedAt = "error", "ai-compare restarted while this side was "+r.Status, "infrastructure", r.UpdatedAt
			sd.token = ""
			sd.hub.Preload(r.Terminal)
			sd.hub.Note("session ended: " + sd.endReason)
			sd.hub.Close()
			interrupted = append(interrupted, sd)
		}
	}

	s.mu.Lock()
	for id, c := range loaded {
		if len(c.sides) == 2 {
			s.all[id] = c
		}
	}
	s.mu.Unlock()
	for _, sd := range interrupted {
		s.save(sd, true)
	}

	followed := map[string]bool{}
	for _, r := range toResume {
		info, err := s.opts.Docker.ContainerInspect(ctx, r.sd.containerID, client.ContainerInspectOptions{})
		switch {
		case err != nil:
			s.fail(r.c, r.sd, "run", fmt.Errorf("its container is gone after a restart: %w", err))
		case r.sd.status == "running" && info.Container.State != nil && info.Container.State.Running && r.sd.token != "":
			followed[r.sd.containerID] = true
			go s.reattach(r.c, r.sd, r.since)
		default:
			// The agent ended while api was down (or verification was interrupted): verify now.
			if r.sd.result.Outcome == "" {
				code := -1
				if info.Container.State != nil {
					code = info.Container.State.ExitCode
				}
				if code == 0 {
					r.sd.result.Outcome, r.sd.result.OutcomeReason = "finished", "The CLI exited (code 0) while ai-compare was restarting"
				} else {
					r.sd.result.Outcome, r.sd.result.OutcomeReason, r.sd.result.OutcomeFailure = "error", fmt.Sprintf("The CLI exited with code %d while ai-compare was restarting", code), "agent"
				}
			}
			if r.sd.result.AgentEndedAt == nil {
				t := r.since
				r.sd.result.AgentEndedAt = &t
			}
			r.sd.hub.Close()
			go s.verify(context.Background(), r.c, r.sd)
		}
	}
	s.StopOrphans(ctx, followed)
	// A series whose last attempt ended while api was down carries on.
	for _, c := range loaded {
		if c.seriesID != "" && len(c.sides) == 2 {
			s.continueSeries(c)
		}
	}
	s.opts.Log.Info("comparisons loaded", "count", len(loaded), "interrupted_sides", len(interrupted), "reattached", len(followed))
	return nil
}

// StopOrphans stops running agent containers that no side follows any more.
func (s *Service) StopOrphans(ctx context.Context, followed map[string]bool) {
	res, err := s.opts.Docker.ContainerList(ctx, client.ContainerListOptions{
		Filters: client.Filters{}.Add("label", "ai-compare.role=agent").Add("status", "running"),
	})
	if err != nil {
		s.opts.Log.Warn("could not list leftover agent containers", "error", err)
		return
	}
	for _, c := range res.Items {
		if followed[c.ID] {
			continue
		}
		s.opts.Log.Info("stopping a leftover agent container", "container", c.ID[:12], "comparison", c.Labels["ai-compare.comparison"])
		s.opts.Docker.ContainerStop(ctx, c.ID, client.ContainerStopOptions{})
	}
}

// proxySnapshot is the live proxy session, or the saved one for a side loaded from the database.
func (sd *side) proxySnapshot() *proxy.Snapshot {
	if sd.session != nil {
		snap := sd.session.Snapshot()
		return &snap
	}
	return sd.stored
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}
