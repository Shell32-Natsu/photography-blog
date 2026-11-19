package cmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"math"
)

// thumbnailSize is the longest edge of a thumbnail, in pixels.
const thumbnailSize = 1024

// createThumbnail generates a thumbnail from image data: a JPEG that fits in
// a thumbnailSize square, the right way up.
func createThumbnail(imgData []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// The thumbnail carries no EXIF data, so a rotation the camera only
	// recorded there has to be applied to the pixels.
	img := upright(toRGBA(src), exifOrientation(imgData))
	w, h := fitSize(img.Bounds().Dx(), img.Bounds().Dy(), thumbnailSize)
	thumb := resize(img, w, h)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}
	return buf.Bytes(), nil
}

// fitSize returns the size a w×h image has when it is scaled down, keeping
// its proportions, to fit in a max×max square. Smaller images are left alone.
func fitSize(w, h, max int) (int, int) {
	if w <= max && h <= max {
		return w, h
	}
	short := func(long, other int) int {
		if n := int(float64(max) * float64(other) / float64(long)); n > 1 {
			return n
		}
		return 1
	}
	if w > h {
		return max, short(w, h)
	}
	return short(h, w), max
}

// toRGBA copies an image of any kind into one whose pixels can be addressed
// directly.
func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// resize scales an image to w×h with a Lanczos filter, first across, then
// down.
func resize(img *image.RGBA, w, h int) *image.RGBA {
	if w != img.Bounds().Dx() {
		img = scaleWidth(img, w)
	}
	if h != img.Bounds().Dy() {
		img = transpose(scaleWidth(transpose(img), h))
	}
	return img
}

// scaleWidth resamples every row of src to w pixels.
func scaleWidth(src *image.RGBA, w int) *image.RGBA {
	sw, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	// Each output pixel is a weighted sum of the source pixels around the
	// point it comes from. When shrinking, the filter is widened by the same
	// factor so that every source pixel still contributes.
	ratio := float64(sw) / float64(w)
	scale := math.Max(ratio, 1)
	radius := math.Ceil(3 * scale)
	first := make([]int, w)
	weights := make([][]float64, w)
	for x := range weights {
		center := (float64(x)+0.5)*ratio - 0.5
		lo := int(math.Ceil(center - radius))
		if lo < 0 {
			lo = 0
		}
		hi := int(math.Floor(center + radius))
		if hi > sw-1 {
			hi = sw - 1
		}
		ws := make([]float64, hi-lo+1)
		sum := 0.0
		for i := range ws {
			ws[i] = lanczos((float64(lo+i) - center) / scale)
			sum += ws[i]
		}
		for i := range ws {
			ws[i] /= sum
		}
		first[x], weights[x] = lo, ws
	}

	for y := 0; y < h; y++ {
		in := src.Pix[y*src.Stride:]
		out := dst.Pix[y*dst.Stride:]
		for x := 0; x < w; x++ {
			var r, g, b, a float64
			p := first[x] * 4
			for _, wt := range weights[x] {
				r += wt * float64(in[p])
				g += wt * float64(in[p+1])
				b += wt * float64(in[p+2])
				a += wt * float64(in[p+3])
				p += 4
			}
			out[x*4], out[x*4+1], out[x*4+2], out[x*4+3] = clamp(r), clamp(g), clamp(b), clamp(a)
		}
	}
	return dst
}

// lanczos is the Lanczos filter with three lobes.
func lanczos(x float64) float64 {
	x = math.Abs(x)
	if x == 0 {
		return 1
	}
	if x >= 3 {
		return 0
	}
	sinc := func(x float64) float64 { return math.Sin(math.Pi*x) / (math.Pi * x) }
	return sinc(x) * sinc(x/3)
}

func clamp(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// upright undoes an EXIF orientation (1-8), so that the pixels are stored
// the way the picture is meant to be seen.
func upright(img *image.RGBA, orientation int) *image.RGBA {
	switch orientation {
	case 2: // mirrored
		flipH(img)
	case 3: // upside down
		flipH(img)
		flipV(img)
	case 4: // mirrored and upside down
		flipV(img)
	case 5: // mirrored along the diagonal
		img = transpose(img)
	case 6: // needs a quarter turn clockwise
		img = transpose(img)
		flipH(img)
	case 7: // mirrored along the other diagonal
		img = transpose(img)
		flipH(img)
		flipV(img)
	case 8: // needs a quarter turn anticlockwise
		img = transpose(img)
		flipV(img)
	}
	return img
}

// transpose swaps rows and columns.
func transpose(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s, d := y*src.Stride+x*4, x*dst.Stride+y*4
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}

// flipH mirrors the image left to right, in place.
func flipH(img *image.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var px [4]byte
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride:]
		for l, r := 0, (w-1)*4; l < r; l, r = l+4, r-4 {
			copy(px[:], row[l:l+4])
			copy(row[l:l+4], row[r:r+4])
			copy(row[r:r+4], px[:])
		}
	}
}

// flipV mirrors the image top to bottom, in place.
func flipV(img *image.RGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	tmp := make([]byte, w*4)
	for t, b := 0, h-1; t < b; t, b = t+1, b-1 {
		top, bottom := img.Pix[t*img.Stride:t*img.Stride+w*4], img.Pix[b*img.Stride:b*img.Stride+w*4]
		copy(tmp, top)
		copy(top, bottom)
		copy(bottom, tmp)
	}
}

// exifOrientation reads the EXIF orientation of a JPEG file: 1 for upright,
// which is also the answer for anything that does not say.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	// Walk the segments in front of the image data looking for the EXIF one.
	for p := 2; p+4 <= len(data); {
		if data[p] != 0xFF {
			return 1
		}
		marker := data[p+1]
		if marker == 0xFF { // padding
			p++
			continue
		}
		size := int(binary.BigEndian.Uint16(data[p+2:]))
		if marker == 0xDA || size < 2 || p+2+size > len(data) {
			return 1
		}
		segment := data[p+4 : p+2+size]
		if marker == 0xE1 && bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			return tiffOrientation(segment[6:])
		}
		p += 2 + size
	}
	return 1
}

// tiffOrientation finds the orientation tag in the first directory of the
// TIFF structure that EXIF data is stored in.
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	dir := int64(order.Uint32(tiff[4:]))
	if dir < 8 || dir+2 > int64(len(tiff)) {
		return 1
	}
	entries := int64(order.Uint16(tiff[dir:]))
	for i := int64(0); i < entries; i++ {
		entry := dir + 2 + 12*i
		if entry+12 > int64(len(tiff)) {
			break
		}
		if order.Uint16(tiff[entry:]) == 0x0112 {
			if v := int(order.Uint16(tiff[entry+8:])); v >= 1 && v <= 8 {
				return v
			}
			break
		}
	}
	return 1
}
