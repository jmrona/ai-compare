package presets

import (
	"slices"
	"testing"
)

func TestPresetLifecycle(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("Strict backend!", "Rules for APIs", []string{"opencode", "bogus", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "strict-backend" || !slices.Equal(p.CLIs, []string{"opencode", "claude"}) {
		t.Fatalf("created %+v", p)
	}
	if _, err := s.WriteFile(p.Slug, "project", "AGENTS.md", []byte("# Rules")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(p.Slug, "project", ".claude/skills/migrations/SKILL.md", []byte("skill")); err != nil {
		t.Fatal(err)
	}
	warns, err := s.WriteFile(p.Slug, "home", ".codex/config.toml", []byte(`token = "sk-abcdefghijklmnopqrstuvwxyz123"`))
	if err != nil || len(warns) != 1 {
		t.Fatalf("secret warning: %v %v", warns, err)
	}
	if _, err := s.WriteFile(p.Slug, "project", "../escape.md", []byte("x")); err == nil {
		t.Error("a path with .. was accepted")
	}
	got, err := s.Get(p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	cats := map[string]string{}
	for _, f := range got.Files {
		cats[f.Root+"/"+f.Path] = f.Category
	}
	if cats["project/AGENTS.md"] != "instructions" || cats["project/.claude/skills/migrations/SKILL.md"] != "skills" || cats["home/.codex/config.toml"] != "config" {
		t.Errorf("categories = %v", cats)
	}
	hash := got.Hash
	if err := s.MoveFile(p.Slug, "project", "AGENTS.md", "project", "docs/AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	moved, _ := s.Get(p.Slug)
	if moved.Hash == hash {
		t.Error("the hash did not change after a move")
	}
	dup, err := s.Duplicate(p.Slug, "")
	if err != nil || dup.Slug != "strict-backend-copy" || len(dup.Files) != 3 {
		t.Fatalf("duplicate = %+v, %v", dup, err)
	}
	if list, _ := s.List(); len(list) != 2 {
		t.Errorf("list has %d presets", len(list))
	}
	if err := s.Delete(p.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(p.Slug); err != ErrNotFound {
		t.Errorf("after delete: %v", err)
	}
}

func TestParseMeta(t *testing.T) {
	p := parseMeta("---\ntitle: \"A preset\"\ndescription: Short\nclis: [opencode, codex]\n---\n\nNotes here.\n")
	if p.Title != "A preset" || p.Description != "Short" || !slices.Equal(p.CLIs, []string{"opencode", "codex"}) || p.Notes != "Notes here." {
		t.Errorf("parseMeta = %+v", p)
	}
}
