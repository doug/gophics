package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/examples/news/internal/store"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
)

// harness builds the real app over a library seeded into a scratch directory,
// so these are end-to-end tests of the widget tree rather than of a mock.
func harness(t *testing.T, items ...*store.Item) (*app.Headless, *library.Library) {
	t.Helper()
	library.SetDataDir(t.TempDir())

	lib := library.Open()
	if lib.OpenError() != "" {
		t.Fatal(lib.OpenError())
	}
	// Nothing in a test may touch the network: the queue polls on open by
	// default, and the seeded catalog is 25 real sources.
	lib.Prefs.SetRefreshOnLaunch(false)
	lib.Prefs.SetPrefetch(false)

	for _, f := range lib.Subs.All() {
		lib.Subs.Remove(f.ID)
	}
	if err := lib.Subs.Add(catalog.Feed{
		ID: "testfeed", Title: "Test Feed", URL: "https://test.example/feed.xml",
		Category: "tech", Priority: catalog.Normal, Kind: catalog.Compounding,
	}); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if err := lib.Store.Put(it); err != nil {
			t.Fatal(err)
		}
	}

	h, err := app.NewHeadless(App{Env: &Env{Lib: lib}}, app.Config{
		Size:         geom.Size{W: 420, H: 820},
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: Fonts(),
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	settle(h)
	return h, lib
}

func settle(h *app.Headless) {
	for i := 0; i < 300 && h.Step(0.016); i++ {
		h.Render()
	}
	h.Render()
}

// waitFor renders until the condition holds, for the asynchronous loads the app
// does off the frame goroutine.
func waitFor(t *testing.T, h *app.Headless, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
		settle(h)
	}
	if !cond() {
		t.Fatalf("timed out waiting for %s; on screen: %v", what, labels(h))
	}
}

func labels(h *app.Headless) []string {
	var out []string
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

// rectOf returns the laid-out rectangle of the first node whose label contains
// substr. Presence in the semantics tree is not evidence that anything is
// visible: a widget with no width still reports its label.
func rectOf(h *app.Headless, substr string) (geom.Rect, bool) {
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if strings.Contains(n.Label, substr) {
			return n.Rect, true
		}
	}
	return geom.Rect{}, false
}

func onScreen(h *app.Headless, substr string) bool {
	for _, l := range labels(h) {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func article(id, title string, published time.Time, body string, words int) *store.Item {
	return &store.Item{
		ID: id, FeedID: "testfeed", FeedName: "Test Feed", Category: "tech",
		Title: title, Link: "https://test.example/" + id, GUID: id,
		Published: published, Fetched: time.Now(),
		Summary: "A short teaser for " + title, ContentHTML: body,
		WordCount: words, Source: store.SourceExtracted,
	}
}

const bodyHTML = `<p>The first paragraph explains the <b>whole idea</b> and links to
<a href="https://go.dev/blog">the Go blog</a> for the details.</p>
<h2>A section heading</h2>
<p>A second paragraph with <i>emphasis</i> and some <code>inline_code</code>.</p>
<blockquote><p>A quotation that should be set apart from the prose around it.</p></blockquote>
<ul><li>First point</li><li>Second point</li></ul>
<pre><code>func main() { println("hi") }</code></pre>`

// otherBodyHTML is deliberately distinct, so a test that means to open the top
// row cannot pass by opening the second one.
// longParagraph is filler with enough words to make an article that must be
// scrolled.
const longParagraph = `A B-tree keeps its keys in sorted order and its leaves at a uniform depth, which is what makes a lookup cost a logarithm rather than a scan. The interesting part is not the shape but the fan-out: a node sized to a disk page holds hundreds of keys, so even a very large table is only three or four reads deep.`

const otherBodyHTML = `<p>An entirely different opening sentence about cache eviction.</p>`

// rowY is a point inside the first queue row. The header is short and the
// category filter is hidden while there is only one category, so the first row
// starts just below the title bar.
const rowY = 60

func TestQueueShowsRankedArticlesAndOpensOne(t *testing.T) {
	now := time.Now()
	h, lib := harness(t,
		article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900),
		article("b", "An older piece about caching", now.Add(-96*time.Hour), otherBodyHTML, 700),
	)

	waitFor(t, h, "the queue", func() bool { return onScreen(h, "How indexes actually work") })
	if !onScreen(h, "An older piece about caching") {
		t.Error("second article missing from the queue")
	}
	// Both are unread, and the fresher one should lead with no learning yet.
	q := lib.Queue(library.QueueOptions{})
	if len(q) != 2 || q[0].Item.ID != "a" {
		t.Fatalf("queue order wrong: %+v", q)
	}

	// Open the top article: its body must render, not just its headline.
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article body", func() bool { return onScreen(h, "The first paragraph explains") })

	for _, want := range []string{"A section heading", "A quotation that should be set apart", "First point"} {
		if !onScreen(h, want) {
			t.Errorf("article is missing %q", want)
		}
	}
	// It must be the top-ranked article that opened, not whichever row the tap
	// happened to land on.
	if onScreen(h, "An entirely different opening sentence") {
		t.Error("tapping the first row opened the second article")
	}
}

func TestOpeningAnArticleMarksItReadAndTeachesTheModel(t *testing.T) {
	now := time.Now()
	h, lib := harness(t, article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900))
	waitFor(t, h, "the queue", func() bool { return onScreen(h, "How indexes actually work") })

	before := lib.Rank.Trained()
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article body", func() bool { return onScreen(h, "The first paragraph explains") })

	if lib.Rank.Trained() <= before {
		t.Error("opening an article taught the ranking model nothing")
	}
	if q := lib.Queue(library.QueueOptions{}); len(q) != 0 {
		t.Errorf("an opened article is still in the unread queue: %d", len(q))
	}
}

