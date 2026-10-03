package comparison

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"ai-compare/backend/internal/workspace"
)

// maxHarnessFile leaves out large or binary files when reading a harness for the adviser.
const maxHarnessFile = 64 << 10

// HarnessText returns the text of the harness files a side ran with, keyed by path ("home/…" for
// a preset's home files): its preset snapshot, the project's own harness files, or nothing.
func (s *Service) HarnessText(id, key string) (map[string]string, error) {
	c, sd, err := s.side(id, key)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	switch sd.cfg.Harness.Kind {
	case "preset":
		dir := filepath.Join(s.opts.Workspace.ArtifactDir(id, key), "preset")
		readText(filepath.Join(dir, "project"), "", nil, out)
		readText(filepath.Join(dir, "home"), "home/", nil, out)
	case "project":
		if c.projectPath != "" {
			readText(filepath.Join(s.opts.Workspace.StagingDir(), id, "project"), "", workspace.IsHarnessFile, out)
		}
	}
	return out, nil
}

func readText(dir, prefix string, keep func(string) bool, out map[string]string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxHarnessFile {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || !utf8.Valid(data) {
			return nil
		}
		out[prefix+rel] = strings.TrimSpace(string(data))
		return nil
	})
}
