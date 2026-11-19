// Command photoblog generates the static site from config.json.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/photography-blog/internal/config"
	"github.com/photography-blog/internal/photo"
	"github.com/photography-blog/internal/site"
)

func main() {
	var (
		configPath = flag.String("config", "config.json", "albums and photos to publish")
		themeDir   = flag.String("theme", "template", "theme directory")
		outDir     = flag.String("out", "public", "output directory")
		imageDir   = flag.String("images", "image", "directory of locally stored photos, copied to the output if it exists")
		metaPath   = flag.String("meta", "photos.json", "cache of photo sizes and EXIF data")
		offline    = flag.Bool("offline", false, "do not look up photos that are missing from the cache")
		refresh    = flag.Bool("refresh", false, "look up every photo again, ignoring the cache")
	)
	flag.Parse()
	log.SetFlags(0)

	root, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	urls := root.Photos()

	store, err := photo.OpenStore(*metaPath)
	if err != nil {
		log.Fatal(err)
	}
	keep := make(map[string]bool, len(urls))
	for _, u := range urls {
		keep[u] = true
	}
	if *refresh {
		keep = nil
	}
	store.Prune(keep)

	if !*offline {
		prober := &photo.Prober{
			Client: &http.Client{Timeout: 30 * time.Second},
			Thumb:  site.ThumbURL,
		}
		failed := prober.Fill(context.Background(), urls, store, func(done, total int) {
			if done%50 == 0 || done == total {
				fmt.Fprintf(os.Stderr, "\rreading photos %d/%d", done, total)
				if done == total {
					fmt.Fprintln(os.Stderr)
				}
			}
		})
		for _, f := range failed {
			log.Printf("warning: %s", f)
		}
		if err := store.Save(); err != nil {
			log.Fatal(err)
		}
	}
	for _, u := range urls {
		if m, _ := store.Get(u); m.Sideways {
			log.Printf("warning: thumbnail is not rotated like the original: %s", u)
		}
	}

	home := site.Build(root, store.Get)
	if err := site.Render(home, *themeDir, *outDir); err != nil {
		log.Fatal(err)
	}
	if err := site.CopyImages(*imageDir, *outDir); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d albums, %d photos → %s\n", home.Site.Albums, home.Site.Photos, *outDir)
}
