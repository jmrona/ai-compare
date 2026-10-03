package comparison

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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

// Load reads the saved comparisons. Sides that were still running belong to a previous api
// process (StopOrphans stops their containers), so they are closed as errors.
func (s *Service) Load(ctx context.Context) error {
	if s.opts.DB == nil {
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
		c := &comparison{id: r.ID, createdAt: r.CreatedAt.UTC(), projectPath: r.ProjectPath, prompt: r.Prompt, sides: map[string]*side{}}
		json.Unmarshal(r.Profile, &c.profile)
		loaded[c.id] = c
	}
	var interrupted []*side
	for _, r := range sideRows {
		c := loaded[r.ComparisonID]
		if c == nil {
			continue
		}
		sd := &side{
			key: r.Side, comparisonID: c.id, status: r.Status, endReason: r.EndReason, createdAt: c.createdAt,
			containerID: r.ContainerID, hub: terminal.NewHub(s.opts.Log),
		}
		json.Unmarshal(r.Config, &sd.cfg)
		json.Unmarshal(r.Phases, &sd.phases)
		json.Unmarshal(r.PriceSnapshot, &sd.price)
		json.Unmarshal(r.Logs, &sd.logs)
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
		sd.hub.Write(r.Terminal)
		if !terminalStatuses[sd.status] {
			sd.status, sd.endReason, sd.endedAt = "error", "ai-compare restarted while this side was "+r.Status, r.UpdatedAt
			sd.hub.Note("session ended: " + sd.endReason)
			interrupted = append(interrupted, sd)
		}
		sd.hub.Close()
		c.sides[sd.key] = sd
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
	s.opts.Log.Info("comparisons loaded", "count", len(loaded), "interrupted_sides", len(interrupted))
	return nil
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
