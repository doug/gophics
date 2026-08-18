// Package library is the application layer of the news reader: it owns where
// data lives, which feeds the reader is subscribed to, how a refresh runs, and
// how article images are cached for offline reading.
//
// The packages below it (catalog, feed, fetch, extract, store, pick) are a
// straight port of the rss command-line pipeline and know nothing about an app.
// Everything phone-shaped — a data directory handed in by the host, a refresh
// that reports progress to a UI, cookies captured from a login web view — lives
// here.
package library

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	dirMu   sync.RWMutex
	dataDir string
)

// SetDataDir fixes the directory holding the store, subscriptions, ranking
// model and image cache. Mobile hosts must call this before anything else:
// Android's app-private storage is only known to the Java side
// (Context.getFilesDir), and on iOS the sandbox path changes between installs.
//
// Desktop builds may leave it unset and get the user's config directory.
func SetDataDir(dir string) {
	dirMu.Lock()
	defer dirMu.Unlock()
	dataDir = dir
}

// DataDir returns the directory in use, creating it if needed. When no host has
// called SetDataDir it falls back to the platform config directory, and to the
// working directory if even that is unavailable — a reader that writes
// somewhere odd is better than one that cannot start.
func DataDir() string {
	dirMu.RLock()
	dir := dataDir
	dirMu.RUnlock()

	if dir == "" {
		if cfg, err := os.UserConfigDir(); err == nil {
			dir = filepath.Join(cfg, "gophics-news")
		} else {
			dir = ".gophics-news"
		}
		dirMu.Lock()
		if dataDir == "" {
			dataDir = dir
		}
		dir = dataDir
		dirMu.Unlock()
	}
	os.MkdirAll(dir, 0o700)
	return dir
}

// path joins a name onto the data directory.
func path(elem ...string) string {
	return filepath.Join(append([]string{DataDir()}, elem...)...)
}

// StorePath is the article store: one directory of JSON files per day.
func StorePath() string { return path("store") }

// SubscriptionsPath is the user's own feed list, as distinct from the
// suggestion catalog compiled into the binary.
func SubscriptionsPath() string { return path("subscriptions.json") }

// ModelPath is the learned ranking model.
func ModelPath() string { return path("ranking.json") }

// ImageCacheDir holds downloaded images, so a queue prefetched on wifi still
// renders with no connection at all.
func ImageCacheDir() string { return path("images") }

// CookieDir holds one file per publisher whose subscription the reader is
// using. Files are written 0600 and only ever sent to the host they came from.
func CookieDir() string { return path("cookies") }

// CookiePath is where the cookies for one domain live.
func CookiePath(domain string) string {
	return filepath.Join(CookieDir(), sanitizeDomain(domain)+".txt")
}

// sanitizeDomain keeps a domain usable as a filename without letting it escape
// the cookie directory.
func sanitizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "www.")
	var b strings.Builder
	for _, r := range d {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
