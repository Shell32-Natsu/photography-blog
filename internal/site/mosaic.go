package site

// The album index is a mosaic: a dense grid in which upright covers get an
// upright tile, panoramas a wide one, and every few landscape covers one is
// enlarged. The grid has 2 to 5 columns depending on the screen.

// tile sizes in grid cells, by shape
var tileSize = map[string][2]int{"": {1, 1}, "tall": {1, 2}, "wide": {2, 1}, "big": {2, 2}}

// gridWidths are the column counts the theme uses, with how much a gap in
// each of them matters.
var gridWidths = map[int]int{2: 3, 3: 2, 4: 4, 5: 1}

// shape assigns a tile shape to every album.
func shape(albums []*Album) {
	shapes := make([]string, len(albums))
	choices := make([][]string, len(albums)) // preferred shape first
	landscape := 0
	for i, a := range albums {
		choices[i] = []string{""}
		if a.Cover == nil || a.Cover.W == 0 {
			continue
		}
		switch r := a.Cover.Ratio; {
		case r < 0.9:
			choices[i] = []string{"tall", ""}
		case r > 2.2:
			choices[i] = []string{"wide", ""}
		default:
			if landscape%5 == 0 {
				choices[i] = []string{"big", ""}
			} else {
				choices[i] = []string{"", "big"}
			}
			landscape++
		}
		shapes[i] = choices[i][0]
	}

	// A dense grid can still end with gaps. Try the alternatives for the
	// last few tiles and keep the arrangement that leaves the fewest.
	const tail = 10
	first := len(albums) - tail
	if first < 0 {
		first = 0
	}
	best, bestCost := append([]string(nil), shapes...), cost(shapes)
	var try func(i int)
	try = func(i int) {
		if i == len(albums) {
			if c := cost(shapes); c < bestCost {
				best, bestCost = append(best[:0], shapes...), c
			}
			return
		}
		for _, s := range choices[i] {
			shapes[i] = s
			try(i + 1)
		}
	}
	try(first)

	for i, a := range albums {
		a.Shape = best[i]
	}
}

func cost(shapes []string) int {
	total := 0
	for cols, weight := range gridWidths {
		total += weight * gaps(shapes, cols)
	}
	return total
}

// gaps lays the tiles out the way a browser fills a dense grid and counts
// the cells that stay empty.
func gaps(shapes []string, cols int) int {
	var rows [][]bool
	used := 0
	fits := func(r, c, w, h int) bool {
		for y := r; y < r+h && y < len(rows); y++ {
			for x := c; x < c+w; x++ {
				if rows[y][x] {
					return false
				}
			}
		}
		return true
	}
	for _, s := range shapes {
		w, h := tileSize[s][0], tileSize[s][1]
		if w > cols {
			w = cols
		}
	place:
		for r := 0; ; r++ {
			for c := 0; c+w <= cols; c++ {
				if !fits(r, c, w, h) {
					continue
				}
				for len(rows) < r+h {
					rows = append(rows, make([]bool, cols))
				}
				for y := r; y < r+h; y++ {
					for x := c; x < c+w; x++ {
						rows[y][x] = true
					}
				}
				used += w * h
				break place
			}
		}
	}
	return len(rows)*cols - used
}
