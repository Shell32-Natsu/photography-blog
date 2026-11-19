package photo

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	tests := []struct{ got, want string }{
		{formatShutter(1.0 / 450), "1/450s"},
		{formatShutter(0.0022222222), "1/450s"},
		{formatShutter(1.0 / 8000), "1/8000s"},
		{formatShutter(0.5), "0.5s"},
		{formatShutter(8), "8s"},
		{formatShutter(2.5), "2.5s"},
		{formatAperture(4), "ƒ/4"},
		{formatAperture(2.8), "ƒ/2.8"},
		{formatAperture(22), "ƒ/22"},
		{formatFocal(50.5), "51mm"},
		{formatFocal(18), "18mm"},
		{formatFocal(4.25), "4.3mm"},
		{formatDate("2021:03:26 14:06:59"), "2021.03.26"},
		{formatDate("0000:00:00 00:00:00"), ""},
		{formatDate(""), ""},
		{cleanLens("Fujifilm Fujinon XF18-55mmF2.8-4 R LM OIS"), "XF18-55mmF2.8-4 R LM OIS"},
		{cleanLens("SIGMA 18-50mm F2.8 DC DN | Contemporary 021"), "SIGMA 18-50mm F2.8 DC DN"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestFields(t *testing.T) {
	var none *Exif
	if got := none.Fields(); got != nil {
		t.Errorf("nil Exif: got %v", got)
	}
	e := &Exif{Model: "X-S10", Aperture: "ƒ/4", ISO: "ISO 160"}
	if got := strings.Join(e.Fields(), "|"); got != "X-S10|ƒ/4|ISO 160" {
		t.Errorf("got %q", got)
	}
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProbeRemote(t *testing.T) {
	orig, thumb := jpegBytes(t, 3000, 2000), jpegBytes(t, 1024, 683)
	var ranged int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			ranged++
		}
		switch r.URL.Path {
		case "/orig/a b.jpg":
			w.Write(orig)
		case "/thumb/a b.jpg":
			w.Write(thumb)
		case "/orig/nothumb.jpg":
			w.Write(orig)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := &Prober{Thumb: func(u string) string { return strings.Replace(u, "/orig/", "/thumb/", 1) }}

	m, err := p.Probe(context.Background(), srv.URL+"/orig/a b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if m.W != 1024 || m.H != 683 || m.Sideways {
		t.Errorf("got %+v, want the thumbnail's 1024x683", m)
	}
	if ranged != 2 {
		t.Errorf("%d ranged requests, want 2", ranged)
	}

	// without a thumbnail the original's size is the best we have
	m, err = p.Probe(context.Background(), srv.URL+"/orig/nothumb.jpg")
	if err != nil || m.W != 3000 || m.H != 2000 {
		t.Errorf("got %+v, %v; want 3000x2000", m, err)
	}

	if _, err := p.Probe(context.Background(), srv.URL+"/orig/missing.jpg"); err == nil {
		t.Error("missing photo: want an error")
	}
}

func TestFillUsesCache(t *testing.T) {
	hits := 0
	img := jpegBytes(t, 800, 600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if strings.HasSuffix(r.URL.Path, "gone.jpg") {
			http.NotFound(w, r)
			return
		}
		w.Write(img)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "photos.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{srv.URL + "/a.jpg", srv.URL + "/b.jpg", srv.URL + "/gone.jpg"}
	p := &Prober{Workers: 2}
	if failed := p.Fill(context.Background(), urls, store, nil); len(failed) != 1 {
		t.Errorf("failed = %v, want only gone.jpg", failed)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	// a second run reads the two known photos from disk and retries the third
	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	hits = 0
	p.Fill(context.Background(), urls, store, nil)
	if hits != 1 {
		t.Errorf("%d requests on the second run, want 1", hits)
	}
	if m, ok := store.Get(urls[0]); !ok || m.W != 800 || m.H != 600 {
		t.Errorf("cached entry: %+v, %v", m, ok)
	}

	store.Prune(map[string]bool{urls[0]: true})
	if _, ok := store.Get(urls[1]); ok {
		t.Error("pruned entry is still there")
	}
}

func TestProbeLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image", "a.jpg"), jpegBytes(t, 600, 900), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	os.Chdir(dir)

	m, err := (&Prober{}).Probe(context.Background(), "/image/a.jpg")
	if err != nil || m.W != 600 || m.H != 900 {
		t.Errorf("got %+v, %v; want 600x900", m, err)
	}
}

func TestStoreFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photos.json")
	store, _ := OpenStore(path)
	store.Put("https://x/b.jpg", Meta{W: 2, H: 1})
	store.Put("https://x/a.jpg", Meta{W: 1, H: 2, Exif: &Exif{Aperture: "ƒ/4", Lens: "A & B <1>"}})
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := `{
"https://x/a.jpg": {"w":1,"h":2,"exif":{"lens":"A & B <1>","aperture":"ƒ/4"}},
"https://x/b.jpg": {"w":2,"h":1}
}
`
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
