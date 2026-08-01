package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Note is one markdown file in the vault.
type Note struct {
	Path string // absolute path on disk
	Name string // base name without .md (display + [[wikilink]] target)
	Body string // file contents
}

// Vault is a folder of .md notes — the app's whole data model, held in memory
// and written back to disk on save. Deliberately plain data.
type Vault struct {
	Dir   string
	Notes []Note
}

// LoadVault reads every .md file in dir (non-recursive), sorted by name.
func LoadVault(dir string) (*Vault, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	v := &Vault{Dir: dir}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v.Notes = append(v.Notes, Note{
			Path: p,
			Name: strings.TrimSuffix(e.Name(), ".md"),
			Body: string(data),
		})
	}
	sort.Slice(v.Notes, func(i, j int) bool { return v.Notes[i].Name < v.Notes[j].Name })
	return v, nil
}

// Save writes body to the note at path and updates the in-memory copy.
func (v *Vault) Save(path, body string) error {
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	for i := range v.Notes {
		if v.Notes[i].Path == path {
			v.Notes[i].Body = body
			return nil
		}
	}
	return nil
}

// Get returns the note at path (by absolute path).
func (v *Vault) Get(path string) (Note, bool) {
	for _, n := range v.Notes {
		if n.Path == path {
			return n, true
		}
	}
	return Note{}, false
}

// ByName returns the note whose display name matches (case-insensitive) — the
// [[wikilink]] resolver.
func (v *Vault) ByName(name string) (Note, bool) {
	for _, n := range v.Notes {
		if strings.EqualFold(n.Name, name) {
			return n, true
		}
	}
	return Note{}, false
}