func TestArticleLinksOpen(t *testing.T) {
	now := time.Now()
	h, _ := harness(t, article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900))
	waitFor(t, h, "the queue", func() bool { return onScreen(h, "How indexes") })
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article body", func() bool { return onScreen(h, "The first paragraph explains") })

	// Sweep the first paragraph for the link.
	found := false
scan:
	for y := float32(100); y < 700; y += 6 {
		for x := float32(24); x < 400; x += 18 {
			h.Tap(geom.Pt{X: x, Y: y})
			for _, u := range h.OpenedURLs {
				if u == "https://go.dev/blog" {
					found = true
					break scan
				}
			}
		}
	}
	if !found {
		t.Fatalf("the article's link never opened; opened=%v", h.OpenedURLs)
	}
}

func TestEmptyQueueSaysSo(t *testing.T) {
	h, _ := harness(t)
	waitFor(t, h, "the empty state", func() bool { return onScreen(h, "caught up") })
}

func TestSourcesTabListsSubscriptionsAndSuggestions(t *testing.T) {
	h, _ := harness(t)
	waitFor(t, h, "the queue", func() bool { return onScreen(h, "caught up") })

	// The tab bar sits at the bottom; Sources is the middle of three.
	h.Tap(geom.Pt{X: 210, Y: 795})
	settle(h)
	waitFor(t, h, "the sources tab", func() bool { return onScreen(h, "Test Feed") })

	if !onScreen(h, "Browse suggestions") {
		t.Error("no way in to the built-in catalog")
	}
}

