// Package presets stores harness presets on disk: harnesses/<slug>/ with preset.md (frontmatter
// with title, description and CLIs, then free notes), project/ (a literal mirror of what goes to
// the project root) and home/ (what goes to the agent's home folder).
package presets

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

type Preset struct {
	Slug        string
	Title       string
	Description string
	CLIs        []string
	Notes       string
	Files       []File
	UpdatedAt   time.Time
	// Hash is a short hash of every file, recorded by each comparison that uses the preset.
	Hash string
}

type File struct {
	// Root is "project" or "home".
	Root     string
	Path     string
	Size     int64
	Category string
}

var (
	ErrNotFound = errors.New("preset not found")
	roots       = []string{"project", "home"}
	knownCLIs   = []string{"opencode", "codex", "claude"}
)

// MaxFileSize bounds a single preset file: harness files are text.
const MaxFileSize = 1 << 20

type Store struct {
	dir string
	mu  sync.Mutex
}

// New keeps presets under dir (created when missing).
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Dir is the folder of one preset.
func (s *Store) Dir(slug string) string { return filepath.Join(s.dir, slug) }

func (s *Store) List() ([]Preset, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	out := []Preset{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if p, err := s.Get(e.Name()); err == nil {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	return out, nil
}

func (s *Store) Get(slug string) (Preset, error) {
	if !validSlug(slug) {
		return Preset{}, ErrNotFound
	}
	dir := s.Dir(slug)
	meta, err := os.ReadFile(filepath.Join(dir, "preset.md"))
	if errors.Is(err, os.ErrNotExist) {
		return Preset{}, ErrNotFound
	}
	if err != nil {
		return Preset{}, err
	}
	p := parseMeta(string(meta))
	p.Slug = slug
	info, _ := os.Stat(filepath.Join(dir, "preset.md"))
	if info != nil {
		p.UpdatedAt = info.ModTime().UTC()
	}
	h := sha256.New()
	for _, root := range roots {
		base := filepath.Join(dir, root)
		filepath.WalkDir(base, func(p2 string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(base, p2)
			rel = filepath.ToSlash(rel)
			info, err := d.Info()
			if err != nil {
				return nil
			}
			p.Files = append(p.Files, File{Root: root, Path: rel, Size: info.Size(), Category: category(root, rel)})
			if info.ModTime().After(p.UpdatedAt) {
				p.UpdatedAt = info.ModTime().UTC()
			}
			data, _ := os.ReadFile(p2)
			fmt.Fprintf(h, "%s/%s\x00%d\x00", root, rel, len(data))
			h.Write(data)
			return nil
		})
	}
	p.Hash = hex.EncodeToString(h.Sum(nil))[:12]
	return p, nil
}

// Create makes an empty preset with a slug derived from the title.
func (s *Store) Create(title, description string, clis []string) (Preset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	title = strings.TrimSpace(title)
	if title == "" {
		return Preset{}, fmt.Errorf("the preset needs a name")
	}
	slug := s.freeSlug(slugify(title))
	if err := os.MkdirAll(filepath.Join(s.Dir(slug), "project"), 0o755); err != nil {
		return Preset{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.Dir(slug), "home"), 0o755); err != nil {
		return Preset{}, err
	}
	if err := s.writeMeta(slug, Preset{Title: title, Description: description, CLIs: clis}); err != nil {
		return Preset{}, err
	}
	return s.Get(slug)
}

// Update changes the title, description, CLIs and notes.
func (s *Store) Update(slug, title, description string, clis []string, notes string) (Preset, error) {
	if _, err := s.Get(slug); err != nil {
		return Preset{}, err
	}
	if strings.TrimSpace(title) == "" {
		return Preset{}, fmt.Errorf("the preset needs a name")
	}
	if err := s.writeMeta(slug, Preset{Title: strings.TrimSpace(title), Description: description, CLIs: clis, Notes: notes}); err != nil {
		return Preset{}, err
	}
	return s.Get(slug)
}

// Duplicate copies a preset under a new title.
func (s *Store) Duplicate(slug, title string) (Preset, error) {
	src, err := s.Get(slug)
	if err != nil {
		return Preset{}, err
	}
	if strings.TrimSpace(title) == "" {
		title = src.Title + " (copy)"
	}
	dst, err := s.Create(title, src.Description, src.CLIs)
	if err != nil {
		return Preset{}, err
	}
	for _, root := range roots {
		if err := CopyDir(filepath.Join(s.Dir(slug), root), filepath.Join(s.Dir(dst.Slug), root)); err != nil {
			return Preset{}, err
		}
	}
	if _, err := s.Update(dst.Slug, dst.Title, src.Description, src.CLIs, src.Notes); err != nil {
		return Preset{}, err
	}
	return s.Get(dst.Slug)
}

func (s *Store) Delete(slug string) error {
	if _, err := s.Get(slug); err != nil {
		return err
	}
	return os.RemoveAll(s.Dir(slug))
}

func (s *Store) ReadFile(slug, root, rel string) ([]byte, error) {
	p, err := s.filePath(slug, root, rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// WriteFile creates or replaces a file and returns warnings about its content.
func (s *Store) WriteFile(slug, root, rel string, content []byte) ([]string, error) {
	if len(content) > MaxFileSize {
		return nil, fmt.Errorf("%s is larger than %d KB; presets hold harness files, which are text", rel, MaxFileSize>>10)
	}
	p, err := s.filePath(slug, root, rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		return nil, err
	}
	s.touch(slug)
	return secretWarnings(rel, content), nil
}

func (s *Store) DeleteFile(slug, root, rel string) error {
	p, err := s.filePath(slug, root, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	removeEmptyParents(filepath.Dir(p), filepath.Join(s.Dir(slug), root))
	s.touch(slug)
	return nil
}

func (s *Store) MoveFile(slug, root, rel, newRoot, newRel string) error {
	from, err := s.filePath(slug, root, rel)
	if err != nil {
		return err
	}
	to, err := s.filePath(slug, newRoot, newRel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s/%s already exists", newRoot, newRel)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}
	removeEmptyParents(filepath.Dir(from), filepath.Join(s.Dir(slug), root))
	s.touch(slug)
	return nil
}

// AddTree copies a folder into the preset's root, keeping relative paths (imports). Entries
// that cannot be read (broken symbolic links, special files) are skipped.
func (s *Store) AddTree(slug, root, src string) (int, error) {
	if _, err := s.Get(slug); err != nil {
		return 0, err
	}
	if !slices.Contains(roots, root) {
		return 0, fmt.Errorf("unknown root %q", root)
	}
	n := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == src {
				return err
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		if _, err := s.WriteFile(slug, root, filepath.ToSlash(rel), data); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

/* ── Helpers ──────────────────────────────────────────────── */

func (s *Store) filePath(slug, root, rel string) (string, error) {
	if _, err := s.Get(slug); err != nil {
		return "", err
	}
	if !slices.Contains(roots, root) {
		return "", fmt.Errorf("unknown root %q: use project or home", root)
	}
	clean := path.Clean("/" + strings.ReplaceAll(rel, `\`, "/"))
	if clean == "/" || strings.Contains(rel, "..") {
		return "", fmt.Errorf("invalid file path %q", rel)
	}
	return filepath.Join(s.Dir(slug), root, filepath.FromSlash(clean[1:])), nil
}

func (s *Store) touch(slug string) {
	now := time.Now()
	os.Chtimes(filepath.Join(s.Dir(slug), "preset.md"), now, now)
}

func (s *Store) freeSlug(base string) string {
	slug := base
	for i := 2; ; i++ {
		if _, err := os.Stat(s.Dir(slug)); errors.Is(err, os.ErrNotExist) {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func (s *Store) writeMeta(slug string, p Preset) error {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", oneLine(p.Title))
	fmt.Fprintf(&b, "description: %s\n", oneLine(p.Description))
	var clis []string
	for _, c := range p.CLIs {
		if slices.Contains(knownCLIs, c) && !slices.Contains(clis, c) {
			clis = append(clis, c)
		}
	}
	fmt.Fprintf(&b, "clis: [%s]\n", strings.Join(clis, ", "))
	b.WriteString("---\n")
	if notes := strings.TrimSpace(p.Notes); notes != "" {
		b.WriteString("\n" + notes + "\n")
	}
	return os.WriteFile(filepath.Join(s.Dir(slug), "preset.md"), []byte(b.String()), 0o644)
}

// parseMeta reads preset.md: a small frontmatter (title, description, clis) and free notes.
func parseMeta(text string) Preset {
	p := Preset{CLIs: []string{}}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		p.Notes = strings.TrimSpace(text)
		return p
	}
	head, body, _ := strings.Cut(text[4:], "\n---")
	p.Notes = strings.TrimSpace(strings.TrimPrefix(body, "\n"))
	sc := bufio.NewScanner(strings.NewReader(head))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "title":
			p.Title = v
		case "description":
			p.Description = v
		case "clis":
			for _, c := range strings.Split(strings.Trim(v, "[]"), ",") {
				if c = strings.TrimSpace(c); c != "" {
					p.CLIs = append(p.CLIs, c)
				}
			}
		}
	}
	return p
}

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.Trim(slugChars.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if s == "" {
		s = "preset"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func validSlug(s string) bool {
	return s != "" && slugify(s) == s
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// category groups files by what they are, from their path.
func category(root, rel string) string {
	lower := strings.ToLower(rel)
	base := path.Base(lower)
	switch {
	case slices.Contains([]string{"agents.md", "claude.md", "claude.local.md", "gemini.md", ".cursorrules", "copilot-instructions.md"}, base):
		return "instructions"
	case strings.Contains(lower, "/skills/") || strings.HasPrefix(lower, "skills/"):
		return "skills"
	case strings.Contains(lower, "/rules/") || strings.HasPrefix(lower, "rules/"):
		return "rules"
	case strings.Contains(lower, "/agents/") || strings.Contains(lower, "/agent/"):
		return "agents"
	case strings.Contains(lower, "/commands/") || strings.Contains(lower, "/command/"):
		return "commands"
	case base == ".mcp.json" || strings.Contains(lower, "mcp"):
		return "mcp"
	case strings.HasSuffix(base, ".json") || strings.HasSuffix(base, ".jsonc") || strings.HasSuffix(base, ".toml") || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml"):
		return "config"
	}
	_ = root
	return "other"
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`(?i)"(api[_-]?key|token|secret|password)"\s*:\s*"[^"$]{16,}"`),
}

// secretWarnings flags values that look like keys: secrets belong in .env, referenced from MCP config.
func secretWarnings(rel string, content []byte) []string {
	for _, re := range secretPatterns {
		if re.Match(content) {
			return []string{rel + " seems to contain a secret (an API key or token). Keep secrets in .env and reference them as environment variables."}
		}
	}
	return nil
}

func removeEmptyParents(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// CopyDir copies a folder tree; a missing source copies nothing.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
