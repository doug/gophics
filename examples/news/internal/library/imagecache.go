package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/doug/gophics/examples/news/internal/fetch"
)

// ImageCache keeps article pictures on disk so a queue prefetched over wifi
// reads the same way underground.
//
// gophics ships widget.NetworkImage, which caches decoded images in memory and
// fetches over HTTP. That is right for an app that is always online and wrong
// here: memory does not survive the app being backgrounded, and the fetch
// cannot happen at all on the train. This cache is the disk half, and ui.Img is
// the widget that draws from it.
type ImageCache struct {
	dir string

	mu       sync.Mutex
	mem      map[string]image.Image
	inFlight map[string]chan struct{}
	client   http.Client
}

// NewImageCache opens the cache directory.
func NewImageCache(dir string) *ImageCache {
	os.MkdirAll(dir, 0o700)
	return &ImageCache{
		dir:      dir,
		mem:      map[string]image.Image{},
		inFlight: map[string]chan struct{}{},
		client:   http.Client{Timeout: 30 * time.Second},
	}
}

// memLimit is how many decoded images to hold. Decoded bitmaps are far larger
// than their files, so this is deliberately much smaller than the disk cache;
// re-decoding from disk is fast and, unlike re-downloading, always possible.
const memLimit = 60

// Path is where a URL's bytes live on disk. Naming by hash keeps every
// publisher's query-string-laden CDN URL to one flat, valid filename.
func (c *ImageCache) Path(url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:])[:32])
}

// Have reports whether a URL is already downloaded.
func (c *ImageCache) Have(url string) bool {
	if url == "" {
		return false
	}
	return fileExists(c.Path(url))
}

// Fetch downloads one image if it is not already cached. Failures are not worth
// reporting upward: a missing picture must never break an article, and the next
// refresh will try again.
func (c *ImageCache) Fetch(ctx context.Context, url string) error {
	if url == "" || c.Have(url) {
		return nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("not an http url")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", fetch.DefaultUserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	// Write through a temporary file so an interrupted download can never be
	// mistaken later for a complete one.
	tmp := c.Path(url) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, maxImageBytes))
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, c.Path(url))
}

// maxImageBytes caps one download. Publishers occasionally link the 12-megapixel
// original; nothing on a phone screen needs it.
const maxImageBytes = 8 << 20

// Load returns the decoded image, from memory, then disk, then the network.
// It is called from a background goroutine by the Img widget, never from the
// frame loop.
func (c *ImageCache) Load(ctx context.Context, url string) (image.Image, error) {
	if url == "" {
		return nil, fmt.Errorf("no url")
	}
	c.mu.Lock()
	if img, ok := c.mem[url]; ok {
		c.mu.Unlock()
		return img, nil
	}
	// Single-flight: a fast scroll asks for the same picture from several rows
	// at once, and one download is enough.
	if ch, ok := c.inFlight[url]; ok {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		img, ok := c.mem[url]
		c.mu.Unlock()
		if ok {
			return img, nil
		}
		return c.decodeFile(url)
	}
	ch := make(chan struct{})
	c.inFlight[url] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.inFlight, url)
		c.mu.Unlock()
		close(ch)
	}()

	if !c.Have(url) {
		if err := c.Fetch(ctx, url); err != nil {
			return nil, err
		}
	}
	img, err := c.decodeFile(url)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.mem) >= memLimit {
		// Nothing clever: drop an arbitrary slice of the cache. Scrolling
		// re-decodes from disk in a millisecond and the alternative is tracking
		// access order on every frame.
		i := 0
		for k := range c.mem {
			delete(c.mem, k)
			if i++; i >= memLimit/2 {
				break
			}
		}
	}
	c.mem[url] = img
	c.mu.Unlock()
	return img, nil
}

// Cached returns an already-decoded image without touching disk or network, so
// a widget can draw on its first frame when the picture is warm.
func (c *ImageCache) Cached(url string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img, ok := c.mem[url]
	return img, ok
}

func (c *ImageCache) decodeFile(url string) (image.Image, error) {
	f, err := os.Open(c.Path(url))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		// A file that will not decode is worse than no file: it would be
		// retried from disk forever. Drop it so a later refresh refetches.
		os.Remove(c.Path(url))
		return nil, err
	}
	return img, nil
}

// Size reports the bytes on disk.
func (c *ImageCache) Size() int64 {
	var total int64
	entries, _ := os.ReadDir(c.dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			total += info.Size()
		}
	}
	return total
}

// Prune deletes least-recently-modified images until the cache fits in
// maxBytes. Called after a refresh so the cache cannot grow without bound on a
// phone that is never plugged into anything.
func (c *ImageCache) Prune(maxBytes int64) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type ent struct {
		path string
		size int64
		mod  time.Time
	}
	var all []ent
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, ent{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= maxBytes {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.Before(all[j].mod) })
	for _, e := range all {
		if total <= maxBytes {
			break
		}
		if os.Remove(e.path) == nil {
			total -= e.size
		}
	}
	c.mu.Lock()
	clear(c.mem)
	c.mu.Unlock()
}

// Clear empties the cache entirely, for the settings screen.
func (c *ImageCache) Clear() {
	entries, _ := os.ReadDir(c.dir)
	for _, e := range entries {
		os.Remove(filepath.Join(c.dir, e.Name()))
	}
	c.mu.Lock()
	clear(c.mem)
	c.mu.Unlock()
}
