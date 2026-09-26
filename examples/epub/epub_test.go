package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestParseEPUB exercises the whole pipeline: buildEPUB() zips a spec-shaped
// .epub, parseEPUB() reads it back through container.xml → OPF spine → XHTML.
func TestParseEPUB(t *testing.T) {
	b, err := parseEPUB(buildEPUB())
	if err != nil {
		t.Fatalf("parseEPUB: %v", err)
	}
	if b.Title != sampleMeta.title || b.Author != sampleMeta.author {
		t.Fatalf("metadata = %q by %q", b.Title, b.Author)
	}
	if len(b.Chapters) != len(sampleChapters) {
		t.Fatalf("chapters = %d, want %d", len(b.Chapters), len(sampleChapters))
	}

	c0 := b.Chapters[0]
	if !c0.Blocks[0].Heading || c0.Blocks[0].Text != "I. The Tide-Clock" {
		t.Fatalf("chapter 0 first block = %+v", c0.Blocks[0])
	}
	if c0.Title != "I. The Tide-Clock" {
		t.Fatalf("chapter 0 title = %q", c0.Title)
	}
	paras := 0
	for _, bl := range c0.Blocks {
		if !bl.Heading {
			paras++
		}
		// Whitespace must be collapsed — no source newlines/double spaces leak.
		if strings.ContainsAny(bl.Text, "\n\t") || strings.Contains(bl.Text, "  ") {
			t.Errorf("uncollapsed block text: %q", bl.Text)
		}
	}
	if paras != 3 {
		t.Errorf("chapter 0 paragraphs = %d, want 3", paras)
	}

	// Spine order is preserved and inline <em> text survives flattening.
	if b.Chapters[1].Title != "II. What the Almanac Said" {
		t.Errorf("chapter 1 title = %q", b.Chapters[1].Title)
	}
	var joined string
	for _, bl := range b.Chapters[1].Blocks {
		joined += bl.Text + " "
	}
	if !strings.Contains(joined, "do not correct it") {
		t.Error("inline <em> text was dropped")
	}
}

// A book from "Open EPUB…" is not the bundled one. Real chapters are HTML in
// XML clothing — &nbsp; and &mdash; on most pages, paragraphs wrapped in
// <div>s — and a spine can name an item the manifest does not have. None of
// that may lose text: the first entity used to end the chapter silently.
func TestParseEPUBFromTheWild(t *testing.T) {
	b, err := parseEPUB(zipOf(
		"mimetype", "application/epub+zip",
		"META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/book.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/book.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Wild</dc:title><dc:creator>Anon</dc:creator></metadata>
  <manifest>
    <item id="one" href="text/one.xhtml" media-type="application/xhtml+xml"/>
    <item id="two" href="text/two.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine><itemref idref="one"/><itemref idref="missing"/><itemref idref="two"/></spine>
</package>`,
		"OEBPS/text/one.xhtml", `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
<h2>One</h2>
<p>Ink&nbsp;and&mdash;paper</p>
<p>after the entity</p>
</body></html>`,
		"OEBPS/text/two.xhtml", `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml"><body>
<div class="section"><p>wrapped in a div</p></div>
</body></html>`,
	))
	if err != nil {
		t.Fatalf("parseEPUB: %v", err)
	}
	if len(b.Chapters) != 2 {
		t.Fatalf("chapters = %d, want 2 (the unresolvable itemref is skipped)", len(b.Chapters))
	}
	if got := blockTexts(b.Chapters[0]); strings.Join(got, "|") != "One|Ink and—paper|after the entity" {
		t.Errorf("chapter with entities = %q", got)
	}
	if got := blockTexts(b.Chapters[1]); strings.Join(got, "|") != "wrapped in a div" {
		t.Errorf("div-wrapped chapter = %q", got)
	}
	if b.Chapters[1].Title != "Chapter 2" {
		t.Errorf("headingless chapter title = %q", b.Chapters[1].Title)
	}
}

// A chapter the decoder cannot read is reported, not shown shorter. The UI
// keeps the bundled book on error, which is an honest outcome; a chapter that
// ends mid-page with no explanation is not.
func TestParseEPUBReportsAnUnreadableChapter(t *testing.T) {
	_, err := parseEPUB(zipOf(
		"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="book.opf"/></rootfiles></container>`,
		"book.opf", `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="c" href="c.xhtml"/></manifest><spine><itemref idref="c"/></spine></package>`,
		"c.xhtml", `<html><body><p>fine</p><p>broken</q></body></html>`,
	))
	if err == nil {
		t.Fatal("a chapter with a mismatched end tag parsed without error")
	}
	if !strings.Contains(err.Error(), "c.xhtml") {
		t.Errorf("error does not name the chapter: %v", err)
	}
}

func blockTexts(c Chapter) []string {
	out := make([]string, len(c.Blocks))
	for i, b := range c.Blocks {
		out[i] = b.Text
	}
	return out
}

// zipOf builds a zip from name/body pairs, in order.
func zipOf(pairs ...string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i+1 < len(pairs); i += 2 {
		w, _ := zw.Create(pairs[i])
		_, _ = io.WriteString(w, pairs[i+1])
	}
	_ = zw.Close()
	return buf.Bytes()
}

// TestLoadErrorNamesTheFileAndTheChapter pins the message the library shows
// under "Open EPUB…" when a book is refused: a reader whose 30th chapter has a
// stray end tag is told which file and where, not just that it failed.
func TestLoadErrorNamesTheFileAndTheChapter(t *testing.T) {
	err := errors.New("epub: ch30.xhtml: unexpected end element </q>")
	msg := loadError("tide.epub", err)
	for _, want := range []string{"tide.epub", "ch30.xhtml", err.Error()} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not mention %q", msg, want)
		}
	}
	if msg := loadError("", err); !strings.Contains(msg, "that file") {
		t.Errorf("with no name, message %q should still read naturally", msg)
	}
}
