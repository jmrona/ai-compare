package main

import (
	"archive/tar"
	"archive/zip"
	"errors"
	"io"
	"path"
	"strings"
)

// skipInZip are folders left out of downloads: Git history and installed dependencies can be
// recreated and would make the archive large.
var skipInZip = []string{".git", "node_modules"}

// tarToZip streams a tar archive (as Docker exports a folder) into a zip.
func tarToZip(w io.Writer, r io.Reader) error {
	zw := zip.NewWriter(w)
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path.Clean("/"+h.Name), "/")
		if name == "" || name == "." || skipped(name) {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if _, err := zw.Create(name + "/"); err != nil {
				return err
			}
		case tar.TypeReg:
			fh := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: h.ModTime}
			fh.SetMode(h.FileInfo().Mode())
			f, err := zw.CreateHeader(fh)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
		// Symlinks and special files are skipped: zip has no portable way to store them.
	}
	return zw.Close()
}

func skipped(name string) bool {
	for _, part := range strings.Split(name, "/") {
		for _, s := range skipInZip {
			if part == s {
				return true
			}
		}
	}
	return false
}
