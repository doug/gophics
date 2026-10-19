package extract

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/doug/gophics/intl"
)

// A small app's worth of call sites, as an in-memory tree. fstest.MapFS rather
// than testdata files because the fixture *is* Go source: a real .go file under
// testdata would be compiled by `go build ./...` and would have to be valid,
// importable and vet-clean, which is a lot of ceremony for a string the
// extractor only ever parses.
var tree = fstest.MapFS{
	"ui/cart.go": &fstest.MapFile{Data: []byte(`package ui

import "github.com/doug/gophics/intl"

func build(c *intl.Catalog) {
	_ = c.Get("checkout.button")
	_ = c.Plural("cart.items", 3)
	_ = c.Ordinal("cart.place", 1)
	_ = c.Get("checkout.button") // a second reference to the same key
}
`)},
	"ui/seed.go": &fstest.MapFile{Data: []byte(`package ui

import "github.com/doug/gophics/intl"

func seed(c *intl.Catalog) {
	c.Set("checkout.button", "Check out")
	c.SetForms("cart.items", intl.Forms{
		intl.One:   "{n} item",
		intl.Other: "{n} items",
	})
	c.SetOrdinalForms("cart.place", intl.Forms{
		intl.One:   "{n}st",
		intl.Two:   "{n}nd",
		intl.Few:   "{n}rd",
		intl.Other: "{n}th",
	})
}
`)},
	// Must be skipped: a key in a test is a fixture, not a string to localise.
	"ui/cart_test.go": &fstest.MapFile{Data: []byte(`package ui

func TestX() { _ = "never.extracted" }
`)},
	// Must be skipped by directory.
	"vendor/other/x.go": &fstest.MapFile{Data: []byte(`package other

func f(c interface{ Get(string) string }) { _ = c.Get("vendored.key") }
`)},
	"testdata/x.go": &fstest.MapFile{Data: []byte(`package testdata

func f(c interface{ Get(string) string }) { _ = c.Get("fixture.key") }
`)},
}

func find(msgs []Message, key string, kind Kind) *Message {
	for i := range msgs {
		if msgs[i].Key == key && msgs[i].Kind == kind {
			return &msgs[i]
		}
	}
	return nil
}

func TestWalkFindsKeysAndKinds(t *testing.T) {
	msgs, err := Walk(tree, ".")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		key  string
		kind Kind
	}{
		{"checkout.button", Plain},
		{"cart.items", Cardinal},
		{"cart.place", Ordinal},
	} {
		if find(msgs, c.key, c.kind) == nil {
			t.Errorf("%s (%s) was not extracted", c.key, c.kind)
		}
	}

	for _, key := range []string{"never.extracted", "vendored.key", "fixture.key"} {
		for _, kind := range []Kind{Plain, Cardinal, Ordinal} {
			if m := find(msgs, key, kind); m != nil {
				t.Errorf("%s should have been skipped, found at %v", key, m.Refs)
			}
		}
	}
}

// Refs are what make a surplus key diagnosable, so repeated call sites have to
// accumulate and a repeat of the same line must not.
func TestWalkCollectsCallSites(t *testing.T) {
	msgs, err := Walk(tree, ".")
	if err != nil {
		t.Fatal(err)
	}
	m := find(msgs, "checkout.button", Plain)
	if m == nil {
		t.Fatal("key missing")
	}
	// Two Gets in cart.go, one Set in seed.go.
	if len(m.Refs) != 3 {
		t.Errorf("Refs = %v, want three distinct call sites", m.Refs)
	}
	for i := 1; i < len(m.Refs); i++ {
		if m.Refs[i] <= m.Refs[i-1] {
			t.Errorf("Refs are not sorted and deduplicated: %v", m.Refs)
		}
	}
}

