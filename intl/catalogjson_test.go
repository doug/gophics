package intl

import (
	"bytes"
	"strings"
	"testing"
)

// A catalog has to leave the program to be translated and come back. These
// tests are about that trip, which is the only part of the pipeline where a
// mistake is silent: a form dropped on the way out looks exactly like a form a
// translator never filled in.
func TestCatalogJSONRoundTrip(t *testing.T) {
	in := NewCatalog("ru-RU")
	in.SetForms("cart.items", Forms{
		One:   "{n} товар",
		Few:   "{n} товара",
		Many:  "{n} товаров",
		Other: "{n} товара",
	})
	in.Set("checkout.button", "Оформить заказ")
	in.SetOrdinalForms("place", Forms{Other: "{n}-й"})

	var buf bytes.Buffer
	if err := in.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := ReadJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// The language subtag survives, reduced as NewCatalog reduces it.
	if out.Lang() != "ru" {
		t.Errorf("Lang() = %q, want ru", out.Lang())
	}
	// Every form, selected through the language's real rules.
	for n, want := range map[int]string{
		1: "1 товар", 3: "3 товара", 5: "5 товаров", 11: "11 товаров", 21: "21 товар",
	} {
		if got := out.Plural("cart.items", n); got != want {
			t.Errorf("after round trip, Plural(%d) = %q, want %q", n, got, want)
		}
	}
	if got := out.Get("checkout.button"); got != "Оформить заказ" {
		t.Errorf("Get = %q", got)
	}
	if got := out.Ordinal("place", 3); got != "3-й" {
		t.Errorf("Ordinal = %q", got)
	}

	// Writing the round-tripped catalog again must produce the same bytes, or
	// re-extraction churns the diff on every run and translators stop trusting
	// it.
	var again bytes.Buffer
	if err := out.WriteJSON(&again); err != nil {
		t.Fatal(err)
	}
	if again.String() != buf.String() {
		t.Errorf("write/read/write is not stable:\n%s\nvs\n%s", buf.String(), again.String())
	}
}

// The cardinal and ordinal namespaces must stay apart across the file, or a
// key that carries both loses one of them.
func TestCatalogJSONKeepsNamespacesApart(t *testing.T) {
	in := NewCatalog("en")
	in.SetForms("day", Forms{One: "{n} day", Other: "{n} days"})
	in.SetOrdinalForms("day", Forms{One: "{n}st day", Two: "{n}nd day", Few: "{n}rd day", Other: "{n}th day"})

	var buf bytes.Buffer
	if err := in.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := ReadJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Plural("day", 3); got != "3 days" {
		t.Errorf("Plural = %q", got)
	}
	if got := out.Ordinal("day", 3); got != "3rd day" {
		t.Errorf("Ordinal = %q", got)
	}
}

// Prose must survive readable. An ampersand written as a \u0026 escape in a
// translator's editor is a bug report, which is why WriteJSON turns
// SetEscapeHTML off: catalog text is prose, not HTML.
func TestCatalogJSONDoesNotEscapeProse(t *testing.T) {
	c := NewCatalog("en")
	c.Set("terms", `Accept the "Terms & Conditions" <here>`)
	var buf bytes.Buffer
	if err := c.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	// Matching on the escape's digits rather than the backslash form keeps
	// this assertion out of Go's own escape rules.
	if strings.Contains(buf.String(), "u0026") || strings.Contains(buf.String(), "u003c") {
		t.Errorf("prose was HTML-escaped:\n%s", buf.String())
	}
	out, err := ReadJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Get("terms"); got != `Accept the "Terms & Conditions" <here>` {
		t.Errorf("round trip changed the text: %q", got)
	}
}

// A misspelled category is an error, not a silently dropped form. A translator
// who writes "singular" has produced a file whose singular never appears, and
// the worst place to find that out is a running UI in a language you do not
// read.
func TestReadJSONRejectsUnknownCategory(t *testing.T) {
	const bad = `{"lang":"en","messages":{"items":{"singular":"{n} item","other":"{n} items"}}}`
	if _, err := ReadJSON(strings.NewReader(bad)); err == nil {
		t.Error("ReadJSON accepted a non-CLDR category name")
	} else if !strings.Contains(err.Error(), "singular") {
		t.Errorf("the error should name the offending category: %v", err)
	}
}

func TestReadJSONRequiresLang(t *testing.T) {
	if _, err := ReadJSON(strings.NewReader(`{"messages":{}}`)); err == nil {
		t.Error("ReadJSON accepted a catalog with no language")
	}
	if _, err := ReadJSON(strings.NewReader(`not json`)); err == nil {
		t.Error("ReadJSON accepted malformed input")
	}
}

// An empty catalog round-trips, which the first extraction run produces.
func TestCatalogJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := NewCatalog("de").WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	out, err := ReadJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if out.Lang() != "de" {
		t.Errorf("Lang() = %q", out.Lang())
	}
	msgs, ords := out.Keys()
	if len(msgs) != 0 || len(ords) != 0 {
		t.Errorf("empty catalog came back with %v / %v", msgs, ords)
	}
}

// Forms hands out a copy: a caller must not be able to mutate a catalog other
// goroutines are reading.
func TestFormsReturnsACopy(t *testing.T) {
	c := NewCatalog("en")
	c.SetForms("items", Forms{One: "{n} item", Other: "{n} items"})
	f, ok := c.Forms("items")
	if !ok {
		t.Fatal("Forms reported the key absent")
	}
	f[One] = "mutated"
	if got := c.Plural("items", 1); got != "1 item" {
		t.Errorf("mutating the returned Forms changed the catalog: %q", got)
	}
	if _, ok := c.Forms("absent"); ok {
		t.Error("Forms reported an absent key present")
	}
	if _, ok := c.OrdinalForms("items"); ok {
		t.Error("OrdinalForms found a cardinal key")
	}
}

func TestKeysAreSorted(t *testing.T) {
	c := NewCatalog("en")
	for _, k := range []string{"z", "a", "m"} {
		c.Set(k, k)
		c.SetOrdinalForms(k, Forms{Other: k})
	}
	msgs, ords := c.Keys()
	for _, got := range [][]string{msgs, ords} {
		if len(got) != 3 || got[0] != "a" || got[1] != "m" || got[2] != "z" {
			t.Errorf("Keys() = %v, want sorted", got)
		}
	}
}