// The renderer is where an article stops being markup and becomes something
// readable, so it is worth checking directly as well as on screen.
func TestParseArticleStructure(t *testing.T) {
	blocks := parseArticle(bodyHTML, paletteOf(theme.Light()))

	var kinds []blockKind
	for _, b := range blocks {
		kinds = append(kinds, b.kind)
	}
	want := []blockKind{blockPara, blockHeading, blockPara, blockQuote,
		blockListItem, blockListItem, blockCode}
	if len(kinds) != len(want) {
		t.Fatalf("got %d blocks %v, want %d %v", len(kinds), kinds, len(want), want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Errorf("block %d is kind %d, want %d", i, kinds[i], want[i])
		}
	}

	// Inline styling must survive: the link, the bold run, and the code face.
	var link, bold, mono bool
	for _, b := range blocks {
		for _, sp := range b.spans {
			if sp.Link == "https://go.dev/blog" {
				link = true
			}
			if sp.Font == "bold" && strings.Contains(sp.Text, "whole idea") {
				bold = true
			}
			if sp.Font == "mono" {
				mono = true
			}
		}
	}
	if !link || !bold || !mono {
		t.Errorf("inline styles lost: link=%v bold=%v mono=%v", link, bold, mono)
	}

	// The code block keeps its text verbatim.
	if got := blocks[6].text; !strings.Contains(got, `println("hi")`) {
		t.Errorf("code block = %q", got)
	}
	// List items are numbered/bulleted and indented.
	if blocks[4].label != "•" || blocks[4].depth != 1 {
		t.Errorf("list item = %+v", blocks[4])
	}
}

func TestParseArticleHandlesImagesAndOrdinals(t *testing.T) {
	blocks := parseArticle(
		`<figure><img src="https://cdn.example/a.jpg" alt="A long descriptive caption here"/>
		 <figcaption>The caption</figcaption></figure>
		 <ol><li>one</li><li>two</li></ol>`,
		paletteOf(theme.Light()))

	var img, caption bool
	var ordinals []string
	for _, b := range blocks {
		switch b.kind {
		case blockImage:
			if b.url == "https://cdn.example/a.jpg" {
				img = true
			}
		case blockCaption:
			caption = true
		case blockListItem:
			ordinals = append(ordinals, b.label)
		}
	}
	if !img {
		t.Error("image block missing")
	}
	if !caption {
		t.Error("caption missing")
	}
	if strings.Join(ordinals, ",") != "1.,2." {
		t.Errorf("ordered list labels = %v", ordinals)
	}
}

func TestParseArticleOnJunk(t *testing.T) {
	if got := parseArticle("", paletteOf(theme.Light())); got != nil {
		t.Errorf("empty markup should produce no blocks, got %d", len(got))
	}
	// Unclosed tags and stray scripts must not lose the prose.
	blocks := parseArticle(`<p>kept<script>alert(1)</script><div><p>also kept`,
		paletteOf(theme.Light()))
	var text string
	for _, b := range blocks {
		for _, sp := range b.spans {
			text += sp.Text
		}
	}
	if !strings.Contains(text, "kept") || !strings.Contains(text, "also kept") {
		t.Errorf("prose lost: %q", text)
	}
	if strings.Contains(text, "alert") {
		t.Errorf("script content rendered: %q", text)
	}
}

// Markup puts the space between a styled run and the text after it at the start
// of the following text node. Dropping it silently welds the words together —
// "the whole ideaand links to" — which is invisible in the block structure and
// glaring on screen.
func TestParseArticleKeepsSpacesAroundStyledRuns(t *testing.T) {
	blocks := parseArticle(bodyHTML, paletteOf(theme.Light()))
	var text string
	for _, b := range blocks {
		for _, sp := range b.spans {
			text += sp.Text
		}
	}
	for _, want := range []string{
		"the whole idea and links to",
		"the Go blog for the details.",
		"with emphasis and some inline_code",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in rendered text; got %q", want, text)
		}
	}
	if strings.Contains(text, "  ") {
		t.Errorf("doubled spaces in rendered text: %q", text)
	}
}

// A list must actually occupy the screen.
//
// widget.Column defaults to CrossCenter, which gives each child its intrinsic
// cross-axis size — and a LazyList has none. Building the queue into a plain
// Column laid the whole list out at zero width: the header still said "173
// unread", every row still reported its label to the accessibility tree, and
// the screen was blank. Asserting on labels alone cannot see that, so this
// checks the geometry.
func TestQueueRowsHaveRealWidth(t *testing.T) {
	now := time.Now()
	h, _ := harness(t,
		article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900),
		article("b", "An older piece about caching", now.Add(-96*time.Hour), otherBodyHTML, 700),
	)
	waitFor(t, h, "the queue", func() bool { return onScreen(h, "How indexes actually work") })

	r, ok := rectOf(h, "How indexes actually work")
	if !ok {
		t.Fatal("headline not in the semantics tree")
	}
	if w := r.Max.X - r.Min.X; w < 200 {
		t.Errorf("queue row is %.0f wide in a 420-wide window — the list is not being stretched", w)
	}
	if hgt := r.Max.Y - r.Min.Y; hgt < 10 {
		t.Errorf("queue row is %.0f tall", hgt)
	}
}

