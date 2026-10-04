package comparison

import (
	"encoding/json"
	"testing"
)

const sessionFixture = `[
{"info":{"id":"ses_main","parentID":""},"messages":[
 {"info":{"role":"user","time":{"created":1000}},"parts":[{"type":"text","text":"Do it"}]},
 {"info":{"role":"assistant","modelID":"gpt-5.4-mini","time":{"created":2000},"cost":0.01,
   "tokens":{"input":100,"output":20,"reasoning":30,"cache":{"read":50,"write":0}}},
  "parts":[
   {"type":"tool","tool":"task","state":{"status":"completed","input":{"description":"Explore the database","subagent_type":"explore"},
     "metadata":{"sessionId":"ses_child","model":{"modelID":"gpt-5.4-nano"}},"time":{"start":2100,"end":5100}}},
   {"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"pnpm test"},"metadata":{"exit":1},"time":{"start":6000}}},
   {"type":"tool","tool":"edit","state":{"status":"completed","input":{"filePath":"/workspace/a.ts"},"time":{"start":7000}}},
   {"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"pnpm test -- a"},"metadata":{"exit":0},"time":{"start":8000}}},
   {"type":"tool","tool":"bash","state":{"status":"completed","input":{"command":"pnpm lint"},"metadata":{"exit":2},"time":{"start":9000}}},
   {"type":"tool","tool":"read","state":{"status":"error","input":{"filePath":"/workspace/x"},"time":{"start":9500}}},
   {"type":"tool","tool":"skill","state":{"status":"completed","input":{"name":"tdd"},"output":"12345678","time":{"start":9600}}},
   {"type":"text","text":"Should I also add the admin route?"}
  ]}
]},
{"info":{"id":"ses_child","parentID":"ses_main"},"messages":[
 {"info":{"role":"assistant","modelID":"gpt-5.4-nano","time":{"created":2200},"cost":0.002,
   "tokens":{"input":40,"output":10,"reasoning":0,"cache":{"read":0,"write":0}}},
  "parts":[{"type":"tool","tool":"read","state":{"status":"completed","input":{"filePath":"/workspace/db.ts"},"time":{"start":2300}}}]}
]}
]`

func TestSessionFacts(t *testing.T) {
	var exports []ocExport
	if err := json.Unmarshal([]byte(sessionFixture), &exports); err != nil {
		t.Fatal(err)
	}
	f := sessionFacts(exports)

	if len(f.Subagents) != 1 {
		t.Fatalf("subagents = %d, want 1", len(f.Subagents))
	}
	sub := f.Subagents[0]
	if sub.Type != "explore" || sub.Description != "Explore the database" || sub.Model != "gpt-5.4-nano" {
		t.Errorf("subagent = %+v", sub)
	}
	if sub.DurationSec != 3 || sub.Steps != 1 || sub.Tokens != 50 || sub.CostUSD != 0.002 || sub.Tools["read"] != 1 {
		t.Errorf("subagent figures = %+v", sub)
	}
	if len(f.Steps) != 2 || f.ReasoningSteps != 1 || f.ReasoningTokens != 30 {
		t.Errorf("steps = %d, reasoning steps = %d, tokens = %d", len(f.Steps), f.ReasoningSteps, f.ReasoningTokens)
	}
	if f.ToolCalls != 8 || f.ToolFailures != 1 {
		t.Errorf("tool calls = %d, failures = %d", f.ToolCalls, f.ToolFailures)
	}
	if len(f.FailedCommands) != 2 || !f.FailedCommands[0].Fixed || f.FailedCommands[1].Fixed {
		t.Errorf("failed commands = %+v", f.FailedCommands)
	}
	if !f.EndsWithQuestion {
		t.Error("the last message is a question")
	}
	if f.FirstEditSec == nil || *f.FirstEditSec != 6 {
		t.Errorf("first edit = %v, want 6 s", f.FirstEditSec)
	}
	if len(f.Skills) != 1 || f.Skills[0].Name != "tdd" || f.Skills[0].Chars != 8 {
		t.Errorf("skills = %+v", f.Skills)
	}
}
