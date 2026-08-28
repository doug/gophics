// Package prefs is the JSON-file preferences store shared by every shell that
// has a filesystem.
//
// It was desktop-only, which is the only reason mobile had no Preferences
// capability: the code was portable Go the whole time — a map, a cache and an
// atomic rename — sitting behind a `package desktop` build tag. Nothing in it
// is desktop-specific except the question of *which* directory to write in, and
// that is the one thing each shell answers for itself: os.UserConfigDir on the
// desktop, the host's documents directory on a phone.
package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// New returns a store backed by the JSON file at path. The file is read lazily
// on first access and rewritten whole on every change.
func New(path string) *Store { return &Store{path: path} }

// Store is a JSON-file-backed store with an in-memory cache, so Get is cheap
// enough to call from Build while writes stay durable.
type Store struct {
	path string

	mu     sync.RWMutex
	loaded bool
	values map[string]string
}

// load reads the file once. A missing file is an empty store; a corrupt one is
// also treated as empty rather than fatal — losing settings is recoverable, but
// refusing to start over a bad settings file is not something a user can fix.
func (p *Store) load() {
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

func (p *Store) Get(key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	v, ok := p.values[key]
	return v, ok
}

func (p *Store) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	p.values[key] = value
	return p.flush()
}

func (p *Store) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	if _, ok := p.values[key]; !ok {
		return nil
	}
	delete(p.values, key)
	return p.flush()
}

func (p *Store) Keys() []string {
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
func (p *Store) flush() error {
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
