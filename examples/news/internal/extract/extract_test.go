package extract

import (
	"errors"
	"strings"
	"testing"
)

// realisticPage mimics the shape of a news article: site chrome, a sidebar of
// links, the article itself, a share bar, related links and comments.
const realisticPage = `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>Researchers Fold A Protein | The Daily Science</title>
<meta property="og:title" content="Researchers Fold A Protein">
<meta property="og:site_name" content="The Daily Science">
<meta name="author" content="Marie Curie">
<meta property="og:description" content="A short standfirst about the protein.">
<meta property="og:image" content="/img/lead.jpg">
<link rel="canonical" href="https://daily.science/2026/protein">
<script>var tracking = 1;</script>
<style>.x{color:red}</style>
</head><body>
<header class="site-header"><a href="/">The Daily Science</a><nav><a href="/a">A</a><a href="/b">B</a></nav></header>
<div class="wrapper">
 <aside class="sidebar">
  <ul><li><a href="/1">Story one</a></li><li><a href="/2">Story two</a></li><li><a href="/3">Story three</a></li></ul>
 </aside>
 <article class="post-content">
  <h1>Researchers Fold A Protein</h1>
  <p class="byline">By Marie Curie</p>
  <div class="share-buttons"><a href="/tw">Tweet</a><a href="/fb">Share</a></div>
  <p>The first paragraph explains, at some length and with several commas, what the researchers actually did in the laboratory over the course of the year.</p>
  <p>The second paragraph continues the account, adding detail about the method, the instruments involved, and the difficulties encountered along the way.</p>
  <figure><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" data-src="/img/fig1.png" alt="Figure 1"><figcaption>Figure 1: the fold.</figcaption></figure>
  <p>A third paragraph draws the conclusion, noting that the result, while preliminary, points toward a broader class of structures worth investigating.</p>
  <blockquote>It was, in the end, a matter of patience.</blockquote>
  <img src="/track/1x1.gif" width="1" height="1">
  <div class="related-posts"><a href="/r1">Related one</a><a href="/r2">Related two</a><a href="/r3">Related three</a></div>
 </article>
 <section class="comments"><h2>Comments</h2><p>First comment, which is reasonably long and contains commas, but is not part of the article at all.</p></section>
</div>
<footer class="site-footer"><p>Copyright 2026 The Daily Science, all rights reserved.</p></footer>
</body></html>`

func TestFromHTMLRealisticPage(t *testing.T) {
	art, err := FromHTML([]byte(realisticPage), "https://daily.science/2026/protein", DefaultOptions())
	if err != nil {
		t.Fatalf("FromHTML: %v", err)
	}

	if art.Title != "Researchers Fold A Protein" {
		t.Errorf("Title = %q", art.Title)
	}
	if art.Byline != "Marie Curie" {
		t.Errorf("Byline = %q", art.Byline)
	}
	if art.SiteName != "The Daily Science" {
		t.Errorf("SiteName = %q", art.SiteName)
	}
	if art.Canonical != "https://daily.science/2026/protein" {
		t.Errorf("Canonical = %q", art.Canonical)
	}
	if art.LeadImage != "https://daily.science/img/lead.jpg" {
		t.Errorf("LeadImage = %q, want an absolute URL", art.LeadImage)
	}

	// The article body must be present.
	for _, want := range []string{"first paragraph explains", "second paragraph continues", "third paragraph draws"} {
		if !strings.Contains(art.Text, want) {
			t.Errorf("body is missing %q\n---\n%s", want, art.Text)
		}
	}
	// The furniture must not be.
	for _, unwanted := range []string{"Story one", "Tweet", "Related one", "First comment", "Copyright 2026", "var tracking"} {
		if strings.Contains(art.Text, unwanted) {
			t.Errorf("body should not contain %q\n---\n%s", unwanted, art.Text)
		}
	}

	// Lazy-loaded image promoted from data-src and made absolute.
	if !strings.Contains(art.HTML, `src="https://daily.science/img/fig1.png"`) {
		t.Errorf("expected the data-src image to be resolved:\n%s", art.HTML)
	}
	// Tracking pixel dropped.
	if strings.Contains(art.HTML, "1x1.gif") {
		t.Errorf("tracking pixel survived:\n%s", art.HTML)
	}
	// Class and style attributes stripped.
	if strings.Contains(art.HTML, "class=") {
		t.Errorf("class attributes should be stripped:\n%s", art.HTML)
	}
	if art.WordCount < 50 {
		t.Errorf("WordCount = %d, want a realistic count", art.WordCount)
	}
}

