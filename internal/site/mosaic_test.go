package site

import "testing"

func TestGaps(t *testing.T) {
	tests := []struct {
		shapes []string
		cols   int
		want   int
	}{
		{[]string{"", "", "", ""}, 2, 0},
		{[]string{"", "", ""}, 2, 1},
		{[]string{"big", "", "", "", ""}, 4, 0},
		{[]string{"tall", "", ""}, 2, 0},
		{[]string{"tall", "big"}, 2, 2},      // the big tile cannot sit next to the tall one
		{[]string{"", "big", ""}, 3, 0},      // dense: the last tile moves up under the first
		{[]string{"wide", "wide", ""}, 3, 1}, // the small tile fills one of the two leftover cells
		{[]string{"big"}, 1, 0},              // wider than the grid: clamped
	}
	for _, tt := range tests {
		if got := gaps(tt.shapes, tt.cols); got != tt.want {
			t.Errorf("gaps(%q, %d) = %d, want %d", tt.shapes, tt.cols, got, tt.want)
		}
	}
}

func TestShapeFollowsCover(t *testing.T) {
	cover := func(w, h int) *Album {
		return &Album{Cover: &Photo{W: w, H: h, Ratio: float64(w) / float64(h)}}
	}
	// With nothing after them to fill gaps, shapes keep their first choice
	// only where that costs nothing; the first choices are what we check on
	// a layout that tiles cleanly: a big tile and four small ones is 2x4.
	albums := []*Album{cover(1024, 683), cover(1024, 683), cover(1024, 683), cover(1024, 683), cover(1024, 683), {}}
	shape(albums)
	if albums[0].Shape != "big" {
		t.Errorf("first landscape cover: shape %q, want big", albums[0].Shape)
	}
	if albums[5].Shape != "" {
		t.Errorf("album without a cover: shape %q, want none", albums[5].Shape)
	}
}

func TestShapeClosesGaps(t *testing.T) {
	// Three upright covers would leave holes in a 2-column grid if all of
	// them were given a tall tile next to a single small one.
	var albums []*Album
	for i := 0; i < 9; i++ {
		w, h := 1024, 683
		if i%3 == 0 {
			w, h = 683, 1024
		}
		albums = append(albums, &Album{Cover: &Photo{W: w, H: h, Ratio: float64(w) / float64(h)}})
	}
	shape(albums)
	shapes := make([]string, len(albums))
	for i, a := range albums {
		shapes[i] = a.Shape
	}
	if g := gaps(shapes, 2) + gaps(shapes, 4); g != 0 {
		t.Errorf("shapes %q leave %d gaps", shapes, g)
	}
}
