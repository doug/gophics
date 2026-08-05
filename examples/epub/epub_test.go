package main

import (
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
