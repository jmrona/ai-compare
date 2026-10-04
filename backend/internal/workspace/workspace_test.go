package workspace

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestSplitHostPath(t *testing.T) {
	cases := []struct{ in, anchor, rest string }{
		{`C:\Users\me\app`, `C:\`, "Users/me/app"},
		{`d:/code/app/`, `d:\`, "code/app"},
		{`C:\`, `C:\`, ""},
		{"/Users/me/app", "/Users", "me/app"},
		{"/home/me/app/", "/home", "me/app"},
		{"/Volumes", "/Volumes", ""},
	}
	for _, c := range cases {
		anchor, rest, err := splitHostPath(c.in)
		if err != nil || anchor != c.anchor || rest != c.rest {
			t.Errorf("splitHostPath(%q) = %q, %q, %v; want %q, %q", c.in, anchor, rest, err, c.anchor, c.rest)
		}
	}
	for _, bad := range []string{"projects/app", "/", "", `\server\share`} {
		if _, _, err := splitHostPath(bad); err == nil {
			t.Errorf("splitHostPath(%q) should fail", bad)
		}
	}
}

func TestWriteContextIncludesEmptyProject(t *testing.T) {
	var buf bytes.Buffer
	if err := writeContext(&buf, t.TempDir(), "FROM scratch\n", map[string]string{".config/x.json": "{}"}, contextOptions{}); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	tr := tar.NewReader(&buf)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[h.Name] = true
	}
	for _, want := range []string{"Dockerfile", "project/", "home/.config/x.json"} {
		if !names[want] {
			t.Errorf("context is missing %q; has %v", want, names)
		}
	}
}

func TestJoinHostPath(t *testing.T) {
	for _, p := range []string{`C:\`, `C:\Users\me\app`, `/Users`, `/Users/me/app`, `/home/me`} {
		anchor, rest, err := splitHostPath(p)
		if err != nil {
			t.Fatal(err)
		}
		var parts []string
		if rest != "" {
			parts = strings.Split(rest, "/")
		}
		if got := joinHostPath(anchor, parts); got != p {
			t.Errorf("joinHostPath(splitHostPath(%q)) = %q", p, got)
		}
	}
}

func TestHarnessFile(t *testing.T) {
	for p, want := range map[string]bool{
		"AGENTS.md": true, "packages/api/CLAUDE.md": true, ".claude/skills/x/SKILL.md": true, "opencode.json": true,
		".github/copilot-instructions.md": true, "src/agents.ts": false, "README.md": false, ".github/workflows/ci.yml": false,
	} {
		if got := harnessFile(p); got != want {
			t.Errorf("harnessFile(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestSideDockerfileInstallsPluginsAndManagedConfig(t *testing.T) {
	d := sideDockerfile(SideImageOptions{HomeFiles: map[string]string{"a": "b"}, SystemFiles: map[string]string{"/etc/opencode/opencode.json": "{}"}})
	for _, want := range []string{
		"cd /workspace/.opencode && npm install",
		"cd " + AgentHome + "/.config/opencode && npm install",
		"COPY system/ /",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("the Dockerfile lacks %q:\n%s", want, d)
		}
	}
	if strings.Index(d, "/workspace/.opencode && npm install") > strings.Index(d, "git commit") {
		t.Error("project plugin dependencies must be installed before the baseline commit")
	}
	if strings.Index(d, "COPY system/ /") > strings.Index(d, "USER "+AgentUser) {
		t.Error("system files must be copied as root")
	}
}