// Extracting from the source language should hand a translator the text, not a
// list of bare keys. The Set and SetForms literals are where it comes from.
func TestWalkRecoversSourceText(t *testing.T) {
	msgs, err := Walk(tree, ".")
	if err != nil {
		t.Fatal(err)
	}
	if m := find(msgs, "checkout.button", Plain); m == nil || m.Source[intl.Other] != "Check out" {
		t.Errorf("plain source text = %v", m)
	}
	m := find(msgs, "cart.items", Cardinal)
	if m == nil {
		t.Fatal("cart.items missing")
	}
	if m.Source[intl.One] != "{n} item" || m.Source[intl.Other] != "{n} items" {
		t.Errorf("SetForms literal was not recovered: %v", m.Source)
	}
	o := find(msgs, "cart.place", Ordinal)
	if o == nil || o.Source[intl.Few] != "{n}rd" {
		t.Errorf("ordinal forms were not recovered: %v", o)
	}
}

// A RelativeTime call has no key, so it brings the whole key set with it —
// otherwise a translator gets no way to localise a timestamp column.
func TestWalkExpandsRelativeTime(t *testing.T) {
	fsys := fstest.MapFS{"a.go": &fstest.MapFile{Data: []byte(`package a

import "time"

func f(c interface {
	RelativeTime(time.Time, time.Time, int) string
}, t time.Time) {
	_ = c.RelativeTime(t, time.Now(), 0)
}
`)}}
	msgs, err := Walk(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, k := range intl.RelativeKeys() {
		want[k] = true
	}
	got := 0
	for _, m := range msgs {
		if !want[m.Key] {
			continue
		}
		got++
		// A counting key needs the language's plural forms; a named offset is
		// one word. Handing a Russian translator a single box for
		// "{n} дней назад" would lose three of the four forms.
		wantKind := Plain
		if strings.HasSuffix(m.Key, ".past") || strings.HasSuffix(m.Key, ".future") {
			wantKind = Cardinal
		}
		if m.Kind != wantKind {
			t.Errorf("%s extracted as %s, want %s", m.Key, m.Kind, wantKind)
		}
	}
	if got != len(want) {
		t.Errorf("extracted %d of %d relative-time keys", got, len(want))
	}

	// And the stub a translator receives really does have the forms.
	ru := Catalog("ru", msgs, nil)
	f, ok := ru.Forms("reltime.day.past")
	if !ok || len(f) != len(intl.Categories("ru")) {
		t.Errorf("ru stub for reltime.day.past has %d forms, want %d", len(f), len(intl.Categories("ru")))
	}
	if f, ok := ru.Forms("reltime.day.-1"); !ok || len(f) != 1 {
		t.Errorf("ru stub for the named offset has %d forms, want 1", len(f))
	}
}

// A stub must ask for exactly the forms the target language uses — no more, so
// a translator is never asked for a category the language does not have, and no
// fewer, so none is missed.
func TestCatalogStubMatchesLanguageCategories(t *testing.T) {
	msgs, err := Walk(tree, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"ja", "en", "ru", "cy"} {
		c := Catalog(lang, msgs, nil)
		f, ok := c.Forms("cart.items")
		if !ok {
			t.Fatalf("%s: cart.items missing from the stub", lang)
		}
		if len(f) != len(intl.Categories(lang)) {
			t.Errorf("%s: stub has %d cardinal forms, language uses %d",
				lang, len(f), len(intl.Categories(lang)))
		}
		o, ok := c.OrdinalForms("cart.place")
		if !ok {
			t.Fatalf("%s: cart.place missing from the stub", lang)
		}
		if len(o) != len(intl.OrdinalCategories(lang)) {
			t.Errorf("%s: stub has %d ordinal forms, language uses %d",
				lang, len(o), len(intl.OrdinalCategories(lang)))
		}
	}
	// Japanese needs one cardinal form where Welsh needs six, and the stub has
	// to reflect that or the whole exercise is cosmetic.
	ja, _ := Catalog("ja", msgs, nil).Forms("cart.items")
	cy, _ := Catalog("cy", msgs, nil).Forms("cart.items")
	if len(ja) != 1 || len(cy) != 6 {
		t.Errorf("ja %d forms, cy %d forms", len(ja), len(cy))
	}
}

// The source-language stub is filled in from the call sites, so `-lang en` on
// the first run produces a readable file rather than a list of empty strings.
func TestCatalogStubCarriesSourceText(t *testing.T) {
	msgs, _ := Walk(tree, ".")
	en := Catalog("en", msgs, nil)
	if got := en.Get("checkout.button"); got != "Check out" {
		t.Errorf("Get = %q, want the source text", got)
	}
	if got := en.Plural("cart.items", 1); got != "1 item" {
		t.Errorf("Plural(1) = %q", got)
	}
	if got := en.Ordinal("cart.place", 23); got != "23rd" {
		t.Errorf("Ordinal(23) = %q", got)
	}
}

// Re-extraction is the point of extraction tooling, so a second run must keep
// translations already done, fill in keys that are new, and drop keys the
// source no longer references.
func TestExtractionIsReRunnable(t *testing.T) {
	msgs, _ := Walk(tree, ".")

	// A translator's work on the first stub.
	de := Catalog("de", msgs, nil)
	de.SetForms("cart.items", intl.Forms{
		intl.One:   "{n} Artikel",
		intl.Other: "{n} Artikel",
	})
	de.Set("checkout.button", "Zur Kasse")

	// Through the file and back, as it would travel.
	var buf bytes.Buffer
	if err := de.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	prev, err := intl.ReadJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// The source changes: one key appears, one disappears.
	next := append([]Message(nil), msgs...)
	next = append(next, Message{Key: "cart.empty", Kind: Plain, Source: intl.Forms{intl.Other: "Your cart is empty"}})
	for i, m := range next {
		if m.Key == "cart.place" {
			next = append(next[:i], next[i+1:]...)
			break
		}
	}

	merged := Catalog("de", next, prev)

	if got := merged.Get("checkout.button"); got != "Zur Kasse" {
		t.Errorf("a finished translation was lost: %q", got)
	}
	if got := merged.Plural("cart.items", 1); got != "1 Artikel" {
		t.Errorf("a finished plural was lost: %q", got)
	}
	// A new key arrives with the source text as a hint, not silently absent.
	if got := merged.Get("cart.empty"); got != "Your cart is empty" {
		t.Errorf("new key = %q", got)
	}
	// A key the source no longer references is gone, which is what keeps a
	// catalog from accreting dead strings forever.
	if _, ok := merged.OrdinalForms("cart.place"); ok {
		t.Error("a key the source no longer references survived the merge")
	}

	// And the merge is stable: running it again changes nothing.
	var a, b bytes.Buffer
	if err := merged.WriteJSON(&a); err != nil {
		t.Fatal(err)
	}
	again, err := intl.ReadJSON(bytes.NewReader(a.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := Catalog("de", next, again).WriteJSON(&b); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Errorf("re-running the merge churned the file:\n%s\nvs\n%s", a.String(), b.String())
	}
}

func TestRefsListsEveryMessage(t *testing.T) {
	msgs, _ := Walk(tree, ".")
	out := Refs(msgs)
	for _, want := range []string{"checkout.button", "cart.items", "cart.place", "cardinal", "ordinal", "plain"} {
		if !strings.Contains(out, want) {
			t.Errorf("Refs output does not mention %q:\n%s", want, out)
		}
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != len(msgs) {
		t.Errorf("Refs printed %d lines for %d messages", lines, len(msgs))
	}
}

// A file that does not parse must be reported with its position rather than
// skipped, because a silently skipped file is a silently missing translation.
func TestWalkReportsParseErrors(t *testing.T) {
	fsys := fstest.MapFS{"broken.go": &fstest.MapFile{Data: []byte("package a\n\nfunc {")}}
	if _, err := Walk(fsys, "."); err == nil {
		t.Error("Walk accepted a file that does not parse")
	}
}

func TestKindNames(t *testing.T) {
	for k, want := range map[Kind]string{Plain: "plain", Cardinal: "cardinal", Ordinal: "ordinal"} {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d) = %q, want %q", k, got, want)
		}
	}
}
