package presets

import (
	"embed"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed all:defaults
var defaults embed.FS

const seededFile = ".defaults"

func (s *Store) Seed() ([]string, error) {
	return s.seed(defaults, "defaults")
}

func (s *Store) seed(src fs.FS, root string) ([]string, error) {
	record := filepath.Join(s.dir, seededFile)
	data, _ := os.ReadFile(record)
	seeded := strings.Fields(string(data))
	entries, err := fs.ReadDir(src, root)
	if err != nil {
		return nil, err
	}
	var added []string
	for _, e := range entries {
		slug := e.Name()
		if !e.IsDir() || slices.Contains(seeded, slug) {
			continue
		}
		seeded = append(seeded, slug)
		if _, err := os.Stat(s.Dir(slug)); err == nil {
			continue
		}
		if err := copyFS(src, path.Join(root, slug), s.Dir(slug)); err != nil {
			os.RemoveAll(s.Dir(slug))
			return added, err
		}
		added = append(added, slug)
	}
	return added, os.WriteFile(record, []byte(strings.Join(seeded, "\n")+"\n"), 0o644)
}

func copyFS(src fs.FS, from, to string) error {
	return fs.WalkDir(src, from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(to, filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(p, from), "/")))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
