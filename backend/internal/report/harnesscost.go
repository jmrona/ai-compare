package report

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/proxy"
)

const (
	minAgentPrompt     = 2000
	instructionsMarker = "Instructions from: "
	skillsIntro        = "Skills provide specialized instructions"
	skillsEnd          = "</available_skills>"
)

var skillBlock = regexp.MustCompile(`(?s)<skill>\s*<name>([^<]+)</name>.*?</skill>`)

type promptParts struct {
	system string
	tools  int
	prompt int
}

func splitRequest(body []byte) (promptParts, bool) {
	var req struct {
		Instructions string            `json:"instructions"`
		System       json.RawMessage   `json:"system"`
		Input        []json.RawMessage `json:"input"`
		Messages     []json.RawMessage `json:"messages"`
		Tools        json.RawMessage   `json:"tools"`
	}
	if json.Unmarshal(body, &req) != nil || len(req.Tools) == 0 {
		return promptParts{}, false
	}
	p := promptParts{tools: len(req.Tools), system: req.Instructions + textOf(req.System)}
	for _, raw := range append(req.Input, req.Messages...) {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		switch m.Role {
		case "system", "developer":
			p.system += textOf(m.Content)
		default:
			p.prompt += len(textOf(m.Content))
		}
	}
	return p, true
}

func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

func harnessCost(body []byte, requests []proxy.Request, session comparison.SessionFacts, sv comparison.SideView) *HarnessCost {
	parts, ok := splitRequest(body)
	if !ok {
		return nil
	}
	flagged := false
	for _, r := range requests {
		flagged = flagged || r.Tools
	}
	var first *proxy.Request
	var withTools []proxy.Request
	for i, r := range requests {
		agent := r.Tools || (!flagged && r.Usage.PromptTokens() >= minAgentPrompt)
		if agent && r.Usage.Reported {
			if first == nil {
				first = &requests[i]
			}
			withTools = append(withTools, r)
		}
	}
	if first == nil {
		return nil
	}
	sys := parts.system
	instrAt := strings.Index(sys, instructionsMarker)
	skillsAt := strings.Index(sys, skillsIntro)
	skillsStop := strings.Index(sys, skillsEnd)
	if skillsStop >= 0 {
		skillsStop += len(skillsEnd)
	}
	cliEnd := len(sys)
	for _, i := range []int{instrAt, skillsAt} {
		if i >= 0 && i < cliEnd {
			cliEnd = i
		}
	}
	instructions, skills := "", ""
	if instrAt >= 0 {
		end := len(sys)
		if skillsAt > instrAt {
			end = skillsAt
		}
		instructions = sys[instrAt:end]
	}
	if skillsAt >= 0 {
		end := len(sys)
		if skillsStop > skillsAt {
			end = skillsStop
		}
		skills = sys[skillsAt:end]
	}
	totalChars := len(sys) + parts.tools + parts.prompt
	if totalChars == 0 {
		return nil
	}
	tokensFirst := first.Usage.PromptTokens()
	perChar := float64(tokensFirst) / float64(totalChars)
	tok := func(chars int) int64 { return int64(float64(chars)*perChar + 0.5) }

	h := &HarnessCost{
		FirstRequestTokens: tokensFirst,
		Parts: []CostPart{
			{Label: "CLI system prompt", Tokens: tok(cliEnd)},
			{Label: "Tool definitions", Tokens: tok(parts.tools)},
			{Label: "Harness instructions", Tokens: tok(len(instructions))},
			{Label: "Skills list", Tokens: tok(len(skills))},
			{Label: "Prompt", Tokens: tok(parts.prompt)},
		},
		Files: []CostPart{}, Skills: []CostPart{}, SkillsLoaded: []CostPart{},
	}
	for _, block := range strings.Split(instructions, instructionsMarker)[1:] {
		name, _, _ := strings.Cut(block, "\n")
		name = strings.TrimPrefix(strings.TrimSpace(name), "/workspace/")
		h.Files = append(h.Files, CostPart{Label: name, Tokens: tok(len(instructionsMarker) + len(block))})
	}
	for _, m := range skillBlock.FindAllStringSubmatchIndex(skills, -1) {
		h.Skills = append(h.Skills, CostPart{Label: skills[m[2]:m[3]], Tokens: tok(m[1] - m[0])})
	}
	for _, sk := range session.Skills {
		h.SkillsLoaded = append(h.SkillsLoaded, CostPart{Label: sk.Name, Tokens: tok(sk.Chars)})
	}
	byTokens := func(p []CostPart) {
		sort.SliceStable(p, func(i, j int) bool { return p[i].Tokens > p[j].Tokens })
	}
	byTokens(h.Files)
	byTokens(h.Skills)

	h.PerRequest = tok(len(instructions) + len(skills))
	h.Requests = len(withTools)
	h.Total = h.PerRequest * int64(h.Requests)
	var cached, prompt int64
	for _, r := range withTools {
		cached += r.Usage.CacheRead
		prompt += r.Usage.PromptTokens()
	}
	if prompt > 0 {
		h.CacheShare = float64(cached) / float64(prompt)
	}
	if p := sv.PriceSnapshot.Price; p != nil {
		cacheRead := p.Input
		if p.CacheRead != nil {
			cacheRead = *p.CacheRead
		}
		cost := float64(h.Total) * (h.CacheShare*cacheRead + (1-h.CacheShare)*p.Input) / 1_000_000
		h.CostUSD = &cost
		if side := sv.Metrics.CostUSD; side != nil && *side > 0 {
			share := cost / *side
			h.ShareOfSide = &share
		}
	}
	return h
}
