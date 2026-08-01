package ui

import (
	"errors"
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

// Create adds a new note named name (a "# name" stub), writes it to disk, and
// returns it. If a note by that name already exists, it is returned unchanged.
// Returns an error for an empty or unsafe name.
func (v *Vault) Create(name string) (Note, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Note{}, errors.New("note name is empty")
	}
	if strings.ContainsAny(name, `/\`) {
		return Note{}, errors.New("note name cannot contain path separators")
	}
	if n, ok := v.ByName(name); ok {
		return n, nil
	}
	body := "# " + name + "\n\n"
	path := filepath.Join(v.Dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return Note{}, err
	}
	n := Note{Path: path, Name: name, Body: body}
	v.Notes = append(v.Notes, n)
	sort.Slice(v.Notes, func(i, j int) bool { return v.Notes[i].Name < v.Notes[j].Name })
	return n, nil
}

// Delete removes the note at path from disk and from the vault.
func (v *Vault) Delete(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	for i := range v.Notes {
		if v.Notes[i].Path == path {
			v.Notes = append(v.Notes[:i], v.Notes[i+1:]...)
			break
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

// Search returns the notes matching query (case-insensitive substring in name
// or body); an empty query returns every note. Order is preserved.
func (v *Vault) Search(query string) []Note {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return v.Notes
	}
	var out []Note
	for _, n := range v.Notes {
		if strings.Contains(strings.ToLower(n.Name), q) || strings.Contains(strings.ToLower(n.Body), q) {
			out = append(out, n)
		}
	}
	return out
}

// Backlinks returns the notes that link to name via a [[wikilink]].
func (v *Vault) Backlinks(name string) []Note {
	var out []Note
	for _, n := range v.Notes {
		if strings.EqualFold(n.Name, name) {
			continue
		}
		for _, tgt := range wikilinkTargets(n.Body) {
			if strings.EqualFold(tgt, name) {
				out = append(out, n)
				break
			}
		}
	}
	return out
}
