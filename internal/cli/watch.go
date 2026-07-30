package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// watchSource polls the tree under dir for changes to source/asset files and
// sends on the returned channel (coalescing bursts by debounce). It's a
// dependency-free mtime scan — plenty for a dev tool on a Go project. Call the
// returned stop func to end watching.
func watchSource(dir string, debounce time.Duration) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		last := snapshot(dir)
		t := time.NewTicker(300 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if cur := snapshot(dir); cur != last {
					time.Sleep(debounce) // let the save burst settle
					last = snapshot(dir)
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	return ch, func() { close(done) }
}

func snapshot(dir string) string {
	var sb strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "build" || name == "node_modules" ||
				(strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !watchedFile(name) {
			return nil
		}
		if info, err := d.Info(); err == nil {
			fmt.Fprintf(&sb, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		}
		return nil
	})
	return sb.String()
}

func watchedFile(name string) bool {
	switch filepath.Ext(name) {
	case ".go", ".html", ".css", ".js", ".wgsl", ".ttf", ".otf":
		return true
	}
	return false
}
