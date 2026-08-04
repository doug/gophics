//go:build !js

package main

import (
	"os"
	"path/filepath"
)

// fileStore autosaves the game to a JSON file under the user's config dir.
type fileStore struct{ path string }

func platformStore() store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	d := filepath.Join(dir, "gophics-solitaire")
	_ = os.MkdirAll(d, 0o755)
	return &fileStore{path: filepath.Join(d, "game.json")}
}

func (f *fileStore) save(data []byte) { _ = os.WriteFile(f.path, data, 0o644) }

func (f *fileStore) load() ([]byte, bool) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, false
	}
	return data, true
}
