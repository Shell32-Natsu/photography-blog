// Package config reads config.json, the tree of albums and photos that the
// site is generated from.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/photography-blog/internal/photo"
)

// Node is either an album (it has children) or a single photo (it has a
// URL). The root node describes the site itself. An album's URL is its
// cover picture.
type Node struct {
	Title       string      `json:"title,omitempty"`
	Description string      `json:"description,omitempty"`
	Author      string      `json:"author,omitempty"`
	URL         string      `json:"url,omitempty"`
	Exif        *photo.Exif `json:"exif,omitempty"`
	Children    []*Node     `json:"children,omitempty"`
}

// IsAlbum reports whether the node is a collection rather than a photo.
func (n *Node) IsAlbum() bool { return len(n.Children) > 0 }

// Photos returns the URL of every photo below the node, in order, with
// duplicates removed.
func (n *Node) Photos() []string {
	seen := map[string]bool{}
	var urls []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.URL != "" && !seen[n.URL] {
			seen[n.URL] = true
			urls = append(urls, n.URL)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return urls
}

// Load reads and validates the configuration file at path.
func Load(path string) (*Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root Node
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if root.Title == "" {
		return nil, errors.New(path + `: the root node needs a "title"`)
	}
	return &root, nil
}
