package comparison

import (
	"context"
	"fmt"
	"time"

	"ai-compare/backend/internal/db"
	"ai-compare/backend/internal/settings"
	"ai-compare/backend/internal/workspace"
)

// CleanUp removes what retention selects from the comparisons that ended more than olderThan
// ago. Only what ai-compare created for those comparisons is removed (see workspace.Remove);
// reports and the history stay. A comparison counts as cleaned once its containers, images and
// staging copy are gone.
func (s *Service) CleanUp(ctx context.Context, olderThan time.Duration, retention settings.Retention) (int, workspace.Removed, error) {
	targets := workspace.Targets{Containers: retention.Containers, Images: retention.Images, Staging: retention.Staging, Artifacts: retention.Artifacts}
	if targets == (workspace.Targets{}) {
		return 0, workspace.Removed{}, nil
	}
	complete := targets.Containers && targets.Images && targets.Staging
	cutoff := time.Now().Add(-olderThan)
	var due []*comparison
	s.mu.Lock()
	for _, c := range s.all {
		if c.cleanedAt != nil && !targets.Artifacts {
			continue
		}
		ended, last := true, time.Time{}
		for _, sd := range c.sides {
			if !terminalStatuses[sd.status] {
				ended = false
			}
			if sd.endedAt.After(last) {
				last = sd.endedAt
			}
		}
		if ended && last.Before(cutoff) {
			due = append(due, c)
		}
	}
	s.mu.Unlock()

	var total workspace.Removed
	var firstErr error
	cleaned := 0
	for _, c := range due {
		removed, err := s.opts.Workspace.Remove(ctx, c.id, targets)
		total.Containers += removed.Containers
		total.Images += removed.Images
		total.Staging += removed.Staging
		total.Artifacts += removed.Artifacts
		if err != nil {
			s.opts.Log.Warn("clean-up incomplete", "comparison", c.id, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if removed != (workspace.Removed{}) {
			cleaned++
		}
		if !complete || c.cleanedAt != nil {
			continue
		}
		now := time.Now().UTC()
		s.mu.Lock()
		c.cleanedAt = &now
		s.mu.Unlock()
		if s.opts.DB != nil {
			s.opts.DB.MarkCleaned(ctx, db.MarkCleanedParams{ID: c.id, CleanedAt: &now})
		}
	}
	if cleaned > 0 {
		s.opts.Log.Info("retention clean-up", "comparisons", cleaned, "containers", total.Containers, "images", total.Images, "staging", total.Staging, "artefacts", total.Artifacts)
	}
	return cleaned, total, firstErr
}

// RetentionLoop applies the retention setting every hour.
func (s *Service) RetentionLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		st := s.opts.Settings.Get()
		s.CleanUp(ctx, time.Duration(st.RetentionDays)*24*time.Hour, st.Retention)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Delete removes a comparison entirely: its Docker objects, staging copy, artefacts, report and
// history entry. A comparison still running cannot be deleted.
func (s *Service) Delete(ctx context.Context, id string) error {
	v, err := s.Get(id)
	if err != nil {
		return err
	}
	if v.Live() {
		return fmt.Errorf("comparison %s is still running; finish or cancel its sides first", id)
	}
	if _, err := s.opts.Workspace.RemoveDockerObjects(ctx, id); err != nil {
		s.opts.Log.Warn("deleting a comparison: some Docker objects remain", "comparison", id, "error", err)
	}
	if err := s.opts.Workspace.RemoveArtifacts(id); err != nil {
		return err
	}
	if s.opts.DB != nil {
		if err := s.opts.DB.DeleteComparison(ctx, id); err != nil {
			return err
		}
	}
	s.mu.Lock()
	delete(s.all, id)
	s.mu.Unlock()
	s.publish(Event{DeletedID: id})
	return nil
}
