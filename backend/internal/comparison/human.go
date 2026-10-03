package comparison

import (
	"sort"
	"time"

	"ai-compare/backend/internal/proxy"
)

// humanWait estimates, for an interactive side, how long the agent was waiting for the user:
// the gaps with no model request in flight that contain user input. A gap without input is the
// agent working locally (running commands, editing files) and stays agent time.
func humanWait(reqs []proxy.Request, inputs []time.Time, runStart, end time.Time) float64 {
	if len(inputs) == 0 || runStart.IsZero() || !end.After(runStart) {
		return 0
	}
	type span struct{ from, to time.Time }
	busy := make([]span, 0, len(reqs))
	for _, r := range reqs {
		busy = append(busy, span{r.At, r.At.Add(time.Duration(r.Duration * float64(time.Second)))})
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i].from.Before(busy[j].from) })
	// Merge overlapping requests into busy intervals.
	var merged []span
	for _, b := range busy {
		if n := len(merged); n > 0 && !b.from.After(merged[n-1].to) {
			if b.to.After(merged[n-1].to) {
				merged[n-1].to = b.to
			}
			continue
		}
		merged = append(merged, b)
	}
	// The gaps between them, from the start of the run to its end.
	var gaps []span
	cursor := runStart
	for _, b := range merged {
		if b.from.After(cursor) {
			gaps = append(gaps, span{cursor, b.from})
		}
		if b.to.After(cursor) {
			cursor = b.to
		}
	}
	if end.After(cursor) {
		gaps = append(gaps, span{cursor, end})
	}
	var total time.Duration
	for _, g := range gaps {
		for _, t := range inputs {
			if !t.Before(g.from) && !t.After(g.to) {
				total += g.to.Sub(g.from)
				break
			}
		}
	}
	return total.Seconds()
}
