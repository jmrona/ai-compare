package presets

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) WriteZip(slug string, w io.Writer) (string, error) {
	p, err := s.Get(slug)
	if err != nil {
		return "", err
	}
	name := ZipName(p.Title, slug)
	folder := strings.TrimSuffix(name, ".zip")
	root := s.Dir(slug)
	zw := zip.NewWriter(w)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		info, err := d.Info()
		if err != nil {
			return err
		}
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: folder + "/" + filepath.ToSlash(rel), Method: zip.Deflate, Modified: info.ModTime()})
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
	if err != nil {
		return name, err
	}
	return name, zw.Close()
}

func ZipName(title, slug string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	name = strings.Trim(name, ". ")
	if name == "" {
		name = slug
	}
	return name + ".zip"
}
