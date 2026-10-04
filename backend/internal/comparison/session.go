package comparison

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-compare/backend/internal/proxy"
)

type SessionFacts struct {
	Ready            bool
	Steps            []Step
	Subagents        []Subagent
	Tools            map[string]int
	ToolCalls        int
	ToolFailures     int
	ReasoningSteps   int
	ReasoningTokens  int64
	FailedCommands   []FailedCommand
	LastMessage      string
	EndsWithQuestion bool
	FirstEditSec     *float64
	Skills           []SkillUse
}

type Step struct {
	At         time.Time
	Agent      string
	Model      string
	Input      int64
	CacheRead  int64
	CacheWrite int64
	Output     int64
	Reasoning  int64
	CostUSD    float64
}

type Subagent struct {
	Type        string
	Description string
	Model       string
	Status      string
	StartedAt   time.Time
	DurationSec float64
	Steps       int
	Tokens      int64
	CostUSD     float64
	Tools       map[string]int
}

type FailedCommand struct {
	At       time.Time
	Agent    string
	Command  string
	ExitCode int
	Fixed    bool
}

type SkillUse struct {
	Name  string
	Chars int
}

var editTools = map[string]bool{"edit": true, "write": true, "apply_patch": true, "multiedit": true, "patch": true}

type taskMeta struct {
	SessionID string `json:"sessionId"`
	Model     struct {
		ModelID string `json:"modelID"`
	} `json:"model"`
}

type taskInput struct {
	Description  string `json:"description"`
	SubagentType string `json:"subagent_type"`
}

func subagentTypes(exports []ocExport) map[string]string {
	out := map[string]string{}
	for _, ex := range exports {
		for _, m := range ex.Messages {
			for _, p := range m.Parts {
				if p.Type != "tool" || p.Tool != "task" || p.State == nil {
					continue
				}
				var meta taskMeta
				var in taskInput
				json.Unmarshal(p.State.Metadata, &meta)
				json.Unmarshal(p.State.Input, &in)
				if meta.SessionID != "" {
					out[meta.SessionID] = firstNonEmpty(in.SubagentType, "subagent")
				}
			}
		}
	}
	return out
}

func (s *Service) SessionFacts(id, key string) (SessionFacts, error) {
	if _, _, err := s.side(id, key); err != nil {
		return SessionFacts{}, err
	}
	path := filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "session.json")
	if _, err := readTimeline(path); err != nil {
		s.recollect(id, key)
	}
	return readSessionFacts(path)
}

func readSessionFacts(path string) (SessionFacts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionFacts{Tools: map[string]int{}}, nil
	}
	var exports []ocExport
	if err := json.Unmarshal(data, &exports); err != nil {
		return SessionFacts{Tools: map[string]int{}}, err
	}
	return sessionFacts(exports), nil
}

type bashRun struct {
	at      time.Time
	agent   string
	command string
	exit    int
}

