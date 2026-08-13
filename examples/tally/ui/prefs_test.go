package main

import (
	"path/filepath"
	"sort"
	"testing"
)

// fakePrefs is an in-memory shell.Preferences for testing the load logic.
type fakePrefs struct {
	m       map[string]string
	deleted []string
}

func newFakePrefs(kv map[string]string) *fakePrefs {
	m := map[string]string{}
	for k, v := range kv {
		m[k] = v
	}
	return &fakePrefs{m: m}
}

func (f *fakePrefs) Get(k string) (string, bool) { v, ok := f.m[k]; return v, ok }
func (f *fakePrefs) Set(k, v string) error       { f.m[k] = v; return nil }
func (f *fakePrefs) Delete(k string) error {
	delete(f.m, k)
	f.deleted = append(f.deleted, k)
	return nil
}
func (f *fakePrefs) Keys() []string {
	keys := make([]string, 0, len(f.m))
	for k := range f.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestLoadFallsBackToDemo covers the first-run path and, importantly, the mount
// ordering: the widget tree's first Build happens before the shell wires
// capabilities, so Preferences is nil then. The demo must load anyway (so the
// window isn't empty), without burning the one chance to consult Preferences.
func TestLoadFallsBackToDemo(t *testing.T) {
	s := &state{selected: -1}

	s.loadWith(nil) // the mount-time build: no capabilities yet
	if s.err != nil {
		t.Fatalf("demo failed to load: %v", s.err)
	}
	if s.book == nil || s.tree == nil {
		t.Fatal("no ledger after the first build")
	}
	if s.prefsChecked {
		t.Error("marked preferences as checked while they were still nil")
	}
	demoPath := s.book.Path

	// The frame after wiring: Preferences appears and the remembered ledger
	// replaces the demo.
	real := filepath.Join("testdata", "example.beancount")
	p := newFakePrefs(map[string]string{prefKeyLedger: real})
	s.loadWith(p)
	if !s.prefsChecked {
		t.Error("preferences were not consulted once available")
	}
	if s.book.Path == demoPath {
		t.Error("remembered ledger did not replace the embedded demo")
	}
	if s.book.Path != real {
		t.Errorf("loaded %q, want %q", s.book.Path, real)
	}
	if s.err != nil {
		t.Errorf("unexpected error: %v", s.err)
	}
}

// TestLoadDropsStaleLedgerPath: a remembered file that no longer exists must not
// wedge the app or nag every launch — fall back to the demo and forget the path.
func TestLoadDropsStaleLedgerPath(t *testing.T) {
	s := &state{selected: -1}
	p := newFakePrefs(map[string]string{prefKeyLedger: "/nonexistent/gone.beancount"})

	s.loadWith(p)
	if s.err != nil {
		t.Fatalf("a missing remembered file should fall back cleanly, got %v", s.err)
	}
	if s.book == nil {
		t.Fatal("no ledger loaded after falling back")
	}
	if _, ok := p.Get(prefKeyLedger); ok {
		t.Error("stale ledger path was not forgotten")
	}
	if len(p.deleted) != 1 || p.deleted[0] != prefKeyLedger {
		t.Errorf("expected the stale key to be deleted, got %v", p.deleted)
	}
}

// TestLoadIsIdempotent: once a ledger is open, repeated builds must not reload it
// (Build runs every frame).
func TestLoadIsIdempotent(t *testing.T) {
	s := &state{selected: -1}
	p := newFakePrefs(map[string]string{prefKeyLedger: filepath.Join("testdata", "example.beancount")})

	s.loadWith(p)
	first := s.book
	for i := 0; i < 5; i++ {
		s.loadWith(p)
	}
	if s.book != first {
		t.Error("ledger was reloaded on a later build")
	}
}
