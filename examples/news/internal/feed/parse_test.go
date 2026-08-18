package feed

import (
	"strings"
	"testing"
	"time"
)

func TestParseRSS2(t *testing.T) {
	const doc = `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"
     xmlns:dc="http://purl.org/dc/elements/1.1/">
 <channel>
  <title>Example Blog</title>
  <link>https://example.com/</link>
  <description>Things and stuff</description>
  <image><title>Ignore Me</title><url>https://example.com/logo.png</url></image>
  <item>
   <title>Hello &amp; goodbye</title>
   <link>https://example.com/post/1</link>
   <guid isPermaLink="false">tag:example.com,2026:1</guid>
   <pubDate>Tue, 12 Aug 2026 09:30:00 +0000</pubDate>
   <dc:creator>Ada Lovelace</dc:creator>
   <category>Go</category>
   <category>Testing</category>
   <description>A short teaser.</description>
   <content:encoded><![CDATA[<p>The <em>real</em> body.</p>]]></content:encoded>
   <enclosure url="https://example.com/a.mp3" type="audio/mpeg" length="1234"/>
  </item>
 </channel>
</rss>`

	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Format != "rss" {
		t.Errorf("Format = %q, want rss", f.Format)
	}
	if f.Title != "Example Blog" {
		t.Errorf("feed Title = %q, want Example Blog (an <image><title> must not win)", f.Title)
	}
	if f.Link != "https://example.com/" {
		t.Errorf("feed Link = %q", f.Link)
	}
	if len(f.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(f.Items))
	}
	it := f.Items[0]
	if it.Title != "Hello & goodbye" {
		t.Errorf("Title = %q", it.Title)
	}
	if it.Link != "https://example.com/post/1" {
		t.Errorf("Link = %q", it.Link)
	}
	if it.GUID != "tag:example.com,2026:1" {
		t.Errorf("GUID = %q", it.GUID)
	}
	if it.Author != "Ada Lovelace" {
		t.Errorf("Author = %q", it.Author)
	}
	want := time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC)
	if !it.Published.Equal(want) {
		t.Errorf("Published = %v, want %v", it.Published, want)
	}
	if it.Content != "<p>The <em>real</em> body.</p>" {
		t.Errorf("Content = %q, want the content:encoded body", it.Content)
	}
	if it.Summary != "A short teaser." {
		t.Errorf("Summary = %q", it.Summary)
	}
	if strings.Join(it.Categories, ",") != "Go,Testing" {
		t.Errorf("Categories = %v", it.Categories)
	}
	if len(it.Enclosures) != 1 || it.Enclosures[0].Length != 1234 {
		t.Errorf("Enclosures = %+v", it.Enclosures)
	}
}

// Nature and Science serve RSS 1.0, where <item> elements are siblings of
// <channel> rather than children of it.
func TestParseRDF(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns="http://purl.org/rss/1.0/"
         xmlns:dc="http://purl.org/dc/elements/1.1/">
 <channel rdf:about="https://www.nature.com/nature.rss">
  <title>Nature</title>
  <link>https://www.nature.com/nature/</link>
 </channel>
 <item rdf:about="https://www.nature.com/articles/d41586-026-1">
  <title>A paper about something</title>
  <link>https://www.nature.com/articles/d41586-026-1</link>
  <dc:date>2026-08-14T00:00:00Z</dc:date>
  <dc:creator>Jane Researcher</dc:creator>
  <description>Nature, Published online: 14 August 2026</description>
 </item>
</rdf:RDF>`

	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Format != "rdf" {
		t.Errorf("Format = %q, want rdf", f.Format)
	}
	if f.Title != "Nature" {
		t.Errorf("Title = %q", f.Title)
	}
	if len(f.Items) != 1 {
		t.Fatalf("got %d items, want 1 (RDF items are siblings of channel)", len(f.Items))
	}
	it := f.Items[0]
	if it.Author != "Jane Researcher" {
		t.Errorf("Author = %q", it.Author)
	}
	if got := it.Published.Format("2006-01-02"); got != "2026-08-14" {
		t.Errorf("Published = %v, want 2026-08-14 (from dc:date)", got)
	}
}

func TestParseAtom(t *testing.T) {
	const doc = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
 <title>research!rsc</title>
 <link rel="self" href="https://research.swtch.com/feed.atom"/>
 <link rel="alternate" href="https://research.swtch.com/"/>
 <updated>2026-08-10T12:00:00Z</updated>
 <entry>
  <title>On Generics</title>
  <link rel="alternate" type="text/html" href="https://research.swtch.com/generic"/>
  <link rel="replies" href="https://example.com/replies"/>
  <id>tag:research.swtch.com,2026:generic</id>
  <published>2026-08-10T12:00:00Z</published>
  <updated>2026-08-11T09:00:00Z</updated>
  <author><name>Russ Cox</name></author>
  <summary>Short version.</summary>
  <content type="xhtml"><div><p>Long <b>version</b> with a <br/> break.</p></div></content>
 </entry>
</feed>`

	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Format != "atom" {
		t.Errorf("Format = %q, want atom", f.Format)
	}
	if f.Link != "https://research.swtch.com/" {
		t.Errorf("feed Link = %q, want the alternate link not the self link", f.Link)
	}
	it := f.Items[0]
	if it.Link != "https://research.swtch.com/generic" {
		t.Errorf("Link = %q, want the alternate link", it.Link)
	}
	if it.Author != "Russ Cox" {
		t.Errorf("Author = %q", it.Author)
	}
	// type="xhtml" content is nested elements, not escaped text: it must be
	// re-serialised rather than dropped.
	if !strings.Contains(it.Content, "<b>version</b>") {
		t.Errorf("Content = %q, want re-serialised xhtml", it.Content)
	}
	if !strings.Contains(it.Content, "<br/>") {
		t.Errorf("Content = %q, want self-closed void element", it.Content)
	}
	if it.Summary != "Short version." {
		t.Errorf("Summary = %q", it.Summary)
	}
	if got := it.Updated.Format(time.RFC3339); got != "2026-08-11T09:00:00Z" {
		t.Errorf("Updated = %v", got)
	}
}

