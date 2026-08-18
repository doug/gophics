package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCatalog(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "feeds.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAndSelect(t *testing.T) {
	p := writeCatalog(t, `{
	  "version": 1,
	  "feeds": [
	    {"id":"a","title":"A","url":"https://a.test/f","category":"ai","tags":["papers","ai4science"],"fulltext":"full"},
	    {"id":"b","title":"B","url":"https://b.test/f","category":"tech","tags":["blog"],"fulltext":"teaser"},
	    {"id":"c","title":"C","url":"https://c.test/f","category":"ai","enabled":false}
	  ]
	}`)

	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Feeds) != 3 {
		t.Fatalf("loaded %d feeds", len(c.Feeds))
	}

	// Disabled feeds are excluded unless asked for.
	if got := len(c.Select(Selector{})); got != 2 {
		t.Errorf("default select got %d, want 2", got)
	}
	if got := len(c.Select(Selector{All: true})); got != 3 {
		t.Errorf("select all got %d, want 3", got)
	}
	if got := c.Select(Selector{Categories: []string{"AI"}}); len(got) != 1 || got[0].ID != "a" {
		t.Errorf("category match should be case-insensitive, got %v", got)
	}
	if got := c.Select(Selector{Tags: []string{"AI4SCIENCE"}}); len(got) != 1 {
		t.Errorf("tag match got %d, want 1", len(got))
	}
	if got := c.Select(Selector{IDs: []string{"b"}}); len(got) != 1 || got[0].ID != "b" {
		t.Errorf("id select got %v", got)
	}

	if cats := c.Categories(); strings.Join(cats, ",") != "ai,tech" {
		t.Errorf("Categories = %v, want sorted [ai tech]", cats)
	}
	if _, ok := c.ByID("A"); !ok {
		t.Error("ByID should be case-insensitive")
	}
	if _, ok := c.ByID("nope"); ok {
		t.Error("ByID should report missing feeds")
	}
}

func TestShouldExtractDefaults(t *testing.T) {
	tru, fls := true, false
	cases := []struct {
		name string
		feed Feed
		want bool
	}{
		{"full text feeds are not extracted", Feed{Fulltext: FullText}, false},
		{"teaser feeds are extracted", Feed{Fulltext: Teaser}, true},
		{"partial feeds are extracted", Feed{Fulltext: Partial}, true},
		{"unknown feeds are extracted", Feed{Fulltext: Unknown}, true},
		{"missing classification is extracted", Feed{}, true},
		{"explicit true overrides full", Feed{Fulltext: FullText, Extract: &tru}, true},
		{"explicit false overrides teaser", Feed{Fulltext: Teaser, Extract: &fls}, false},
	}
	for _, c := range cases {
		if got := c.feed.ShouldExtract(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]string{
		"missing id":       `{"feeds":[{"url":"https://x.test","category":"ai"}]}`,
		"missing url":      `{"feeds":[{"id":"a","category":"ai"}]}`,
		"missing category": `{"feeds":[{"id":"a","url":"https://x.test"}]}`,
		"duplicate id": `{"feeds":[
			{"id":"a","url":"https://x.test","category":"ai"},
			{"id":"a","url":"https://y.test","category":"tech"}]}`,
	}
	for name, body := range cases {
		if _, err := Load(writeCatalog(t, body)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// An unknown field is a typo in a hand-edited catalog, and silently ignoring it
// would mean a setting that appears to work but does nothing.
func TestUnknownFieldIsRejected(t *testing.T) {
	body := `{"feeds":[{"id":"a","url":"https://x.test","category":"ai","fulltxet":"full"}]}`
	if _, err := Load(writeCatalog(t, body)); err == nil {
		t.Error("expected an error for a misspelled field")
	}
}

func TestMinInterval(t *testing.T) {
	if got := (Feed{}).MinInterval(); got != 0 {
		t.Errorf("default should be zero, got %v", got)
	}
	if got := (Feed{MinIntervalSeconds: 3}).MinInterval().Seconds(); got != 3 {
		t.Errorf("got %v seconds, want 3", got)
	}
}

func TestSplitList(t *testing.T) {
	cases := map[string][]string{
		"":            nil,
		"   ":         nil,
		"a":           {"a"},
		"a,b":         {"a", "b"},
		" a , b ,, c": {"a", "b", "c"},
	}
	for in, want := range cases {
		got := SplitList(in)
		if len(got) != len(want) {
			t.Errorf("SplitList(%q) = %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("SplitList(%q) = %v, want %v", in, got, want)
				break
			}
		}
	}
}

// The shipped catalog must always load, since every command depends on it.
func TestShippedCatalogIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "feeds.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("feeds.json not present")
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("the shipped catalog does not load: %v", err)
	}
	if len(c.Feeds) == 0 {
		t.Fatal("the shipped catalog has no feeds")
	}

	for _, f := range c.Feeds {
		if !strings.HasPrefix(f.URL, "http://") && !strings.HasPrefix(f.URL, "https://") {
			t.Errorf("feed %s: URL is not http(s): %q", f.ID, f.URL)
		}
		if f.Title == "" {
			t.Errorf("feed %s: missing title", f.ID)
		}
		if strings.ContainsAny(f.ID, " \t/") {
			// Ids become filenames in the store, so they must stay path-safe.
			t.Errorf("feed %s: id must not contain spaces or slashes", f.ID)
		}
	}
	if len(c.Categories()) < 5 {
		t.Errorf("expected several categories, got %v", c.Categories())
	}
}
