package comparison

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"ai-compare/backend/internal/workspace"
)

// maxHarnessFile leaves out large or binary files when reading a harness for the adviser.
const maxHarnessFile = 64 << 10

// MaxHarnessShown is the largest file the Harness tab shows the content of.
const MaxHarnessShown = 512 << 10

// HarnessFile is one file of the harness a side ran with.
type HarnessFile struct {
	Root    string // "project" or "home"
	Path    string
	Size    int64
	Content string
	Omitted bool // binary or too large: Content is empty
}

// HarnessText returns the text of the harness files a side ran with, keyed by path ("home/…" for
// a preset's home files): its preset snapshot, the project's own harness files, or nothing.
func (s *Service) HarnessText(id, key string) (map[string]string, error) {
	files, _, err := s.Harness(id, key, maxHarnessFile)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		if f.Omitted {
			continue
		}
		prefix := ""
		if f.Root == "home" {
			prefix = "home/"
		}
		out[prefix+f.Path] = strings.TrimSpace(f.Content)
	}
	return out, nil
}

// Harness lists the harness files a side ran with, with the content of the text files up to
// limit bytes. available is false when they can no longer be read: a project harness of a
// comparison made before snapshots were kept, whose staging copy retention removed.
func (s *Service) Harness(id, key string, limit int64) (files []HarnessFile, available bool, err error) {
	c, sd, err := s.side(id, key)
	if err != nil {
		return nil, false, err
	}
	dir := s.opts.Workspace.ArtifactDir(id, key)
	switch sd.cfg.Harness.Kind {
	case "preset":
		files = readHarness(filepath.Join(dir, "preset", "project"), "project", nil, limit, files)
		files = readHarness(filepath.Join(dir, "preset", "home"), "home", nil, limit, files)
		return files, true, nil
	case "project":
		if c.projectPath == "" {
			return nil, true, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "harness")); err == nil {
			return readHarness(filepath.Join(dir, "harness"), "project", nil, limit, nil), true, nil
		}
		project := filepath.Join(s.opts.Workspace.StagingDir(), id, "project")
		if _, err := os.Stat(project); err != nil {
			return nil, false, nil
		}
		return readHarness(project, "project", workspace.IsHarnessFile, limit, nil), true, nil
	}
	return nil, true, nil
}

// snapshotProjectHarness keeps the project's own harness files in the artefacts of each side that
// runs with them, so they can still be read once retention has removed the project copy.
func (s *Service) snapshotProjectHarness(c *comparison) {
	src := filepath.Join(s.opts.Workspace.StagingDir(), c.id, "project")
	for k, sd := range c.sides {
		if sd.cfg.Harness.Kind != "project" {
			continue
		}
		dst := filepath.Join(s.opts.Workspace.ArtifactDir(c.id, k), "harness")
		if err := copyHarness(src, dst); err != nil {
			s.opts.Log.Warn("could not keep the project's harness files", "comparison", c.id, "side", k, "error", err)
		}
	}
}

// skipDir leaves out folders that hold no instructions but may be huge.
func skipDir(name string) bool {
	return name == "node_modules" || name == ".git" || name == "worktrees"
}

func copyHarness(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != src && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		if !workspace.IsHarnessFile(rel) || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func readHarness(dir, root string, keep func(string) bool, limit int64, out []HarnessFile) []HarnessFile {
	start := len(out)
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		f := HarnessFile{Root: root, Path: rel, Size: info.Size(), Omitted: true}
		if info.Size() <= limit {
			if data, err := os.ReadFile(p); err == nil && utf8.Valid(data) {
				f.Content, f.Omitted = string(data), false
			}
		}
		out = append(out, f)
		return nil
	})
	added := out[start:]
	sort.Slice(added, func(i, j int) bool { return added[i].Path < added[j].Path })
	return out
}