// arXiv, NHPR and hnrss all serve a valid channel with no entries. That is a
// legitimate state, not a parse failure.
func TestParseEmptyChannelIsNotAnError(t *testing.T) {
	const doc = `<?xml version='1.0' encoding='UTF-8'?>
<rss version="2.0"><channel>
 <title>cs.LG updates on arXiv.org</title>
 <link>http://rss.arxiv.org/rss/cs.LG</link>
 <lastBuildDate>Sat, 15 Aug 2026 04:00:12 +0000</lastBuildDate>
</channel></rss>`

	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Items) != 0 {
		t.Errorf("got %d items, want 0", len(f.Items))
	}
	if f.Title == "" {
		t.Error("feed metadata should still be populated")
	}
}

func TestParseUndeclaredEntityAndBadMarkup(t *testing.T) {
	// &nbsp; is not an XML entity, and the <b> tag is left unclosed.
	const doc = `<rss version="2.0"><channel><title>Sloppy</title>
 <item><title>Caf&eacute;&nbsp;news</title><link>https://x.test/1</link>
  <description>Some <b>bold text</description></item>
</channel></rss>`

	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("got %d items, want 1", len(f.Items))
	}
	if !strings.Contains(f.Items[0].Title, "Café") {
		t.Errorf("Title = %q, want a decoded &eacute;", f.Items[0].Title)
	}
}

func TestParseBOMAndLeadingJunk(t *testing.T) {
	doc := "\xEF\xBB\xBF\n  <rss version=\"2.0\"><channel><title>T</title>" +
		"<item><link>https://x.test/1</link></channel></rss>"
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Title != "T" {
		t.Errorf("Title = %q", f.Title)
	}
}

func TestParseLatin1(t *testing.T) {
	// "Café" encoded as Latin-1: 0xE9 for é.
	doc := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>" +
		"<rss version=\"2.0\"><channel><title>Caf\xE9</title>" +
		"<item><title>Caf\xE9</title><link>https://x.test/1</link></item>" +
		"</channel></rss>")
	f, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Title != "Café" {
		t.Errorf("Title = %q, want Café", f.Title)
	}
}

func TestParseNotAFeed(t *testing.T) {
	if _, err := Parse([]byte("<!doctype html><html><body>nope</body></html>")); err == nil {
		t.Error("want an error for an HTML document")
	}
	if _, err := Parse(nil); err == nil {
		t.Error("want an error for empty input")
	}
}

func TestSyntheticGUIDIsStable(t *testing.T) {
	const doc = `<rss version="2.0"><channel><title>T</title>
 <item><title>No identity here</title></item></channel></rss>`
	first, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	a, b := first.Items[0].GUID, second.Items[0].GUID
	if a == "" || a != b {
		t.Errorf("synthetic GUIDs must be stable: %q vs %q", a, b)
	}
}

func TestItemTextLen(t *testing.T) {
	it := Item{Content: "<p>Hello <b>world</b></p>"}
	if got := it.TextLen(); got < 10 || got > 14 {
		t.Errorf("TextLen = %d, want roughly len(\"Hello world\")", got)
	}
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Tue, 12 Aug 2026 09:30:00 +0000", "2026-08-12T09:30:00Z"},
		{"Tue, 12 Aug 2026 09:30:00 GMT", "2026-08-12T09:30:00Z"},
		{"2026-08-12T09:30:00Z", "2026-08-12T09:30:00Z"},
		{"2026-08-12T09:30:00+02:00", "2026-08-12T07:30:00Z"},
		{"2026-08-12", "2026-08-12T00:00:00Z"},
		{"Tue, 12 Aug 2026 09:30:00 -0700 (PDT)", "2026-08-12T16:30:00Z"},
		{"12 Aug 2026 09:30:00 -0000", "2026-08-12T09:30:00Z"},
		{"August 12, 2026", "2026-08-12T00:00:00Z"},
	}
	for _, c := range cases {
		got, ok := ParseDate(c.in)
		if !ok {
			t.Errorf("ParseDate(%q) failed", c.in)
			continue
		}
		if g := got.Format(time.RFC3339); g != c.want {
			t.Errorf("ParseDate(%q) = %v, want %v", c.in, g, c.want)
		}
	}
	if _, ok := ParseDate("not a date at all"); ok {
		t.Error("want failure for garbage input")
	}
	if _, ok := ParseDate(""); ok {
		t.Error("want failure for empty input")
	}
}
