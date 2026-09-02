package apptest

import (
	"sort"
	"sync"

	"github.com/doug/gophics/shell"
)

// Prefs is an in-memory shell.Preferences for tests.
//
// It exists because three examples had each hand-rolled the same fake within
// a week of each other — solitaire, notes and tally — and the third copy had
// already drifted (it tracked deletions; the others could not). Test doubles
// are the one place copy-paste drift is invisible until it matters: a test
// against a fake that lost a behavior passes against code that broke it.
//
// Deleted records every Delete in order, because "the stale entry was dropped"
// is an assertion two of those examples actually make.
type Prefs struct {
	mu      sync.Mutex
	m       map[string]string
	Deleted []string
}

// NewPrefs returns a fake seeded with kv; nil means empty.
func NewPrefs(kv map[string]string) *Prefs {
	m := map[string]string{}
	for k, v := range kv {
		m[k] = v
	}
	return &Prefs{m: m}
}

func (p *Prefs) Get(key string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[key]
	return v, ok
}

func (p *Prefs) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m[key] = value
	return nil
}

func (p *Prefs) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, key)
	p.Deleted = append(p.Deleted, key)
	return nil
}

func (p *Prefs) Keys() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := make([]string, 0, len(p.m))
	for k := range p.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// interface conformance, checked at compile time rather than first use
var _ shell.Preferences = (*Prefs)(nil)
