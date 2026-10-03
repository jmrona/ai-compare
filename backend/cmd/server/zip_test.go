package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strings"
	"testing"
)

func TestTarToZipSkipsGitAndDependencies(t *testing.T) {
	var in bytes.Buffer
	tw := tar.NewWriter(&in)
	add := func(name, body string, dir bool) {
		h := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if dir {
			h.Typeflag, h.Size, h.Mode = tar.TypeDir, 0, 0o755
		}
		tw.WriteHeader(h)
		io.WriteString(tw, body)
	}
	add("./", "", true)
	add("./poem.md", "# Poem\n", false)
	add("./src/", "", true)
	add("./src/a.js", "x", false)
	add("./.git/HEAD", "ref", false)
	add("./node_modules/pkg/i.js", "y", false)
	tw.Close()

	var out bytes.Buffer
	if err := tarToZip(&out, &in); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "poem.md,src/,src/a.js" {
		t.Fatalf("zip has %s", got)
	}
}
