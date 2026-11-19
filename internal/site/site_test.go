package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/photography-blog/internal/config"
	"github.com/photography-blog/internal/photo"
)

func photoNode(url string) *config.Node { return &config.Node{URL: url} }

func testTree() *config.Node {
	return &config.Node{
		Title: "Blog",
		Children: []*config.Node{
			{Title: "日本", URL: "https://photo-r2.example/b.jpg", Children: []*config.Node{
				photoNode("https://photo-r2.example/a.jpg"),
				photoNode("https://photo-r2.example/b.jpg"),
				{Title: "東京 / 夜", Children: []*config.Node{photoNode("https://photo-r2.example/c d.jpg")}},
			}},
			{Title: "Napa", Children: []*config.Node{
				photoNode("https://photo-r2.example/a.jpg"),
				{URL: "/image/local.jpg", Exif: &photo.Exif{Focal: "35mm", ISO: "ISO 100"}},
			}},
		},
	}
}

func testLookup(url string) (photo.Meta, bool) {
	switch {
	case strings.HasSuffix(url, "a.jpg"):
		return photo.Meta{W: 1024, H: 683, Exif: &photo.Exif{Model: "X-S10", Aperture: "ƒ/4"}}, true
	case strings.HasSuffix(url, "b.jpg"):
		return photo.Meta{W: 682, H: 1024}, true
	}
	return photo.Meta{}, false
}

func TestBuild(t *testing.T) {
	home := Build(testTree(), testLookup)

	if !home.Home || home.Href != "/" || home.File != "index.html" {
		t.Errorf("home: %+v", home)
	}
	if home.Site.Albums != 3 || home.Site.Photos != 4 {
		t.Errorf("site counts: %d albums, %d photos; want 3 and 4", home.Site.Albums, home.Site.Photos)
	}
	if len(home.Albums) != 2 {
		t.Fatalf("%d albums on the home page, want 2", len(home.Albums))
	}

	japan, napa := home.Albums[0], home.Albums[1]
	if japan.Href != "/%E6%97%A5%E6%9C%AC/" || japan.File != "日本/index.html" {
		t.Errorf("album path: href %q, file %q", japan.Href, japan.File)
	}
	if japan.Count != 3 || len(japan.Photos) != 2 || japan.Num != 1 || japan.Of != 2 {
		t.Errorf("album counts: %+v", japan)
	}
	if japan.Cover.URL != "https://photo-r2.example/b.jpg" || japan.Cover.Thumb != "https://photo-thumbs-r2.example/b.jpg" {
		t.Errorf("cover: %+v", japan.Cover)
	}
	if japan.Next != napa || napa.Next != japan {
		t.Error("albums should lead to each other")
	}
	if len(japan.Crumbs) != 1 || japan.Crumbs[0].Href != "/" {
		t.Errorf("crumbs: %+v", japan.Crumbs)
	}
	if got := japan.Origins; len(got) != 1 || got[0] != "https://photo-thumbs-r2.example" {
		t.Errorf("origins: %v", got)
	}

	p := japan.Photos[0]
	if p.N != 1 || p.W != 1024 || p.Ratio != 1.4993 || p.Info != "X-S10|ƒ/4" || p.Name != "a" {
		t.Errorf("photo: %+v", p)
	}

	// a nested album: slashes cannot be part of a directory name
	night := japan.Albums[0]
	if night.File != "日本/東京 - 夜/index.html" || night.Href != "/%E6%97%A5%E6%9C%AC/%E6%9D%B1%E4%BA%AC%20-%20%E5%A4%9C/" {
		t.Errorf("nested path: href %q, file %q", night.Href, night.File)
	}
	if len(night.Crumbs) != 2 || night.Crumbs[1].Title != "日本" || night.Next != nil {
		t.Errorf("nested: crumbs %+v, next %v", night.Crumbs, night.Next)
	}
	if q := night.Photos[0]; q.Name != "c d" || q.Ratio != 1.5 || q.W != 0 {
		t.Errorf("photo without metadata: %+v", q)
	}

	// no cover given: the first photo; shooting data written in config.json wins
	if napa.Cover != napa.Photos[0] {
		t.Error("cover should default to the first photo")
	}
	if local := napa.Photos[1]; local.Thumb != "/image/local.jpg" || local.Info != "35mm|ISO 100" {
		t.Errorf("local photo: %+v", local)
	}

	ids := map[string]bool{}
	for _, a := range flatten(home) {
		if ids[a.ID] {
			t.Errorf("duplicate id %q", a.ID)
		}
		ids[a.ID] = true
	}
}

func TestUniqueSlugs(t *testing.T) {
	root := &config.Node{Title: "Blog", Children: []*config.Node{
		{Title: "A", Children: []*config.Node{photoNode("/image/1.jpg")}},
		{Title: "A", Children: []*config.Node{photoNode("/image/2.jpg")}},
		{Title: "..", Children: []*config.Node{photoNode("/image/3.jpg")}},
	}}
	home := Build(root, testLookup)
	got := []string{home.Albums[0].File, home.Albums[1].File, home.Albums[2].File}
	want := []string{"A/index.html", "A-2/index.html", "album/index.html"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d: %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRender(t *testing.T) {
	out := t.TempDir()
	home := Build(testTree(), testLookup)
	if err := Render(home, "../../template", out); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"index.html", "404.html", "日本/index.html", "日本/東京 - 夜/index.html", "Napa/index.html", "assets/app.css", "assets/app.js", "favicon.svg"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing %s", f)
		}
	}

	page, _ := os.ReadFile(filepath.Join(out, "日本", "index.html"))
	html := string(page)
	for _, want := range []string{
		`<title>日本 · Blog</title>`,
		`href="https://photo-r2.example/a.jpg"`,
		`src="https://photo-thumbs-r2.example/a.jpg"`,
		`style="--r:1.4993"`,
		`data-info="X-S10|ƒ/4"`,
		`width="1024" height="683"`,
		`/assets/app.css?v=` + home.Site.Rev,
		`<link rel="preconnect" href="https://photo-thumbs-r2.example">`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("album page lacks %s", want)
		}
	}
	if strings.Contains(html, "ZgotmplZ") {
		t.Error("the template engine rejected a value as unsafe")
	}

	index, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(index), `href="/%E6%97%A5%E6%9C%AC/"`) {
		t.Error("home page does not link to the album")
	}
}
