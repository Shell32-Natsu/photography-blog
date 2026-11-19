package site

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Render writes every page below home into outDir using the theme in
// themeDir. The theme is a single template, index.html; every other file in
// the theme directory is copied to the output as is.
func Render(home *Album, themeDir, outDir string) error {
	rev, err := copyTree(themeDir, outDir, func(rel string) bool {
		return !strings.HasSuffix(rel, ".html")
	})
	if err != nil {
		return err
	}
	home.Site.Rev = rev

	tmpl, err := template.ParseFiles(filepath.Join(themeDir, "index.html"))
	if err != nil {
		return err
	}
	pages := append(flatten(home), &Album{
		Site:     home.Site,
		ID:       "lost",
		Title:    "404",
		Href:     "/404.html",
		File:     "404.html",
		NotFound: true,
	})
	for _, page := range pages {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, page); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(outDir, filepath.FromSlash(page.File)), buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// CopyImages copies locally stored photos into the output, if there are any.
func CopyImages(dir, outDir string) error {
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	_, err := copyTree(dir, filepath.Join(outDir, filepath.Base(dir)), nil)
	return err
}

func flatten(a *Album) []*Album {
	pages := []*Album{a}
	for _, sub := range a.Albums {
		pages = append(pages, flatten(sub)...)
	}
	return pages
}

// copyTree copies the files under src to dst and returns a short hash of
// everything it copied. keep may be nil to copy every file.
func copyTree(src, dst string, keep func(rel string) bool) (string, error) {
	sum := sha1.New()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") || (keep != nil && !keep(rel)) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		io.WriteString(sum, filepath.ToSlash(rel))
		sum.Write(data)
		return writeFile(filepath.Join(dst, rel), data)
	})
	return hex.EncodeToString(sum.Sum(nil))[:8], err
}

func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}
