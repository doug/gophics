// Package extract finds translatable messages in Go source and turns them into
// a catalog stub.
//
// It lives under internal/ on purpose. The product is the command in
// intl/cmd/gophics-intl; publishing the walker would add a dozen names to a
// public surface the project measures and commits to, in exchange for a
// capability almost no app needs at runtime. Promoting it later is additive, and
// un-publishing it would not be.
//
// # How messages are recognised
//
// By call shape, against the go/ast of each file — not by a registration macro
// or a build-time hook, because neither exists in Go and both would make the
// catalog API worse to use. A call is a message when its selector is one of
// Get, Plural, Ordinal, Set, SetForms or SetOrdinalForms and its first argument
// is a string literal. The receiver is not type-checked: doing so would mean
// running go/types over the whole module, which turns a sub-second tool into a
// multi-second one and fails outright on a tree that does not compile — exactly
// the tree you are most likely to be extracting from mid-change. The cost of
// the looser rule is a false positive when some unrelated type has a Get(string)
// method, which shows up as a surplus key in the catalog: visible, harmless,
// and removable with a comment. The cost of the stricter rule is a tool that
// refuses to run.
//
// A call to Catalog.RelativeTime carries no key, so seeing one adds the whole
// relative-time key set from intl.RelativeKeys instead. That is the only
// special case.
package extract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/doug/gophics/intl"
)

// Kind distinguishes the agreement rules a message needs, which decides which
// plural forms a stub has to offer.
type Kind uint8

const (
	// Plain has no count: one string.
	Plain Kind = iota
	// Cardinal agrees by quantity — CLDR's cardinal rules.
	Cardinal
	// Ordinal agrees by position — CLDR's ordinal rules, a different set.
	Ordinal
)

func (k Kind) String() string {
	switch k {
	case Cardinal:
		return "cardinal"
	case Ordinal:
		return "ordinal"
	}
	return "plain"
}

// Message is one translatable message found in source.
type Message struct {
	Key  string
	Kind Kind
	// Refs are "file:line" call sites, sorted and deduplicated. They are what
	// makes a surplus key diagnosable: a translator or a reviewer can see where
	// the string is used, or that it no longer is.
	Refs []string
	// Source is the message text where the call site supplied one — a
	// Set("k", "text") or SetForms literal in the source language. Empty for a
	// Get or Plural, which only name a key.
	Source intl.Forms
}

// methodKind maps a recognised selector to the agreement its message needs.
var methodKind = map[string]Kind{
	"Get":             Plain,
	"Set":             Plain,
	"Plural":          Cardinal,
	"SetForms":        Cardinal,
	"Ordinal":         Ordinal,
	"SetOrdinalForms": Ordinal,
}

// Walk parses every non-test .go file under root and returns the messages it
// finds, sorted by key.
//
// Test files are skipped: their keys are fixtures, and a translator asked to
// localise "items" from a table test is being asked to localise a test. Vendor
// and dot directories are skipped for the usual reasons.
func Walk(fsys fs.FS, root string) ([]Message, error) {
	byKey := map[string]*Message{}
	fset := token.NewFileSet()

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		// Object resolution is skipped because only call shapes matter, and
		// skipping it is what keeps this fast enough to run on every build.
		//
		// A parse error stops the walk rather than being ignored. ParseFile
		// still returns a partial tree, so extraction *could* carry on and
		// report whatever it found — but what it found would be whatever
		// happened to precede the syntax error, and a catalog silently missing
		// the second half of a file is worse than no catalog. The error carries
		// the position, so the fix is obvious.
		file, perr := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if perr != nil {
			return fmt.Errorf("extract: %w", perr)
		}
		collect(fset, file, byKey)
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]Message, 0, len(byKey))
	for _, m := range byKey {
		sort.Strings(m.Refs)
		m.Refs = dedup(m.Refs)
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

func collect(fset *token.FileSet, file *ast.File, byKey map[string]*Message) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pos := fset.Position(call.Lparen)
		ref := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)

		if sel.Sel.Name == "RelativeTime" {
			// No key argument: the renderer derives its keys from the unit and
			// offset, so the whole set is in play.
			//
			// The past and future keys count ("3 days ago") and so need the
			// target language's full set of cardinal forms; the named offsets
			// are single words and need only one. Getting that backwards would
			// hand a Russian translator a single box for "{n} дней назад" and
			// no way to write the other three forms.
			for _, k := range intl.RelativeKeys() {
				kind := Plain
				if strings.HasSuffix(k, ".past") || strings.HasSuffix(k, ".future") {
					kind = Cardinal
				}
				add(byKey, k, kind, ref, nil)
			}
			return true
		}

		kind, ok := methodKind[sel.Sel.Name]
		if !ok || len(call.Args) == 0 {
			return true
		}
		key, ok := stringLit(call.Args[0])
		if !ok {
			return true
		}
		add(byKey, key, kind, ref, sourceForms(call, kind))
		return true
	})
}