// The same trap in the reader: an article laid out at zero width renders
// nothing while reporting every paragraph to the accessibility tree.
func TestArticleBodyHasRealWidth(t *testing.T) {
	now := time.Now()
	h, _ := harness(t, article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900))
	waitFor(t, h, "the queue", func() bool { return onScreen(h, "How indexes") })
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article body", func() bool { return onScreen(h, "The first paragraph explains") })

	r, ok := rectOf(h, "The first paragraph explains")
	if !ok {
		t.Fatal("body paragraph not in the semantics tree")
	}
	if w := r.Max.X - r.Min.X; w < 200 {
		t.Errorf("article paragraph is %.0f wide in a 420-wide window", w)
	}
}

// The category filter appears only once there are two or more categories, so a
// single-source test never renders it — and it turned out to swallow the entire
// list when it did appear.
func TestQueueWithCategoryFilterStillShowsArticles(t *testing.T) {
	now := time.Now()
	_, lib := harness(t, article("a", "How indexes actually work", now.Add(-2*time.Hour), bodyHTML, 900))

	// A second feed in a different category brings the filter row on screen.
	if err := lib.Subs.Add(catalog.Feed{
		ID: "sciencefeed", Title: "Science Feed", URL: "https://science.example/feed.xml",
		Category: "science", Priority: catalog.Normal,
	}); err != nil {
		t.Fatal(err)
	}
	sci := article("s", "Why rivers are mathematical", now.Add(-5*time.Hour), otherBodyHTML, 800)
	sci.FeedID, sci.FeedName, sci.Category = "sciencefeed", "Science Feed", "science"
	sci.ID = "s"
	if err := lib.Store.Put(sci); err != nil {
		t.Fatal(err)
	}

	// Rebuild so the new feed is picked up.
	h2, _ := rebuild(t, lib)
	waitFor(t, h2, "the filter row", func() bool { return onScreen(h2, "science") })
	waitFor(t, h2, "the queue", func() bool { return onScreen(h2, "How indexes actually work") })

	r, ok := rectOf(h2, "How indexes actually work")
	if !ok {
		t.Fatal("headline missing")
	}
	if hgt := r.Max.Y - r.Min.Y; hgt < 10 {
		t.Errorf("queue row is %.0f tall with the filter row present", hgt)
	}
	// It must sit below the filter, not off the bottom of the window.
	if r.Min.Y < 60 || r.Min.Y > 700 {
		t.Errorf("queue row is at y=%.0f, which is not on screen below the filter", r.Min.Y)
	}
}