func sessionFacts(exports []ocExport) SessionFacts {
	f := SessionFacts{Ready: true, Tools: map[string]int{}}
	types := subagentTypes(exports)
	byID := map[string]*Subagent{}
	var start, firstEdit, lastAt time.Time
	var runs []bashRun

	for _, ex := range exports {
		agent := types[ex.Info.ID]
		sub := &Subagent{Tools: map[string]int{}}
		if agent != "" {
			byID[ex.Info.ID] = sub
		}
		for _, m := range ex.Messages {
			created := ms(m.Info.Time.Created)
			if agent == "" && m.Info.Role == "user" && (start.IsZero() || created.Before(start)) {
				start = created
			}
			if t := m.Info.Tokens; t != nil && m.Info.Role == "assistant" {
				step := Step{At: created, Agent: agent, Model: m.Info.ModelID, Input: t.Input, CacheRead: t.Cache.Read,
					CacheWrite: t.Cache.Write, Output: t.Output, Reasoning: t.Reasoning}
				if m.Info.Cost != nil {
					step.CostUSD = *m.Info.Cost
				}
				f.Steps = append(f.Steps, step)
				if t.Reasoning > 0 {
					f.ReasoningSteps++
					f.ReasoningTokens += t.Reasoning
				}
				if agent != "" {
					sub.Steps++
					sub.Tokens += t.Input + t.Cache.Read + t.Cache.Write + t.Output + t.Reasoning
					sub.CostUSD += step.CostUSD
					sub.Model = firstNonEmpty(sub.Model, m.Info.ModelID)
				}
			}
			for _, p := range m.Parts {
				if p.Type == "text" && m.Info.Role == "assistant" && agent == "" {
					if text := strings.TrimSpace(p.Text); text != "" && !created.Before(lastAt) {
						lastAt, f.LastMessage = created, text
					}
				}
				if p.Type != "tool" || p.State == nil {
					continue
				}
				at := created
				if p.State.Time != nil && p.State.Time.Start > 0 {
					at = ms(p.State.Time.Start)
				}
				f.Tools[p.Tool]++
				f.ToolCalls++
				if agent != "" {
					sub.Tools[p.Tool]++
				}
				if p.State.Status == "error" {
					f.ToolFailures++
				}
				if editTools[p.Tool] && p.State.Status != "error" && (firstEdit.IsZero() || at.Before(firstEdit)) {
					firstEdit = at
				}
				switch p.Tool {
				case "bash":
					var in struct {
						Command string `json:"command"`
					}
					var meta struct {
						Exit *int `json:"exit"`
					}
					json.Unmarshal(p.State.Input, &in)
					json.Unmarshal(p.State.Metadata, &meta)
					exit := 0
					switch {
					case meta.Exit != nil:
						exit = *meta.Exit
					case p.State.Status == "error":
						exit = -1
					}
					runs = append(runs, bashRun{at: at, agent: agent, command: strings.TrimSpace(in.Command), exit: exit})
				case "skill":
					var in struct {
						Name string `json:"name"`
					}
					json.Unmarshal(p.State.Input, &in)
					f.Skills = append(f.Skills, SkillUse{Name: in.Name, Chars: len(p.State.Output)})
				}
			}
		}
	}

	for _, ex := range exports {
		for _, m := range ex.Messages {
			for _, p := range m.Parts {
				if p.Type != "tool" || p.Tool != "task" || p.State == nil {
					continue
				}
				var meta taskMeta
				var in taskInput
				json.Unmarshal(p.State.Metadata, &meta)
				json.Unmarshal(p.State.Input, &in)
				sub := byID[meta.SessionID]
				if sub == nil {
					sub = &Subagent{Tools: map[string]int{}}
				}
				sub.Type = firstNonEmpty(in.SubagentType, "subagent")
				sub.Description = in.Description
				sub.Model = firstNonEmpty(meta.Model.ModelID, sub.Model)
				sub.Status = p.State.Status
				if t := p.State.Time; t != nil && t.Start > 0 {
					sub.StartedAt = ms(t.Start)
					if t.End > t.Start {
						sub.DurationSec = float64(t.End-t.Start) / 1000
					}
				}
				f.Subagents = append(f.Subagents, *sub)
			}
		}
	}
	sort.SliceStable(f.Subagents, func(i, j int) bool { return f.Subagents[i].StartedAt.Before(f.Subagents[j].StartedAt) })
	sort.SliceStable(f.Steps, func(i, j int) bool { return f.Steps[i].At.Before(f.Steps[j].At) })

	sort.SliceStable(runs, func(i, j int) bool { return runs[i].at.Before(runs[j].at) })
	for i, r := range runs {
		if r.exit == 0 {
			continue
		}
		fc := FailedCommand{At: r.at, Agent: r.agent, Command: r.command, ExitCode: r.exit}
		for _, later := range runs[i+1:] {
			if later.exit == 0 && sameCommand(later.command, r.command) {
				fc.Fixed = true
				break
			}
		}
		f.FailedCommands = append(f.FailedCommands, fc)
	}

	f.EndsWithQuestion = endsWithQuestion(f.LastMessage)
	if !start.IsZero() && !firstEdit.IsZero() && firstEdit.After(start) {
		sec := firstEdit.Sub(start).Seconds()
		f.FirstEditSec = &sec
	}
	return f
}

func sameCommand(a, b string) bool {
	fa, fb := strings.Fields(a), strings.Fields(b)
	n := min(2, len(fa), len(fb))
	if n == 0 {
		return false
	}
	for i := range n {
		if fa[i] != fb[i] {
			return false
		}
	}
	return true
}

func endsWithQuestion(text string) bool {
	text = strings.TrimRight(strings.TrimSpace(text), "*_`) ")
	return strings.HasSuffix(text, "?")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Service) Requests(id, key string) ([]proxy.Request, error) {
	_, sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap := sd.proxySnapshot(); snap != nil {
		return snap.Requests, nil
	}
	return nil, nil
}

func (s *Service) FirstRequest(id, key string) []byte {
	data, _ := os.ReadFile(filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "first-request.json"))
	return data
}

func (s *Service) ArtifactText(id, key, name string) string {
	if strings.ContainsAny(name, `/\`) {
		return ""
	}
	data, _ := os.ReadFile(filepath.Join(s.opts.Workspace.ArtifactDir(id, key), name))
	return string(data)
}
