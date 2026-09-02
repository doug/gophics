package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// osStore persists notes as .md files in a local directory — the desktop and
// terminal backing, and what LoadVault opens.
type osStore struct{ dir string }

func (s *osStore) Write(name, body string) (Note, error) {
	path := filepath.Join(s.dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return Note{}, err
	}
	return Note{Path: path, Name: name, Body: body}, nil
}

func (s *osStore) Remove(n Note) error { return os.Remove(n.Path) }

func (s *osStore) Label() string { return s.dir }

// readNotes loads every .md file in dir.
func readNotes(dir string) ([]Note, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var notes []Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		notes = append(notes, Note{Path: p, Name: noteName(e.Name()), Body: string(data)})
	}
	return notes, nil
}

// LoadVault opens the vault at dir — used by tests and by callers that already
// know the folder.
func LoadVault(dir string) (*Vault, error) {
	notes, err := readNotes(dir)
	if err != nil {
		return newVault(nil, nil), err
	}
	return newVault(&osStore{dir: dir}, notes), nil
}

// defaultVault is the vault the app starts with.
//
// There is no build tag here, and that is the point: the question "is there a
// local folder of notes?" has a runtime answer on every platform. Desktop finds
// one and opens it. A browser has no local filesystem, so ReadDir fails and the
// vault starts empty — which is exactly the state the sidebar's open-folder
// prompt exists for. Compiling two versions of the app to reach the same two
// outcomes was the more complicated way to be less honest about it.
func defaultVault() *Vault {
	v, err := LoadVault(vaultDir())
	if err != nil {
		return newVault(nil, nil)
	}
	return v
}

func vaultDir() string {
	if d := os.Getenv("NOTES_DIR"); d != "" {
		return d
	}
	if _, err := os.Stat("examples/notes/vault"); err == nil {
		return "examples/notes/vault"
	}
	return "vault"
}
