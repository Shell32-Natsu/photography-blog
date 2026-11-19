// Package photo reads the facts about a picture that the theme needs up
// front: its pixel size (so the layout never shifts while images load) and
// its shooting data.
package photo

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"strconv"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

// Exif is the shooting data shown in the lightbox. Every field is already
// formatted for display; empty fields are simply left out.
type Exif struct {
	Model    string `json:"model,omitempty"`
	Lens     string `json:"lens,omitempty"`
	Focal    string `json:"focal,omitempty"`
	Aperture string `json:"aperture,omitempty"`
	Shutter  string `json:"shutter,omitempty"`
	ISO      string `json:"iso,omitempty"`
	Date     string `json:"date,omitempty"`
}

// Fields returns the non-empty values in display order.
func (e *Exif) Fields() []string {
	if e == nil {
		return nil
	}
	var out []string
	for _, v := range []string{e.Model, e.Lens, e.Focal, e.Aperture, e.Shutter, e.ISO, e.Date} {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// Meta is what the generator knows about one photo.
type Meta struct {
	// W and H are the displayed size of the thumbnail, in pixels.
	W int `json:"w,omitempty"`
	H int `json:"h,omitempty"`
	// Sideways is set when the original carries an EXIF rotation that the
	// thumbnail does not reflect, so the two do not have the same shape.
	Sideways bool  `json:"sideways,omitempty"`
	Exif     *Exif `json:"exif,omitempty"`
}

// Ratio is width / height, or 0 when the size is unknown.
func (m Meta) Ratio() float64 {
	if m.W <= 0 || m.H <= 0 {
		return 0
	}
	return float64(m.W) / float64(m.H)
}

// size reads the pixel size from the first bytes of an image file.
func size(head []byte) (w, h int, ok bool) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(head))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// readExif extracts shooting data from the first bytes of an image file.
// orientation is the EXIF orientation tag (1-8), or 1 when absent.
func readExif(head []byte) (e *Exif, orientation int) {
	orientation = 1
	x, err := exif.Decode(bytes.NewReader(head))
	if err != nil || x == nil {
		return nil, orientation
	}
	if t, err := x.Get(exif.Orientation); err == nil {
		if v, err := t.Int(0); err == nil && v >= 1 && v <= 8 {
			orientation = v
		}
	}

	str := func(name exif.FieldName) string {
		t, err := x.Get(name)
		if err != nil {
			return ""
		}
		s, err := t.StringVal()
		if err != nil {
			return ""
		}
		return strings.Join(strings.Fields(s), " ")
	}
	rat := func(name exif.FieldName) (float64, bool) {
		t, err := x.Get(name)
		if err != nil {
			return 0, false
		}
		num, den, err := t.Rat2(0)
		if err != nil || den == 0 || num <= 0 {
			return 0, false
		}
		return float64(num) / float64(den), true
	}

	e = &Exif{Model: str(exif.Model), Lens: cleanLens(str(exif.LensModel)), Date: formatDate(str(exif.DateTimeOriginal))}
	if v, ok := rat(exif.FocalLength); ok {
		e.Focal = formatFocal(v)
	}
	if v, ok := rat(exif.FNumber); ok {
		e.Aperture = formatAperture(v)
	}
	if v, ok := rat(exif.ExposureTime); ok {
		e.Shutter = formatShutter(v)
	}
	if t, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if v, err := t.Int(0); err == nil && v > 0 {
			e.ISO = "ISO " + strconv.Itoa(v)
		}
	}
	if len(e.Fields()) == 0 {
		return nil, orientation
	}
	return e, orientation
}

// formatFocal rounds the way lenses are labelled: whole millimetres, except
// for very short focal lengths where the fraction matters.
func formatFocal(mm float64) string {
	if mm < 10 {
		return trimFloat(math.Round(mm*10)/10) + "mm"
	}
	return trimFloat(math.Round(mm)) + "mm"
}

func formatAperture(f float64) string {
	return "ƒ/" + trimFloat(math.Round(f*10)/10)
}

// formatShutter renders an exposure time in seconds the way cameras do:
// "1/450s" for fast speeds, "2.5s" for long ones.
func formatShutter(sec float64) string {
	if sec >= 0.4 {
		return trimFloat(math.Round(sec*10)/10) + "s"
	}
	return fmt.Sprintf("1/%ds", int(math.Round(1/sec)))
}

// formatDate turns EXIF "2021:03:26 14:06:59" into "2021.03.26".
func formatDate(s string) string {
	if len(s) < 10 || s[4] != ':' || s[7] != ':' || strings.HasPrefix(s, "0000") {
		return ""
	}
	return s[0:4] + "." + s[5:7] + "." + s[8:10]
}

// cleanLens shortens the verbose names some raw converters write, e.g.
// "Fujifilm Fujinon XF18-55mmF2.8-4 R LM OIS" or
// "SIGMA 18-50mm F2.8 DC DN | Contemporary 021".
func cleanLens(s string) string {
	s = strings.TrimPrefix(s, "Fujifilm Fujinon ")
	if i := strings.Index(s, " | "); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
