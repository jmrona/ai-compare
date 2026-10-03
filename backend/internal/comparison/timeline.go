package comparison

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// TimelineEvent is one thing the agent did, read from the CLI's session.
type TimelineEvent struct {
	At time.Time
	// Kind is "prompt", "message", "tool", "patch" or "error".
	Kind   string
	Detail string
}

type Timeline struct {
	Events []TimelineEvent
	// usage and cost are the CLI's own count, to cross-check the proxy.
	usage *Usage
	cost  *float64
	Ready bool
}

func (t Timeline) SessionUsage() *Usage     { return t.usage }
func (t Timeline) SessionCostUSD() *float64 { return t.cost }

// The parts of `opencode export` that matter here.
type ocExport struct {
	Messages []struct {
		Info struct {
			Role string `json:"role"`
			Time struct {
				Created int64 `json:"created"`
			} `json:"time"`
			Tokens *struct {
				Input     int64 `json:"input"`
				Output    int64 `json:"output"`
				Reasoning int64 `json:"reasoning"`
				Cache     struct {
					Read  int64 `json:"read"`
					Write int64 `json:"write"`
				} `json:"cache"`
			} `json:"tokens"`
			Cost *float64 `json:"cost"`
		} `json:"info"`
		Parts []ocPart `json:"parts"`
	} `json:"messages"`
}

type ocPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Tool string `json:"tool"`
	Time *struct {
		Start int64 `json:"start"`
	} `json:"time"`
	State *struct {
		Status string          `json:"status"`
		Input  json.RawMessage `json:"input"`
		Error  string          `json:"error"`
		Time   *struct {
			Start int64 `json:"start"`
		} `json:"time"`
	} `json:"state"`
	Files []string `json:"files"`
}

// readTimeline reads the sessions saved by collect-result.sh (a JSON array of `opencode export`).
func readTimeline(path string) (Timeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Timeline{}, err
	}
	var exports []ocExport
	if err := json.Unmarshal(data, &exports); err != nil {
		return Timeline{}, fmt.Errorf("reading the CLI session: %w", err)
	}
	tl := Timeline{Events: []TimelineEvent{}}
	var usage Usage
	var cw int64
	var cost float64
	counted, costed := false, false
	for _, ex := range exports {
		for _, m := range ex.Messages {
			created := ms(m.Info.Time.Created)
			// A patch part has no time of its own; it belongs right after the tool call that made it.
			lastTool := created
			if t := m.Info.Tokens; t != nil {
				usage.Input += t.Input
				usage.Output += t.Output + t.Reasoning
				usage.CacheRead += t.Cache.Read
				cw += t.Cache.Write
				counted = true
			}
			if m.Info.Cost != nil {
				cost += *m.Info.Cost
				costed = true
			}
			for _, p := range m.Parts {
				at := created
				if p.Time != nil && p.Time.Start > 0 {
					at = ms(p.Time.Start)
				}
				switch p.Type {
				case "text":
					text := strings.TrimSpace(p.Text)
					if text == "" {
						continue
					}
					kind := "message"
					if m.Info.Role == "user" {
						kind = "prompt"
					}
					tl.Events = append(tl.Events, TimelineEvent{At: at, Kind: kind, Detail: clip(text, 400)})
				case "tool":
					if p.State == nil {
						continue
					}
					if p.State.Time != nil && p.State.Time.Start > 0 {
						at = ms(p.State.Time.Start)
					}
					lastTool = at
					detail := p.Tool + ": " + toolSummary(p.Tool, p.State.Input)
					if p.State.Status == "error" {
						tl.Events = append(tl.Events, TimelineEvent{At: at, Kind: "error", Detail: clip(detail+" · "+p.State.Error, 400)})
					} else {
						tl.Events = append(tl.Events, TimelineEvent{At: at, Kind: "tool", Detail: clip(detail, 400)})
					}
				case "patch":
					files := make([]string, 0, len(p.Files))
					for _, f := range p.Files {
						files = append(files, strings.TrimPrefix(f, "/workspace/"))
					}
					if len(files) > 0 {
						tl.Events = append(tl.Events, TimelineEvent{At: lastTool.Add(time.Millisecond), Kind: "patch", Detail: "changed " + strings.Join(files, ", ")})
					}
				}
			}
		}
	}
	sort.SliceStable(tl.Events, func(i, j int) bool { return tl.Events[i].At.Before(tl.Events[j].At) })
	if counted {
		usage.CacheWrite = &cw
		tl.usage = &usage
	}
	if costed {
		tl.cost = &cost
	}
	return tl, nil
}

// toolSummary picks the most telling argument of a tool call.
func toolSummary(tool string, input json.RawMessage) string {
	var in map[string]any
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	for _, k := range []string{"command", "filePath", "path", "pattern", "url", "query", "description"} {
		if v, ok := in[k].(string); ok && v != "" {
			return strings.TrimPrefix(v, "/workspace/")
		}
	}
	if patch, ok := in["patchText"].(string); ok {
		var files []string
		for _, line := range strings.Split(patch, "\n") {
			for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: "} {
				if strings.HasPrefix(line, prefix) {
					files = append(files, strings.TrimPrefix(line, prefix))
				}
			}
		}
		return strings.Join(files, ", ")
	}
	b, _ := json.Marshal(in)
	return string(b)
}

func ms(v int64) time.Time { return time.UnixMilli(v).UTC() }

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
