package photo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
)

// Store is the on-disk cache of photo metadata, keyed by photo URL. It is
// meant to be committed next to config.json so that a build only has to look
// at photos it has never seen.
type Store struct {
	path  string
	mu    sync.Mutex
	items map[string]Meta
	dirty bool
}

// OpenStore loads the cache at path. A missing file is an empty cache.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]Meta{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.items); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Get(url string) (Meta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.items[url]
	return m, ok
}

func (s *Store) Put(url string, m Meta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[url] = m
	s.dirty = true
}

// Prune drops entries for photos that are no longer referenced.
func (s *Store) Prune(keep map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for url := range s.items {
		if !keep[url] {
			delete(s.items, url)
			s.dirty = true
		}
	}
}

// Save writes the cache back if anything changed. Entries are sorted and
// written one per line so that diffs stay readable.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return nil
	}
	urls := make([]string, 0, len(s.items))
	for url := range s.items {
		urls = append(urls, url)
	}
	sort.Strings(urls)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	buf.WriteString("{\n")
	for i, url := range urls {
		key, _ := json.Marshal(url)
		buf.Write(key)
		buf.WriteString(": ")
		if err := enc.Encode(s.items[url]); err != nil {
			return err
		}
		buf.Truncate(buf.Len() - 1) // Encode appends a newline
		if i < len(urls)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	if err := os.WriteFile(s.path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	s.dirty = false
	return nil
}
