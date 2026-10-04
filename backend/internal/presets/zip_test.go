package presets

import (
	"archive/zip"
	"bytes"
	"slices"
	"testing"
)

func TestWriteZipKeepsTheLayoutUnderTheTitle(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("Elelem · opencode", "", []string{"opencode"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteFile(p.Slug, "project", ".opencode/opencode.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	name, err := s.WriteZip(p.Slug, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Elelem · opencode.zip" {
		t.Errorf("name = %q", name)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range zr.File {
		got = append(got, f.Name)
	}
	for _, want := range []string{"Elelem · opencode/preset.md", "Elelem · opencode/project/.opencode/opencode.json"} {
		if !slices.Contains(got, want) {
			t.Errorf("zip lacks %q: %v", want, got)
		}
	}
}

func TestZipNameIsSafeForFileSystems(t *testing.T) {
	if got := ZipName(`a/b:c*?"<>|`, "slug"); got != "a-b-c------.zip" {
		t.Errorf("got %q", got)
	}
	if got := ZipName("  ..  ", "fallback"); got != "fallback.zip" {
		t.Errorf("got %q", got)
	}
}
