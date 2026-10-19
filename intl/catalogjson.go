package intl

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// The on-disk catalog format.
//
// A catalog has to leave the program to be translated and come back, and until
// now it could only be built by calling Set from Go — which means the only way
// to ship a translation was to compile it, and the only thing a translator
// could be handed was source code. This is the file the extractor writes and
// the app reads.
//
// JSON because it is in the standard library, every translation tool imports
// it, and a human can resolve a merge conflict in it. Not Gettext PO: the
// tooling is better but the format needs a parser, and a framework that is
// deliberately short on dependencies should not grow one to express a map of
// maps. Not Go source: a generated .go file cannot be loaded without a rebuild,
// which is exactly the constraint being removed.
//
// Keys are written sorted so that re-extracting an unchanged tree produces a
// byte-identical file and the diff is the translation change.

// catalogFile is the wire shape. Ordinals are a separate object for the same
// reason the Catalog keeps a separate map: cardinal and ordinal agreement are
// different rule sets over the same key.
type catalogFile struct {
	Lang     string                       `json:"lang"`
	Messages map[string]map[string]string `json:"messages,omitempty"`
	Ordinals map[string]map[string]string `json:"ordinals,omitempty"`
}

// WriteJSON writes the catalog as JSON.
func (c *Catalog) WriteJSON(w io.Writer) error {
	c.mu.RLock()
	f := catalogFile{
		Lang:     c.lang,
		Messages: encodeForms(c.msgs),
		Ordinals: encodeForms(c.ords),
	}
	c.mu.RUnlock()

	// SetEscapeHTML(false) because catalog text is prose, not HTML: a message
	// containing an ampersand or a quote should survive the round trip
	// readable, and & in a translator's editor is a bug report.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

// ReadJSON reads a catalog written by WriteJSON.
//
// An unknown plural category is an error rather than a silently dropped form.
// A translator who writes "singular" instead of "one" has produced a file whose
// singular never appears, and discovering that by reading the running UI in a
// language you do not speak is the worst possible place to find out.
func ReadJSON(r io.Reader) (*Catalog, error) {
	var f catalogFile
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("intl: reading catalog: %w", err)
	}
	if f.Lang == "" {
		return nil, fmt.Errorf("intl: catalog has no %q field", "lang")
	}
	c := NewCatalog(f.Lang)
	for _, set := range []struct {
		in   map[string]map[string]string
		out  map[string]Forms
		what string
	}{
		{f.Messages, c.msgs, "messages"},
		{f.Ordinals, c.ords, "ordinals"},
	} {
		for key, raw := range set.in {
			forms := Forms{}
			for name, text := range raw {
				cat, ok := ParsePlural(name)
				if !ok {
					return nil, fmt.Errorf("intl: %s[%q]: %q is not a CLDR plural category", set.what, key, name)
				}
				forms[cat] = text
			}
			set.out[key] = forms
		}
	}
	return c, nil
}

func encodeForms(m map[string]Forms) map[string]map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]map[string]string, len(m))
	for key, f := range m {
		raw := make(map[string]string, len(f))
		for cat, text := range f {
			raw[cat.String()] = text
		}
		out[key] = raw
	}
	return out
}

// Keys lists the catalog's message keys in sorted order, and the ordinal keys
// separately, which is what a merge needs: an extractor re-running over a
// changed tree has to tell a key that is still referenced from one that is not.
func (c *Catalog) Keys() (messages, ordinals []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for k := range c.msgs {
		messages = append(messages, k)
	}
	for k := range c.ords {
		ordinals = append(ordinals, k)
	}
	sort.Strings(messages)
	sort.Strings(ordinals)
	return messages, ordinals
}

// Forms returns a copy of the plural forms recorded for a message key, and
// whether the key is present. A copy because the caller must not be able to
// mutate a catalog that other goroutines are reading.
func (c *Catalog) Forms(key string) (Forms, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyForms(c.msgs[key])
}

// OrdinalForms is Forms for the ordinal namespace.
func (c *Catalog) OrdinalForms(key string) (Forms, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return copyForms(c.ords[key])
}

func copyForms(f Forms) (Forms, bool) {
	if f == nil {
		return nil, false
	}
	out := make(Forms, len(f))
	for k, v := range f {
		out[k] = v
	}
	return out, true
}
