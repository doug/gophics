package apisurface

import (
	"fmt"
	"sort"
	"strings"
)

// Change is one API element whose signature moved while its name stayed.
//
// This is the case the manifest did not used to have a word for, and it is the
// most common break there is: the name a caller wrote still exists, still
// compiles here, and no longer accepts what they pass it.
type Change struct {
	Key  string // the dotted name, unchanged
	Kind string // type, field, method, ifacemethod, func, const, var, alias
	Was  string // the signature in the committed manifest
	Now  string // the signature in the tree
}

func (c Change) String() string {
	return fmt.Sprintf("%s %s\n        was: %s\n        now: %s", c.Key, c.Kind, c.Was, c.Now)
}

// Delta is the difference between a committed manifest and a scan of the tree.
type Delta struct {
	Added   []Entry
	Removed []Entry
	Changed []Change
}

// Empty reports whether the surface is unchanged.
func (d Delta) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Breaking lists the parts of the delta that can break a caller who compiled
// against the committed manifest.
//
// Three things qualify. A removal, obviously. A signature change on a name
// that stayed, which is the whole reason signatures are recorded. And an added
// interface method: it reads as an addition, a single + line, but every
// implementation of that interface outside this repo stops compiling — which
// is why ifacemethod is a kind of its own rather than a method.
//
// Everything else — a new function, a new type, a new struct field, a new
// method on a concrete type — is additive, and a v1 package may take it.
func (d Delta) Breaking() []string {
	var out []string
	for _, e := range d.Removed {
		out = append(out, "removed: "+string(e))
	}
	for _, c := range d.Changed {
		out = append(out, "signature changed: "+c.String())
	}
	for _, e := range d.Added {
		if e.Kind() == "ifacemethod" {
			out = append(out, "interface method added (breaks implementers): "+string(e))
		}
	}
	sort.Strings(out)
	return out
}

// Diff compares two surfaces by key, so that a signature change is reported as
// a change rather than as an unrelated removal plus addition.
//
// Keys are unique within a surface by construction: one line per API element,
// and the element's name is the key. A duplicate would mean the extractor
// emitted two renderings of one thing, so the later one wins and the pair
// shows up as a change — which is the loud outcome, and the right one for a
// bug in the extractor.
func Diff(have, want []Entry) Delta {
	h, hk := index(have)
	w, wk := index(want)

	var d Delta
	for _, k := range wk {
		switch old, ok := h[k]; {
		case !ok:
			d.Added = append(d.Added, w[k])
		case old != w[k]:
			d.Changed = append(d.Changed, Change{
				Key:  k,
				Kind: w[k].Kind(),
				Was:  old.Signature(),
				Now:  w[k].Signature(),
			})
		}
	}
	for _, k := range hk {
		if _, ok := w[k]; !ok {
			d.Removed = append(d.Removed, h[k])
		}
	}
	return d
}

func index(entries []Entry) (map[string]Entry, []string) {
	m := make(map[string]Entry, len(entries))
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if e == "" || strings.HasPrefix(string(e), "#") {
			continue
		}
		k := e.Key()
		if _, dup := m[k]; !dup {
			keys = append(keys, k)
		}
		m[k] = e
	}
	sort.Strings(keys)
	return m, keys
}

// ParseManifest reads a rendered manifest back into entries, dropping comments
// and blank lines. The round trip is what lets the gate diff by key instead of
// by line.
func ParseManifest(text string) []Entry {
	var out []Entry
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, Entry(line))
	}
	return out
}
