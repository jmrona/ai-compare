package workspace

import (
	"archive/tar"
	"bytes"
	"io"
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
	if err := writeContext(&buf, t.TempDir(), "FROM scratch\n", map[string]string{".config/x.json": "{}"}); err != nil {
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