// sourceForms recovers the source-language text a Set or SetForms call site
// supplied, so extraction from the source language produces a file a translator
// can read rather than a list of bare keys.
func sourceForms(call *ast.CallExpr, kind Kind) intl.Forms {
	if len(call.Args) < 2 {
		return nil
	}
	// Set(key, text)
	if s, ok := stringLit(call.Args[1]); ok {
		return intl.Forms{intl.Other: s}
	}
	// SetForms(key, intl.Forms{intl.One: "...", ...})
	lit, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		return nil
	}
	out := intl.Forms{}
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name := ""
		switch k := kv.Key.(type) {
		case *ast.Ident:
			name = k.Name
		case *ast.SelectorExpr:
			name = k.Sel.Name
		}
		cat, ok := intl.ParsePlural(name)
		if !ok {
			continue
		}
		if text, ok := stringLit(kv.Value); ok {
			out[cat] = text
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func add(byKey map[string]*Message, key string, kind Kind, ref string, src intl.Forms) {
	// Keyed by kind as well, because the same key legitimately carries a
	// cardinal and an ordinal message and they land in different namespaces.
	id := kind.String() + "\x00" + key
	m, ok := byKey[id]
	if !ok {
		m = &Message{Key: key, Kind: kind}
		byKey[id] = m
	}
	m.Refs = append(m.Refs, ref)
	if src != nil {
		if m.Source == nil {
			m.Source = intl.Forms{}
		}
		for k, v := range src {
			m.Source[k] = v
		}
	}
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func dedup(s []string) []string {
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// Catalog turns extracted messages into a stub for a target language.
//
// Each message gets exactly the plural forms that language uses — derived from
// the rules by intl.Categories, so a Russian stub has one/few/many/other and a
// Japanese one has only other. A translator is then never asked for a form the
// language does not have, which is where "translated by someone who does not
// speak it" usually starts.
//
// prev, when non-nil, supplies translations already done: a key it carries keeps
// its text, and a key it does not get the source text (for the source language)
// or an empty string. That is what makes re-extraction safe to run on every
// build — the point of extraction tooling is that it can be re-run.
func Catalog(lang string, msgs []Message, prev *intl.Catalog) *intl.Catalog {
	out := intl.NewCatalog(lang)
	cardinal, ordinal := intl.Categories(lang), intl.OrdinalCategories(lang)

	for _, m := range msgs {
		cats := []intl.Plural{intl.Other}
		switch m.Kind {
		case Cardinal:
			cats = cardinal
		case Ordinal:
			cats = ordinal
		}

		var old intl.Forms
		if prev != nil {
			if m.Kind == Ordinal {
				old, _ = prev.OrdinalForms(m.Key)
			} else {
				old, _ = prev.Forms(m.Key)
			}
		}

		forms := intl.Forms{}
		for _, cat := range cats {
			switch {
			case old[cat] != "":
				forms[cat] = old[cat]
			case m.Source[cat] != "":
				forms[cat] = m.Source[cat]
			case m.Source[intl.Other] != "":
				// A source that only supplied Other still tells a translator
				// what the message means, which is more use than "".
				forms[cat] = m.Source[intl.Other]
			default:
				forms[cat] = ""
			}
		}
		if m.Kind == Ordinal {
			out.SetOrdinalForms(m.Key, forms)
		} else {
			out.SetForms(m.Key, forms)
		}
	}
	return out
}

// Refs renders the call sites of every message as a comment block, for a tool
// that wants to show a translator where a string is used. Kept out of the JSON
// so the catalog file stays a translation and not a build artifact — file and
// line numbers churn on every edit and would make every diff unreadable.
func Refs(msgs []Message) string {
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", m.Kind, m.Key, strings.Join(shorten(m.Refs), " "))
	}
	return b.String()
}

func shorten(refs []string) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = filepath.ToSlash(r)
	}
	return out
}
