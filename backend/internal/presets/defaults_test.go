package presets

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestSeedAddsDefaultsOnce(t *testing.T) {
	src := fstest.MapFS{
		"defaults/starter/preset.md":                       {Data: []byte("---\ntitle: Starter\nclis: [opencode]\n---\n")},
		"defaults/starter/project/.opencode/opencode.json": {Data: []byte("{}")},
	}
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	added, err := s.seed(src, "defaults")
	if err != nil || len(added) != 1 {
		t.Fatalf("added %v, err %v", added, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir("starter"), "project", ".opencode", "opencode.json")); err != nil {
		t.Fatalf("hidden folders must be copied: %v", err)
	}
	if p, err := s.Get("starter"); err != nil || p.Title != "Starter" {
		t.Fatalf("preset = %+v, %v", p, err)
	}

	os.RemoveAll(s.Dir("starter"))
	if added, _ := s.seed(src, "defaults"); len(added) != 0 {
		t.Fatalf("a deleted default must not come back, added %v", added)
	}
}

func TestEmbeddedDefaultsAreValidPresets(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	added, err := s.Seed()
	if err != nil || len(added) == 0 {
		t.Fatalf("added %v, err %v", added, err)
	}
	for _, slug := range added {
		p, err := s.Get(slug)
		if err != nil || p.Title == "" || len(p.Files) == 0 {
			t.Errorf("default %s: %+v, %v", slug, p, err)
		}
	}
}
