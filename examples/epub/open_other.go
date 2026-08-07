//go:build !js

package main

import "os"

// openEPUB loads the file named by the EPUB_PATH environment variable and calls
// cb with its bytes. The framework has no native file dialog yet, so on desktop
// the path is supplied out of band, e.g.
//
//	EPUB_PATH=~/Books/moby-dick.epub go run ./examples/epub
//
// then tap "Open EPUB…". Unset (or an unreadable path) is a no-op, leaving the
// bundled sample in place. On mobile there is no env either, so it stays on the
// sample — the web build is the one with an interactive picker (open_web.go).
func openEPUB(cb func([]byte)) {
	path := os.Getenv("EPUB_PATH")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	cb(data)
}