// rebuild starts a fresh headless app over an existing library.
func rebuild(t *testing.T, lib *library.Library) (*app.Headless, *library.Library) {
	t.Helper()
	h, err := app.NewHeadless(App{Env: &Env{Lib: lib}}, app.Config{
		Size:         geom.Size{W: 420, H: 820},
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: Fonts(),
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	settle(h)
	return h, lib
}

// A figure carrying both an alt text and a figcaption must produce one caption.
// Publishers routinely put the same sentence in both, and the reader printed it
// twice under the photograph.
func TestFigureCaptionIsNotDuplicated(t *testing.T) {
	blocks := parseArticle(
		`<figure><img src="https://cdn.example/prison.jpg"
		   alt="The state prison in Concord, New Hampshire. (Zoey Knox photo 2024 / NHPR)"/>
		 <figcaption>The state prison in Concord, New Hampshire. (Zoey Knox / NHPR)</figcaption>
		 </figure>`, paletteOf(theme.Light()))

	var captions []string
	for _, b := range blocks {
		if b.kind == blockCaption {
			var s string
			for _, sp := range b.spans {
				s += sp.Text
			}
			captions = append(captions, s)
		}
	}
	if len(captions) != 1 {
		t.Fatalf("expected one caption, got %d: %q", len(captions), captions)
	}
	if !strings.Contains(captions[0], "Zoey Knox / NHPR") {
		t.Errorf("the figcaption should win over the alt text, got %q", captions[0])
	}

	// Without a figcaption, a descriptive alt text is still worth showing.
	solo := parseArticle(`<figure><img src="https://cdn.example/a.jpg" alt="A long descriptive caption"/></figure>`,
		paletteOf(theme.Light()))
	found := false
	for _, b := range solo {
		if b.kind == blockCaption {
			found = true
		}
	}
	if !found {
		t.Error("an alt text with no figcaption should still become a caption")
	}
}

// The feed's lead image and the body's first photograph are usually the same
// picture at different CDN crops, so their URLs differ and comparing them let
// the reader show it twice.
func TestLeadImageIsSuppressedWhenTheBodyOpensWithOne(t *testing.T) {
	withImage := parseArticle(`<figure><img src="https://cdn.example/big.jpg"/></figure><p>Text.</p>`,
		paletteOf(theme.Light()))
	if !bodyOpensWithImage(withImage) {
		t.Error("a body starting with a figure should suppress the lead image")
	}

	textFirst := parseArticle(`<p>One.</p><p>Two.</p><p>Three.</p><p>Four.</p>
		<figure><img src="https://cdn.example/late.jpg"/></figure>`, paletteOf(theme.Light()))
	if bodyOpensWithImage(textFirst) {
		t.Error("an image far down the article should not suppress the lead image")
	}
}

// A very long listing must be truncated, not merely given too little room:
// nothing clips, so an over-tall block paints straight over the prose below it.
func TestLongCodeBlocksAreTruncated(t *testing.T) {
	var lines []string
	for i := range 500 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	got, n := clampCode(strings.Join(lines, "\n"))
	if n > maxCodeLines+3 {
		t.Errorf("kept %d lines, want about %d", n, maxCodeLines)
	}
	if !strings.Contains(got, "more lines") {
		t.Errorf("truncation should say what was dropped: %q", got[max(0, len(got)-120):])
	}

	short := "one\ntwo\nthree"
	if got, n := clampCode(short); got != short || n != 3 {
		t.Errorf("a short listing must pass through unchanged: %q %d", got, n)
	}
}

// Leaving a finished article must record the strongest signal the model gets.
//
// This is worth an end-to-end test rather than a unit one because the failure
// mode was invisible: widget.Disposer is `interface{ Dispose() }`, so a method
// written as Dispose(ctx) compiles, satisfies nothing, and is never called. The
// app looked completely normal and simply never learned from a finished read.
func TestFinishingAnArticleRecordsMoreThanOpeningIt(t *testing.T) {
	now := time.Now()
	// Long enough that reaching the end means actually scrolling there, which
	// is the signal the reader listens for.
	long := strings.Repeat(`<p>`+longParagraph+`</p>`, 12)
	h, lib := harness(t, article("a", "A long piece", now.Add(-time.Hour), long, 2000))

	waitFor(t, h, "the queue", func() bool { return onScreen(h, "A long piece") })
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article", func() bool { return onScreen(h, "B-tree keeps its keys") })

	afterOpen := lib.Rank.Trained()
	if afterOpen == 0 {
		t.Fatal("opening the article recorded nothing at all")
	}

	// Read to the end, which is what distinguishes a finished article from an
	// opened one.
	h.Move(geom.Pt{X: 210, Y: 400})
	for range 15 {
		h.Scroll(geom.Pt{Y: -1200})
		h.Render()
		settle(h)
	}

	// Go back, which unmounts the reader.
	h.Tap(geom.Pt{X: 24, Y: 30})
	settle(h)
	waitFor(t, h, "the queue again", func() bool { return onScreen(h, "A long piece") })

	if got := lib.Rank.Trained(); got <= afterOpen {
		t.Errorf("leaving a finished article added nothing (%v then %v) — is Dispose being called?",
			afterOpen, got)
	}
}

// An article short enough to need no scrolling must still be able to count as
// read: the end-reached signal never fires for content that fits, and the dwell
// fallback asks for longer than reading it takes.
func TestAShortArticleThatFitsOnScreenCountsAsRead(t *testing.T) {
	old := shortReadDwell
	shortReadDwell = 0
	t.Cleanup(func() { shortReadDwell = old })

	now := time.Now()
	h, lib := harness(t, article("a", "A short note", now.Add(-time.Hour),
		`<p>One paragraph, quickly finished.</p>`, 40))

	waitFor(t, h, "the queue", func() bool { return onScreen(h, "A short note") })
	h.Tap(geom.Pt{X: 210, Y: rowY})
	settle(h)
	waitFor(t, h, "the article", func() bool { return onScreen(h, "One paragraph, quickly finished") })
	afterOpen := lib.Rank.Trained()

	h.Tap(geom.Pt{X: 24, Y: 30})
	settle(h)
	waitFor(t, h, "the queue again", func() bool { return onScreen(h, "A short note") })

	if got := lib.Rank.Trained(); got <= afterOpen {
		t.Errorf("a short article read in full recorded nothing extra (%v then %v)", afterOpen, got)
	}
}

// An <li> whose text is wrapped in a block — <li><p>text</p></li> — or which
// holds a nested list has already been flushed by the child's own handler, so
// the list case's own flush emits nothing and the item silently loses its
// bullet and indent. Both shapes are ordinary in extracted article markup.
func TestListItemsWithBlockChildrenKeepTheirBullets(t *testing.T) {
	t.Run("wrapped in a paragraph", func(t *testing.T) {
		blocks := parseArticle(`<ul><li><p>Wrapped</p></li><li>Plain</li></ul>`,
			paletteOf(theme.Light()))
		items := listItems(blocks)
		if len(items) != 2 {
			t.Fatalf("got %d list items, want 2: %+v", len(items), blocks)
		}
		for _, b := range items {
			if b.label != "•" || b.depth != 1 {
				t.Errorf("item %q has label %q depth %d, want \"•\" depth 1", spanText(b), b.label, b.depth)
			}
		}
	})

	t.Run("nested list", func(t *testing.T) {
		blocks := parseArticle(`<ul><li>Outer<ul><li>Inner</li></ul></li></ul>`,
			paletteOf(theme.Light()))
		items := listItems(blocks)
		if len(items) != 2 {
			t.Fatalf("got %d list items, want 2: %+v", len(items), blocks)
		}
		outer, inner := items[0], items[1]
		if spanText(outer) != "Outer" || outer.depth != 1 || outer.label != "•" {
			t.Errorf("outer item = %q depth %d label %q", spanText(outer), outer.depth, outer.label)
		}
		if spanText(inner) != "Inner" || inner.depth != 2 {
			t.Errorf("inner item = %q depth %d", spanText(inner), inner.depth)
		}
	})

	t.Run("several paragraphs in one item", func(t *testing.T) {
		blocks := parseArticle(`<ol><li><p>First para</p><p>Second para</p></li></ol>`,
			paletteOf(theme.Light()))
		items := listItems(blocks)
		if len(items) != 2 {
			t.Fatalf("got %d list items, want 2: %+v", len(items), blocks)
		}
		// One bullet for the item, and the continuation indented to match.
		if items[0].label != "1." {
			t.Errorf("first block should carry the number, got %q", items[0].label)
		}
		if items[1].label != "" {
			t.Errorf("continuation should not repeat the number, got %q", items[1].label)
		}
		if items[1].depth != items[0].depth {
			t.Errorf("continuation indent %d does not match the item's %d", items[1].depth, items[0].depth)
		}
	})
}

func listItems(blocks []block) []block {
	var out []block
	for _, b := range blocks {
		if b.kind == blockListItem {
			out = append(out, b)
		}
	}
	return out
}

func spanText(b block) string {
	var s string
	for _, sp := range b.spans {
		s += sp.Text
	}
	return strings.TrimSpace(s)
}
