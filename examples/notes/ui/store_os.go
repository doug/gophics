//go:build !js

package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// osStore persists notes as .md files in a directory — the desktop and terminal
// backend.
type osStore struct{ dir string }

func (s *osStore) List() ([]Note, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var notes []Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		p := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		notes = append(notes, Note{
			Path: p,
			Name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Body: string(data),
		})
	}
	return notes, nil
}

func (s *osStore) Write(name, body string) (Note, error) {
	path := filepath.Join(s.dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return Note{}, err
	}
	return Note{Path: path, Name: name, Body: body}, nil
}

func (s *osStore) Remove(n Note) error { return os.Remove(n.Path) }

func (s *osStore) Label() string { return s.dir }

// LoadVault opens the vault at dir — a desktop convenience (used by tests and
// callers that already know the folder).
func LoadVault(dir string) (*Vault, error) { return OpenVault(&osStore{dir: dir}) }

// defaultStore is the OS filesystem vault: a folder is always available.
func defaultStore() store { return &osStore{dir: vaultDir()} }

func vaultDir() string {
	if d := os.Getenv("NOTES_DIR"); d != "" {
		return d
	}
	if _, err := os.Stat("examples/notes/vault"); err == nil {
		return "examples/notes/vault"
	}
	return "vault"
}

// openFolder is a no-op on desktop: a folder is always open, so the sidebar's
// open-folder prompt never appears.
func openFolder(*workspaceState) {}
