//go:build !js

// Desktop implementation of the preferences capability (shell/prefs.go): a JSON
// file under the user's config directory. No native API is needed — every desktop
// OS gives us a filesystem and a per-user config location — so this one
// implementation serves macOS, Linux and Windows alike.
package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/doug/gophics/shell"
)

// Preferences publishes the capability, or nil if the platform gives us nowhere
// to write (os.UserConfigDir fails when HOME/APPDATA is unset), so callers see the
// usual "unsupported here" rather than a store that silently drops writes.
func (w *window) Preferences() shell.Preferences {
	w.prefsOnce.Do(func() {
		dir, err := os.UserConfigDir()
		if err != nil {
			return
		}
		w.prefs = &filePrefs{path: filepath.Join(dir, appDirName(w.appID), "preferences.json")}
	})
	if w.prefs == nil {
		return nil // typed-nil guard: returning w.prefs directly would be non-nil
	}
	return w.prefs
}

// appDirName sanitizes an app identifier into a single directory name, falling
// back to the executable's name so an app that sets no identifier still gets a
// stable, recognizable location instead of a shared one.
func appDirName(id string) string {
	if id == "" {
		if exe, err := os.Executable(); err == nil {
			id = filepath.Base(exe)
		}
	}
	if id == "" {
		id = "gophics-app"
	}
	// Keep it a plain single path element.
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, id)
	if id == "" || id == "." || id == ".." {
		id = "gophics-app"
	}
	return id
}

// filePrefs is a JSON-file-backed store with an in-memory cache, so Get is cheap
// enough to call from Build while writes stay durable.
type filePrefs struct {
	path string

	mu     sync.RWMutex
	loaded bool
	values map[string]string
}

// load reads the file once. A missing file is an empty store; a corrupt one is
// also treated as empty rather than fatal — losing settings is recoverable, but
// refusing to start over a bad settings file is not something a user can fix.
func (p *filePrefs) load() {
	if p.loaded {
		return
	}
	p.loaded = true
	p.values = map[string]string{}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(data, &m) == nil && m != nil {
		p.values = m
	}
}

func (p *filePrefs) Get(key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	v, ok := p.values[key]
	return v, ok
}

func (p *filePrefs) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	p.values[key] = value
	return p.flush()
}

func (p *filePrefs) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	if _, ok := p.values[key]; !ok {
		return nil
	}
	delete(p.values, key)
	return p.flush()
}

func (p *filePrefs) Keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	keys := make([]string, 0, len(p.values))
	for k := range p.values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flush writes the whole store, atomically: settings are small, and a
// write-temp-then-rename keeps a crash mid-write from truncating the file into
// something unparsable. Callers hold the lock.
func (p *filePrefs) flush() error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p.values, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.path), ".preferences-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, p.path)
}
