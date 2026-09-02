package ui

import (
	"errors"
	"sort"
	"strings"
)

// Note is one markdown file in the vault.
type Note struct {
	Path string // stable identity: the file path on desktop, the note name on web
	Name string // base name without .md (display + [[wikilink]] target)
	Body string // file contents
}

// store persists a vault's notes. A local directory backs it on desktop
// (store_os.go); a folder the user picked backs it anywhere the FolderPicker
// capability exists (folderstore.go). Everything else about a Vault is pure
// in-memory logic that works identically on every platform.
//
// Loading is not part of this. Both backings already produce their notes while
// opening — one reads a directory, the other is handed a folder — so a List
// method here would have been a second way to do it that only one caller used.
type store interface {
	Write(name, body string) (Note, error) // create or overwrite; returns the note with its Path
	Remove(n Note) error                   // delete the note's file
	Label() string                         // folder path/name, for display
}

// Vault is a folder of .md notes — the app's whole data model, held in memory
// and written back through its store. Deliberately plain data.
type Vault struct {
	store store
	Notes []Note
}

// newVault holds notes that have already been loaded. A nil store yields an
// empty vault — the starting state before the user opens a folder.
func newVault(s store, notes []Note) *Vault {
	v := &Vault{store: s, Notes: notes}
	sortNotes(v.Notes)
	return v
}

// HasStore reports whether a backing folder is open — always true on desktop,
// false on web until the user opens one.
func (v *Vault) HasStore() bool { return v.store != nil }

// Label is the open folder's path or name, for display.
func (v *Vault) Label() string {
	if v.store == nil {
		return ""
	}
	return v.store.Label()
}

// adopt swaps in a freshly opened store and its notes (the web open-folder flow).
func (v *Vault) adopt(s store, notes []Note) {
	v.store = s
	v.Notes = notes
	sortNotes(v.Notes)
}

func sortNotes(ns []Note) {
	sort.Slice(ns, func(i, j int) bool { return ns[i].Name < ns[j].Name })
}

// Save writes body to the note identified by path and updates the in-memory copy.
func (v *Vault) Save(path, body string) error {
	if v.store == nil {
		return errors.New("no folder open")
	}
	n, ok := v.Get(path)
	if !ok {
		return errors.New("note not found")
	}
	if _, err := v.store.Write(n.Name, body); err != nil {
		return err
	}
	for i := range v.Notes {
		if v.Notes[i].Path == path {
			v.Notes[i].Body = body
			break
		}
	}
	return nil
}

// Create adds a new note named name (a "# name" stub), writes it, and returns
// it. An existing note by that name is returned unchanged. Returns an error for
// an empty or unsafe name, or when no folder is open.
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
	if v.store == nil {
		return Note{}, errors.New("no folder open")
	}
	n, err := v.store.Write(name, "# "+name+"\n\n")
	if err != nil {
		return Note{}, err
	}
	v.Notes = append(v.Notes, n)
	sortNotes(v.Notes)
	return n, nil
}

// Delete removes the note identified by path from its folder and the vault.
func (v *Vault) Delete(path string) error {
	if v.store == nil {
		return errors.New("no folder open")
	}
	n, ok := v.Get(path)
	if !ok {
		return errors.New("note not found")
	}
	if err := v.store.Remove(n); err != nil {
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

// Get returns the note with the given identity path.
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