func TestFromHTMLKeepImagesOff(t *testing.T) {
	opts := DefaultOptions()
	opts.KeepImages = false
	art, err := FromHTML([]byte(realisticPage), "https://daily.science/2026/protein", opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(art.HTML, "<img") {
		t.Errorf("images should be removed:\n%s", art.HTML)
	}
	if !strings.Contains(art.Text, "Figure 1: the fold") {
		t.Log("note: figcaption dropped along with the figure")
	}
}

func TestFromHTMLTooShort(t *testing.T) {
	page := `<html><body><div><p>Too little.</p></div></body></html>`
	_, err := FromHTML([]byte(page), "https://x.test/", DefaultOptions())
	if !errors.Is(err, ErrTooShort) {
		t.Errorf("err = %v, want ErrTooShort", err)
	}
}

// Some pages defeat the scorer entirely; the paragraph fallback should still
// return the prose.
func TestFromHTMLFallbackToParagraphs(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	for range 6 {
		sb.WriteString("<p>A paragraph of prose that is long enough to count, with commas, clauses, and a reasonable amount of substance to it.</p>")
	}
	sb.WriteString("</body></html>")

	art, err := FromHTML([]byte(sb.String()), "https://x.test/", DefaultOptions())
	if err != nil {
		t.Fatalf("FromHTML: %v", err)
	}
	if !strings.Contains(art.Text, "A paragraph of prose") {
		t.Errorf("Text = %q", art.Text)
	}
}

func TestCleanTitle(t *testing.T) {
	cases := []struct{ in, site, want string }{
		{"Real Headline Here | The Publication", "The Publication", "Real Headline Here"},
		{"Real Headline Here - The Publication", "", "Real Headline Here"},
		{"Short | X", "", "Short | X"}, // head too short to trim confidently
		{"A Headline With No Separator", "", "A Headline With No Separator"},
		{"Headline — Some Site", "Some Site", "Headline"},
	}
	for _, c := range cases {
		if got := cleanTitle(c.in, c.site); got != c.want {
			t.Errorf("cleanTitle(%q, %q) = %q, want %q", c.in, c.site, got, c.want)
		}
	}
}

func TestLargestFromSrcset(t *testing.T) {
	got := largestFromSrcset("small.jpg 320w, medium.jpg 640w, large.jpg 1280w")
	if got != "large.jpg" {
		t.Errorf("got %q, want large.jpg", got)
	}
	if got := largestFromSrcset("only.jpg"); got != "only.jpg" {
		t.Errorf("got %q", got)
	}
	if got := largestFromSrcset(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeFeedHTML(t *testing.T) {
	in := `<div class="post"><p>Hello <b>world</b>.</p>` +
		`<script>evil()</script>` +
		`<p><a href="/rel" onclick="x()">link</a></p>` +
		`<iframe src="https://youtube.com/x"></iframe>` +
		`<img src="/img.png" alt="pic"></div>`

	got := Sanitize(in, "https://blog.test/post/1", DefaultOptions())

	if strings.Contains(got, "script") || strings.Contains(got, "evil") {
		t.Errorf("script survived: %s", got)
	}
	if strings.Contains(got, "onclick") {
		t.Errorf("event handler survived: %s", got)
	}
	if strings.Contains(got, "iframe") {
		t.Errorf("iframe survived: %s", got)
	}
	if !strings.Contains(got, `href="https://blog.test/rel"`) {
		t.Errorf("relative href not absolutised: %s", got)
	}
	if !strings.Contains(got, `src="https://blog.test/img.png"`) {
		t.Errorf("relative img src not absolutised: %s", got)
	}
	if !strings.Contains(got, "<b>world</b>") {
		t.Errorf("inline markup lost: %s", got)
	}
}

func TestSanitizeProducesSelfClosedVoidElements(t *testing.T) {
	got := Sanitize(`<p>a<br>b<hr></p>`, "", DefaultOptions())
	if !strings.Contains(got, "<br/>") {
		t.Errorf("want a self-closed br for XHTML validity, got %s", got)
	}
}

func TestSanitizeEmpty(t *testing.T) {
	if got := Sanitize("", "", DefaultOptions()); got != "" {
		t.Errorf("got %q", got)
	}
	if got := Sanitize("   ", "", DefaultOptions()); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestPlainText(t *testing.T) {
	got := PlainText("<p>Hello <b>world</b>.</p><p>Second.</p>")
	if !strings.Contains(got, "Hello world") || !strings.Contains(got, "Second") {
		t.Errorf("PlainText = %q", got)
	}
	if strings.Contains(got, "<") {
		t.Errorf("tags survived: %q", got)
	}
}

func TestFromHTMLLatin1(t *testing.T) {
	// A page declaring Latin-1 with a 0xE9 byte for é.
	page := "<html><head><meta charset=\"iso-8859-1\"><title>Caf\xE9</title></head><body>" +
		"<div><p>Le caf\xE9 est une boisson, pr\xE9par\xE9e \xE0 partir des graines torr\xE9fi\xE9es, " +
		"qui contient de la caf\xE9ine, et qui se boit g\xE9n\xE9ralement chaude dans de nombreux pays.</p>" +
		"<p>Une deuxi\xE8me phrase, avec des virgules, pour donner assez de texte au score de lisibilit\xE9.</p></div>" +
		"</body></html>"

	art, err := FromHTML([]byte(page), "https://fr.test/", DefaultOptions())
	if err != nil {
		t.Fatalf("FromHTML: %v", err)
	}
	if !strings.Contains(art.Text, "café") {
		t.Errorf("Latin-1 not decoded: %q", art.Text)
	}
}

func TestFromHTMLNoBody(t *testing.T) {
	if _, err := FromHTML([]byte("not html at all"), "", DefaultOptions()); err == nil {
		t.Error("want an error for input with no article")
	}
}

func TestDropsDuplicateHeadlineAndUILabels(t *testing.T) {
	page := `<!doctype html><html><head>
<title>Why Aging May Be a Program, Not a Breakdown</title>
<meta property="og:title" content="Why Aging May Be a Program, Not a Breakdown">
</head><body><article>
<p><a href="/tag/qa/">Q&amp;A</a></p>
<h1>Why Aging May Be a Program, Not a Breakdown</h1>
<p>Read Later</p>
<p>By deciphering the molecular signatures of millions of mouse cells, a researcher has found that aging is not haphazard wear and tear, but rather a remodeling of the cell society over time.</p>
<p>The second paragraph continues at length, with commas and clauses, describing the experiments and what they showed about how cells change.</p>
<h2>A real section heading</h2>
<p>A third paragraph, also substantial, discussing the implications of the result for the biology of aging.</p>
<p>Advertisement</p>
</article></body></html>`

	art, err := FromHTML([]byte(page), "https://q.test/x", DefaultOptions())
	if err != nil {
		t.Fatalf("FromHTML: %v", err)
	}
	// The headline must not be repeated inside the body.
	if strings.Contains(art.HTML, "<h1>") {
		t.Errorf("duplicated headline survived:\n%s", art.HTML)
	}
	for _, junk := range []string{"Read Later", "Advertisement"} {
		if strings.Contains(art.Text, junk) {
			t.Errorf("UI label %q survived:\n%s", junk, art.Text)
		}
	}
	// A genuine section heading must be kept.
	if !strings.Contains(art.HTML, "A real section heading") {
		t.Errorf("a real section heading was removed:\n%s", art.HTML)
	}
	// The prose must be intact.
	if !strings.Contains(art.Text, "molecular signatures") {
		t.Errorf("body prose lost:\n%s", art.Text)
	}
}

func TestKeepsLongTextContainingLabelWords(t *testing.T) {
	// "Advertisement" inside a real sentence must not trigger removal.
	page := `<html><body><div>
<p>The word Advertisement appears in this sentence, which is a genuine piece of prose about advertising history and should therefore be preserved in full.</p>
<p>A second paragraph, with commas and clauses, ensures there is enough text here for the scorer to accept this container as the article body.</p>
</div></body></html>`
	art, err := FromHTML([]byte(page), "https://x.test/", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(art.Text, "genuine piece of prose") {
		t.Errorf("prose containing a label word was removed:\n%s", art.Text)
	}
}

func TestNormalizeLabel(t *testing.T) {
	cases := map[string]string{
		"  Read   Later  ": "read later",
		"Share·":           "share",
		"Advertisement.":   "advertisement",
		"":                 "",
	}
	for in, want := range cases {
		if got := normalizeLabel(in); got != want {
			t.Errorf("normalizeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
