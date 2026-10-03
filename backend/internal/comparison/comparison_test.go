package comparison

import (
	"testing"
	"time"

	"ai-compare/backend/internal/proxy"
)

func TestHumanWait(t *testing.T) {
	t0 := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	reqs := []proxy.Request{
		{At: at(0), Duration: 10},  // busy 0–10
		{At: at(12), Duration: 5},  // busy 12–17: the 10–12 gap has no input (agent working)
		{At: at(60), Duration: 10}, // busy 60–70: the 17–60 gap ends with the user typing
	}
	inputs := []time.Time{at(55)}
	if got := humanWait(reqs, inputs, t0, at(80)); got != 43 {
		t.Errorf("humanWait = %v, want 43", got)
	}
	if got := humanWait(reqs, nil, t0, at(80)); got != 0 {
		t.Errorf("without input humanWait = %v, want 0", got)
	}
	// Overlapping requests merge into one busy interval.
	overlap := []proxy.Request{{At: at(0), Duration: 10}, {At: at(5), Duration: 20}, {At: at(40), Duration: 1}}
	if got := humanWait(overlap, []time.Time{at(30)}, t0, at(41)); got != 15 {
		t.Errorf("with overlapping requests humanWait = %v, want 15", got)
	}
}

func TestParseDiff(t *testing.T) {
	d := parseDiff(`diff --git a/poem.md b/poem.md
new file mode 100644
index 0000000..3bed4c5
--- /dev/null
+++ b/poem.md
@@ -0,0 +1,2 @@
+## Invoices
+Paper trails
diff --git a/src/a.ts b/src/a.ts
index 1..2 100644
--- a/src/a.ts
+++ b/src/a.ts
@@ -1,2 +1,2 @@
 keep
-old
+new
`)
	want := []DiffLine{
		{"file", "poem.md"}, {"@@", "@@ -0,0 +1,2 @@"}, {"+", "+## Invoices"}, {"+", "+Paper trails"},
		{"file", "src/a.ts"}, {"@@", "@@ -1,2 +1,2 @@"}, {" ", " keep"}, {"-", "-old"}, {"+", "+new"},
	}
	if len(d.Lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(d.Lines), len(want), d.Lines)
	}
	for i := range want {
		if d.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, d.Lines[i], want[i])
		}
	}
}

func TestParseDiffLeavesOutDependencies(t *testing.T) {
	d := parseDiff("diff --git a/node_modules/x/index.js b/node_modules/x/index.js\n@@ -0,0 +1 @@\n+junk\n" +
		"diff --git a/main.ts b/main.ts\n@@ -0,0 +1 @@\n+real\n")
	if len(d.Lines) != 3 || d.Lines[0] != (DiffLine{"file", "main.ts"}) || d.Lines[2] != (DiffLine{"+", "+real"}) {
		t.Errorf("lines = %+v", d.Lines)
	}
	files, n := withoutDependencies([]FileChange{{"node_modules/x/a.js", 1, 0}, {"src/.venv/b.py", 1, 0}, {"main.ts", 1, 0}})
	if n != 2 || len(files) != 1 || files[0].Path != "main.ts" {
		t.Errorf("withoutDependencies = %+v, %d", files, n)
	}
}

func TestParseNumstat(t *testing.T) {
	got := parseNumstat("3\t1\tsrc/a.ts\n-\t-\timage.png\n\n")
	if len(got) != 2 || got[0] != (FileChange{"src/a.ts", 3, 1}) || got[1] != (FileChange{"image.png", 0, 0}) {
		t.Errorf("parseNumstat = %+v", got)
	}
}

func TestReadTimeline(t *testing.T) {
	tl, err := readTimeline("testdata/session.json")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range tl.Events {
		kinds[e.Kind]++
	}
	if kinds["prompt"] != 1 || kinds["tool"] != 1 || kinds["patch"] != 1 || kinds["message"] < 1 {
		t.Errorf("event kinds = %v", kinds)
	}
	for i := 1; i < len(tl.Events); i++ {
		if tl.Events[i].At.Before(tl.Events[i-1].At) {
			t.Errorf("events are not in time order at %d", i)
		}
	}
	u := tl.SessionUsage()
	if u == nil || u.Input != 278+495 || u.Output != 184+14+17 || u.CacheRead != 5632*2 {
		t.Errorf("session usage = %+v", u)
	}
	if c := tl.SessionCostUSD(); c == nil || *c < 0.00064 || *c > 0.00065 {
		t.Errorf("session cost = %v", c)
	}
}
