package cmd

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestFitSize(t *testing.T) {
	tests := []struct{ w, h, wantW, wantH int }{
		{6240, 4160, 1024, 682},
		{4160, 6240, 682, 1024},
		{3972, 3972, 1024, 1024},
		{4096, 2304, 1024, 576},
		{800, 600, 800, 600}, // already small enough: left alone
		{1024, 1024, 1024, 1024},
		{50000, 10, 1024, 1},
	}
	for _, tt := range tests {
		if w, h := fitSize(tt.w, tt.h, 1024); w != tt.wantW || h != tt.wantH {
			t.Errorf("fitSize(%d, %d) = %dx%d, want %dx%d", tt.w, tt.h, w, h, tt.wantW, tt.wantH)
		}
	}
}

type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

// withOrientation inserts an EXIF block holding only the orientation tag
// into a JPEG file.
func withOrientation(file []byte, orientation uint16, order byteOrder) []byte {
	tiff := []byte("II*\x00")
	if order == binary.BigEndian {
		tiff = []byte("MM\x00*")
	}
	tiff = order.AppendUint32(tiff, 8) // the directory follows the header
	tiff = order.AppendUint16(tiff, 1) // one entry
	tiff = order.AppendUint16(tiff, 0x0112)
	tiff = order.AppendUint16(tiff, 3) // a 16-bit value
	tiff = order.AppendUint32(tiff, 1)
	tiff = order.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0) // padding, no further directory

	exif := append([]byte("Exif\x00\x00"), tiff...)
	segment := binary.BigEndian.AppendUint16([]byte{0xFF, 0xE1}, uint16(len(exif)+2))
	segment = append(segment, exif...)
	return append(append(append([]byte{}, file[:2]...), segment...), file[2:]...)
}

func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{A: 255}
			if y < h/2 { // top half red, bottom half blue
				c.R = 255
			} else {
				c.B = 255
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExifOrientation(t *testing.T) {
	plain := testJPEG(t, 16, 8)
	if got := exifOrientation(plain); got != 1 {
		t.Errorf("no EXIF: got %d, want 1", got)
	}
	for _, order := range []byteOrder{binary.LittleEndian, binary.BigEndian} {
		for o := uint16(1); o <= 8; o++ {
			if got := exifOrientation(withOrientation(plain, o, order)); got != int(o) {
				t.Errorf("%v orientation %d: got %d", order, o, got)
			}
		}
		if got := exifOrientation(withOrientation(plain, 9, order)); got != 1 {
			t.Errorf("%v invalid orientation: got %d, want 1", order, got)
		}
	}
	// damaged or foreign data must not panic and reads as upright; the EXIF
	// block built above ends at byte 38
	rotated := withOrientation(plain, 6, binary.LittleEndian)
	for n := 0; n < 38; n++ {
		if got := exifOrientation(rotated[:n]); got != 1 {
			t.Errorf("truncated to %d bytes: got %d, want 1", n, got)
		}
	}
	if got := exifOrientation([]byte("\x89PNG\r\n\x1a\n....")); got != 1 {
		t.Errorf("PNG: got %d, want 1", got)
	}
}

func TestUpright(t *testing.T) {
	// a 2x1 image: A B
	a, b := color.RGBA{1, 1, 1, 255}, color.RGBA{2, 2, 2, 255}
	tests := []struct {
		orientation int
		w, h        int
		first, last color.RGBA
	}{
		{1, 2, 1, a, b},
		{2, 2, 1, b, a},
		{3, 2, 1, b, a},
		{4, 2, 1, a, b},
		{5, 1, 2, a, b},
		{6, 1, 2, a, b}, // quarter turn clockwise: the left end goes to the top
		{7, 1, 2, b, a},
		{8, 1, 2, b, a}, // quarter turn anticlockwise: the right end goes to the top
	}
	for _, tt := range tests {
		img := image.NewRGBA(image.Rect(0, 0, 2, 1))
		img.SetRGBA(0, 0, a)
		img.SetRGBA(1, 0, b)
		got := upright(img, tt.orientation)
		if got.Bounds().Dx() != tt.w || got.Bounds().Dy() != tt.h {
			t.Errorf("orientation %d: %v, want %dx%d", tt.orientation, got.Bounds(), tt.w, tt.h)
			continue
		}
		if first, last := got.RGBAAt(0, 0), got.RGBAAt(tt.w-1, tt.h-1); first != tt.first || last != tt.last {
			t.Errorf("orientation %d: ends %v %v, want %v %v", tt.orientation, first, last, tt.first, tt.last)
		}
	}
}

func TestResizeKeepsFlatColour(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 10, 120, 250, 255
	}
	got := resize(img, 30, 20)
	if got.Bounds().Dx() != 30 || got.Bounds().Dy() != 20 {
		t.Fatalf("size %v, want 30x20", got.Bounds())
	}
	for i := 0; i < len(got.Pix); i += 4 {
		if got.Pix[i] != 10 || got.Pix[i+1] != 120 || got.Pix[i+2] != 250 || got.Pix[i+3] != 255 {
			t.Fatalf("pixel %d: %v, want the original colour", i/4, got.Pix[i:i+4])
		}
	}
}

func TestCreateThumbnail(t *testing.T) {
	// 3000x2000, red on top; marked as needing a quarter turn clockwise
	data := withOrientation(testJPEG(t, 3000, 2000), 6, binary.LittleEndian)
	out, err := createThumbnail(data)
	if err != nil {
		t.Fatal(err)
	}
	thumb, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := thumb.Bounds(); b.Dx() != 682 || b.Dy() != 1024 {
		t.Fatalf("size %v, want 682x1024", b)
	}
	if exifOrientation(out) != 1 {
		t.Error("the thumbnail should carry no rotation of its own")
	}
	// turned clockwise, the red top ends up on the right
	isRed := func(x, y int) bool {
		r, _, b, _ := thumb.At(x, y).RGBA()
		return r > 0xC000 && b < 0x4000
	}
	if !isRed(600, 512) || isRed(80, 512) {
		t.Error("the picture was not turned a quarter clockwise")
	}

	if _, err := createThumbnail([]byte("not an image")); err == nil {
		t.Error("garbage in: want an error")
	}
}
