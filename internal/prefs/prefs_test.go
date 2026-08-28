package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilePrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "preferences.json")
	p := New(path)

	if _, ok := p.Get("missing"); ok {
		t.Error("empty store returned a value")
	}
	if got := p.Keys(); len(got) != 0 {
		t.Errorf("empty store keys = %v", got)
	}

	if err := p.Set("ledger", "/Users/x/main.beancount"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := p.Set("theme", "dark"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if v, ok := p.Get("ledger"); !ok || v != "/Users/x/main.beancount" {
		t.Errorf("Get(ledger) = %q, %v", v, ok)
	}
	if got := p.Keys(); len(got) != 2 || got[0] != "ledger" || got[1] != "theme" {
		t.Errorf("Keys() = %v, want sorted [ledger theme]", got)
	}

	// A separate store over the same file must see what was written: this is the
	// property that makes settings survive a restart.
	fresh := New(path)
	if v, ok := fresh.Get("theme"); !ok || v != "dark" {
		t.Errorf("reloaded Get(theme) = %q, %v — settings did not persist", v, ok)
	}

	if err := p.Delete("theme"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := p.Get("theme"); ok {
		t.Error("deleted key still present")
	}
	if err := p.Delete("theme"); err != nil {
		t.Errorf("deleting an absent key should be a no-op, got %v", err)
	}
	reloaded := New(path)
	if _, ok := reloaded.Get("theme"); ok {
		t.Error("delete did not persist")
	}
}

// TestFilePrefsCorruptFile pins the recovery choice: a settings file we can't
// parse is treated as empty rather than fatal. Losing settings is recoverable;
// refusing to start because of a bad settings file is not something a user can fix.
func TestFilePrefsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte("{not json at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(path)
	if _, ok := p.Get("anything"); ok {
		t.Error("corrupt file yielded a value")
	}
	// And it must recover: writing works and replaces the bad file.
	if err := p.Set("k", "v"); err != nil {
		t.Fatalf("Set after corrupt read: %v", err)
	}
	if v, ok := (New(path)).Get("k"); !ok || v != "v" {
		t.Errorf("store did not recover from a corrupt file: %q %v", v, ok)
	}
}

// TestFilePrefsAtomicWrite checks the flush leaves no temp files behind — the
// rename must replace the target, not accumulate droppings next to it.
func TestFilePrefsAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := &Store{path: filepath.Join(dir, "preferences.json")}
	for i := 0; i < 5; i++ {
		if err := p.Set("k", "v"); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "preferences.json" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory contains %v, want only preferences.json", names)
	}
}
