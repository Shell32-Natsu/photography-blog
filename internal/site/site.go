// Package site turns the configuration tree into pages and writes them out.
package site

import (
	"fmt"
	"math"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/photography-blog/internal/config"
	"github.com/photography-blog/internal/photo"
)

// Site holds what every page shares.
type Site struct {
	Title       string
	Description string
	Author      string
	Albums      int    // number of album pages
	Photos      int    // number of distinct photos
	Rev         string // changes whenever a theme asset changes
	Year        int
}

// Photo is one picture in a gallery.
type Photo struct {
	URL   string  // full-size image
	Thumb string  // image shown in the grid
	Name  string  // file name without extension, used for deep links
	N     int     // position in its album, 1-based
	W, H  int     // thumbnail size, 0 when unknown
	Ratio float64 // width / height used for layout
	Info  string  // shooting data, fields separated by "|"
}

// Crumb is one step of the path back to the home page.
type Crumb struct {
	Title string
	Href  string
}

// Album is a page: the home page or a collection. It also describes the
// tile that represents the album on its parent's page.
type Album struct {
	Site        *Site
	ID          string // unique, safe to use as a CSS identifier
	Title       string
	Description string
	Href        string // absolute URL path, "/" for the home page
	File        string // output file, relative to the output directory
	Home        bool
	NotFound    bool
	Cover       *Photo
	Count       int    // photos in this album and the albums below it
	Num, Of     int    // position among the sibling albums, 1-based
	Shape       string // tile shape on the parent page: "", "tall", "wide" or "big"
	Crumbs      []Crumb
	Albums      []*Album
	Photos      []*Photo
	Next        *Album   // the sibling to continue with
	Origins     []string // hosts the page loads thumbnails from
}

// ThumbURL maps a photo to its thumbnail. Originals live in the "photo-r2"
// bucket and scripts/metadata-editor mirrors them, downsized, to
// "photo-thumbs-r2"; anything else is shown as is.
func ThumbURL(u string) string {
	if strings.Contains(u, "photo-r2") {
		return strings.Replace(u, "photo-r2", "photo-thumbs-r2", 1)
	}
	return u
}

// Lookup returns the cached metadata of a photo.
type Lookup func(url string) (photo.Meta, bool)

// Build creates the page tree for the configuration.
func Build(root *config.Node, lookup Lookup) *Album {
	b := &builder{
		lookup: lookup,
		site: &Site{
			Title:       root.Title,
			Description: root.Description,
			Author:      root.Author,
			Photos:      len(root.Photos()),
			Year:        time.Now().Year(),
		},
	}
	home := b.album(root, nil, nil)
	b.site.Albums = b.pages - 1
	return home
}

type builder struct {
	site   *Site
	lookup Lookup
	pages  int
}

func (b *builder) album(n *config.Node, segments []string, crumbs []Crumb) *Album {
	a := &Album{
		Site:        b.site,
		ID:          fmt.Sprintf("a%d", b.pages),
		Title:       n.Title,
		Description: n.Description,
		Home:        b.pages == 0,
		Href:        href(segments),
		File:        path.Join(path.Join(segments...), "index.html"),
		Crumbs:      crumbs,
	}
	b.pages++

	below := append(crumbs[:len(crumbs):len(crumbs)], Crumb{Title: n.Title, Href: a.Href})
	used := map[string]bool{}
	for _, c := range n.Children {
		switch {
		case c.IsAlbum():
			seg := unique(slug(c.Title), used)
			sub := b.album(c, append(segments[:len(segments):len(segments)], seg), below)
			a.Albums = append(a.Albums, sub)
			a.Count += sub.Count
		case c.URL != "":
			p := b.photo(c)
			p.N = len(a.Photos) + 1
			a.Photos = append(a.Photos, p)
		}
	}
	a.Count += len(a.Photos)

	switch {
	case n.URL != "":
		a.Cover = b.photo(n)
	case len(a.Photos) > 0:
		a.Cover = a.Photos[0]
	case len(a.Albums) > 0:
		a.Cover = a.Albums[0].Cover
	}

	for i, sub := range a.Albums {
		sub.Num, sub.Of = i+1, len(a.Albums)
		if len(a.Albums) > 1 {
			sub.Next = a.Albums[(i+1)%len(a.Albums)]
		}
	}
	shape(a.Albums)
	a.Origins = origins(a)
	return a
}

func (b *builder) photo(n *config.Node) *Photo {
	p := &Photo{URL: n.URL, Thumb: ThumbURL(n.URL), Name: name(n.URL), Ratio: 1.5}
	meta, _ := b.lookup(n.URL)
	if r := meta.Ratio(); r > 0 {
		p.W, p.H, p.Ratio = meta.W, meta.H, math.Round(r*1e4)/1e4
	}
	exif := meta.Exif
	if n.Exif != nil {
		exif = n.Exif // written by hand in config.json
	}
	p.Info = strings.Join(exif.Fields(), "|")
	return p
}

func origins(a *Album) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p *Photo) {
		if p == nil {
			return
		}
		u, err := url.Parse(p.Thumb)
		if err != nil || u.Host == "" {
			return
		}
		if o := u.Scheme + "://" + u.Host; !seen[o] {
			seen[o] = true
			out = append(out, o)
		}
	}
	add(a.Cover)
	for _, sub := range a.Albums {
		add(sub.Cover)
	}
	for _, p := range a.Photos {
		add(p)
	}
	return out
}

// href builds the URL of an album from its path segments.
func href(segments []string) string {
	var sb strings.Builder
	sb.WriteByte('/')
	for _, s := range segments {
		sb.WriteString(url.PathEscape(s))
		sb.WriteByte('/')
	}
	return sb.String()
}

// slug makes an album title usable as a directory name.
func slug(title string) string {
	s := strings.TrimSpace(strings.NewReplacer("/", "-", "\\", "-").Replace(title))
	if s == "" || s == "." || s == ".." {
		return "album"
	}
	return s
}

func unique(s string, used map[string]bool) string {
	out := s
	for i := 2; used[out]; i++ {
		out = fmt.Sprintf("%s-%d", s, i)
	}
	used[out] = true
	return out
}

// name is the file name of a photo without its extension.
func name(u string) string {
	if p, err := url.Parse(u); err == nil && p.Path != "" {
		u = p.Path
	}
	base := path.Base(u)
	return strings.TrimSuffix(base, path.Ext(base))
}
