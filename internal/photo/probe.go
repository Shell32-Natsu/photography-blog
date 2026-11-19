package photo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// headBytes is how much of a file is read. JPEG keeps its size and EXIF
// block at the very start, so the rest of a multi-megabyte original never
// has to be downloaded.
const headBytes = 128 << 10

// Prober looks up the metadata of photos that are not in the cache yet.
type Prober struct {
	Client *http.Client
	// Thumb maps a photo URL to the URL of the image shown in the grid.
	Thumb func(url string) string
	// Workers is the number of photos fetched at once.
	Workers int
}

// IsRemote reports whether a photo URL points to another host rather than to
// a file inside the site.
func IsRemote(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// Fill probes every URL that store does not know yet and records the
// result. Photos that cannot be read are returned in failed and are not
// cached, so the next build tries them again.
func (p *Prober) Fill(ctx context.Context, urls []string, store *Store, progress func(done, total int)) (failed []string) {
	var todo []string
	for _, u := range urls {
		if _, ok := store.Get(u); !ok {
			todo = append(todo, u)
		}
	}
	if len(todo) == 0 {
		return nil
	}

	workers := p.Workers
	if workers <= 0 {
		workers = 16
	}
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		done int
		jobs = make(chan string)
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				m, err := p.Probe(ctx, u)
				mu.Lock()
				if err != nil {
					failed = append(failed, fmt.Sprintf("%s: %v", u, err))
				} else {
					store.Put(u, m)
				}
				done++
				if progress != nil {
					progress(done, len(todo))
				}
				mu.Unlock()
			}
		}()
	}
	for _, u := range todo {
		jobs <- u
	}
	close(jobs)
	wg.Wait()
	return failed
}

// Probe reads the size and shooting data of a single photo.
func (p *Prober) Probe(ctx context.Context, url string) (Meta, error) {
	fetch := p.fetch
	if !IsRemote(url) {
		fetch = readLocal
	}

	orig, origErr := fetch(ctx, url)
	var m Meta
	var orientation int
	m.Exif, orientation = readExif(orig)
	ow, oh, origOK := size(orig)
	if origOK && orientation >= 5 {
		ow, oh = oh, ow // browsers show the original rotated
	}

	thumbURL := url
	if p.Thumb != nil {
		thumbURL = p.Thumb(url)
	}
	if thumbURL == url {
		if !origOK {
			return m, firstErr(origErr, "unknown image format")
		}
		m.W, m.H = ow, oh
		return m, nil
	}

	thumb, thumbErr := fetch(ctx, thumbURL)
	tw, th, thumbOK := size(thumb)
	switch {
	case thumbOK:
		m.W, m.H = tw, th
		// A thumbnail made without applying the EXIF rotation is landscape
		// where the original is portrait, or the other way round.
		m.Sideways = origOK && (tw > th) != (ow > oh) && tw != th && ow != oh
	case origOK:
		m.W, m.H = ow, oh
	default:
		return m, firstErr(thumbErr, "unknown image format")
	}
	return m, nil
}

func (p *Prober) fetch(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		data, retry, err := p.fetchOnce(ctx, url)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, lastErr
}

func (p *Prober) fetchOnce(ctx context.Context, url string) (data []byte, retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", headBytes-1))
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests,
			fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, headBytes))
	if err != nil && len(data) == 0 {
		return nil, true, err
	}
	return data, false, nil
}

// readLocal reads a photo stored in the site itself. A URL such as
// "/image/a.jpg" refers to the file "image/a.jpg" in the working directory.
func readLocal(_ context.Context, url string) ([]byte, error) {
	f, err := os.Open(strings.TrimPrefix(url, "/"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 16*headBytes))
}

func firstErr(err error, fallback string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", fallback)
}
