// Package extract pulls the readable article body out of a web page.
//
// The algorithm is the well-understood Readability heuristic: score block
// elements by how much punctuated prose they contain, discount them by how much
// of that text is inside links, and keep the winning subtree plus its
// substantial siblings. It is implemented here rather than taken from a library
// so the scoring stays adjustable and the dependency surface stays at one
// package.
package extract

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"

	"github.com/doug/gophics/examples/news/internal/textenc"
)

// Article is an extracted document.
type Article struct {
	Title     string
	Byline    string
	SiteName  string
	Excerpt   string
	HTML      string // cleaned body markup, safe to embed in XHTML
	Text      string // plain text rendering
	WordCount int
	Canonical string
	LeadImage string
}

// Options tunes extraction.
type Options struct {
	// MinTextLength is the shortest body, in characters, worth accepting.
	// Below it extraction reports ErrTooShort so callers can fall back to the
	// feed's own summary.
	MinTextLength int

	// KeepImages retains <img> elements. Turn it off for e-readers where
	// images cost download time and add little.
	KeepImages bool
}

// DefaultOptions is a reasonable starting point.
func DefaultOptions() Options {
	return Options{MinTextLength: 250, KeepImages: true}
}

// ErrTooShort means no candidate held enough prose to be an article.
var ErrTooShort = errors.New("extract: no substantial article body found")

// Patterns borrowed from Mozilla's Readability, which encode a decade of
// observation about how publishers name their wrappers.
var (
	// "paywall" is deliberately absent: publishers use it to mark the content
	// that the paywall protects, which is the article itself. The New Yorker
	// puts class="paywall" on every body paragraph.
	reUnlikely = regexp.MustCompile(`(?i)-ad-|ai2html|banner|breadcrumb|combx|comment|community|cover-wrap|disqus|extra|footer|gdpr|header|legends|menu|related|remark|replies|rss|shoutbox|sidebar|skyscraper|social|sponsor|supplemental|ad-break|agegate|pagination|pager|popup|yom-remote|newsletter|subscribe|recirc|promo|share|author-bio|more-from|nav`)
	reMaybe    = regexp.MustCompile(`(?i)and|article|body|column|content|main|shadow`)
	rePositive = regexp.MustCompile(`(?i)article|body|content|entry|hentry|h-entry|main|page|pagination|post|text|blog|story`)
	reNegative = regexp.MustCompile(`(?i)-ad-|hidden|^hid$|banner|combx|comment|com-|contact|footer|footnote|gdpr|masthead|media|meta|outbrain|promo|related|scroll|share|shoutbox|sidebar|skyscraper|sponsor|shopping|tags|widget|social`)
	reByline   = regexp.MustCompile(`(?i)byline|author|dateline|writtenby|p-author`)
)

// tagsToDrop never contain article prose.
var tagsToDrop = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true,
	"form": true, "button": true, "input": true, "select": true,
	"textarea": true, "svg": true, "canvas": true, "object": true,
	"embed": true, "applet": true, "map": true, "dialog": true,
}

// FromHTML extracts the article from a page. pageURL is used to resolve
// relative links and may be empty.
func FromHTML(raw []byte, pageURL string, opts Options) (*Article, error) {
	if opts.MinTextLength <= 0 {
		opts.MinTextLength = DefaultOptions().MinTextLength
	}

	raw = textenc.ToUTF8(raw, declaredCharset(raw))
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("extract: parse: %w", err)
	}

	base, _ := url.Parse(pageURL)
	art := &Article{}
	readMeta(doc, art)
	if art.Canonical != "" && base != nil {
		if u, err := base.Parse(art.Canonical); err == nil {
			art.Canonical = u.String()
			base = u
		}
	}

	body := first(doc, "body")
	if body == nil {
		body = doc
	}

	stripNoise(body)
	if art.Byline == "" {
		art.Byline = findByline(body)
	}

	candidate := selectContent(body, opts)
	if candidate == nil {
		return nil, ErrTooShort
	}

	cleanContent(candidate, base, opts)
	dropBoilerplate(candidate, art.Title)

	art.Text = textContent(candidate)
	art.WordCount = len(strings.Fields(art.Text))
	if len(art.Text) < opts.MinTextLength {
		return nil, ErrTooShort
	}

	art.HTML = renderChildren(candidate)
	if art.Excerpt == "" {
		art.Excerpt = firstSentences(art.Text, 300)
	}
	if art.LeadImage != "" && base != nil {
		if u, err := base.Parse(art.LeadImage); err == nil {
			art.LeadImage = u.String()
		}
	}
	return art, nil
}

// declaredCharset reads a charset from a meta tag without a full parse, since
// the parse itself needs to know the encoding.
func declaredCharset(raw []byte) string {
	head := raw
	if len(head) > 4096 {
		head = head[:4096]
	}
	lower := strings.ToLower(string(head))
	if _, after, ok := strings.Cut(lower, "charset="); ok {
		rest := after
		rest = strings.TrimLeft(rest, `"' `)
		end := strings.IndexAny(rest, `"' ;/>`+"\r\n\t")
		if end < 0 {
			end = len(rest)
		}
		return rest[:end]
	}
	return ""
}

// readMeta harvests title, byline, site name and canonical URL from <head>.
func readMeta(doc *html.Node, art *Article) {
	metaProps := map[string]string{}
	walk(doc, func(n *html.Node) bool {
		switch tag(n) {
		case "meta":
			key := strings.ToLower(attr(n, "property"))
			if key == "" {
				key = strings.ToLower(attr(n, "name"))
			}
			if v := attr(n, "content"); key != "" && v != "" {
				if _, seen := metaProps[key]; !seen {
					metaProps[key] = strings.TrimSpace(v)
				}
			}
		case "link":
			if strings.Contains(strings.ToLower(attr(n, "rel")), "canonical") && art.Canonical == "" {
				art.Canonical = attr(n, "href")
			}
		case "title":
			if art.Title == "" {
				art.Title = textContent(n)
			}
		}
		return true
	})

	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := metaProps[k]; v != "" {
				return v
			}
		}
		return ""
	}

	if v := pick("og:title", "twitter:title", "dc.title", "citation_title"); v != "" {
		art.Title = v
	}
	art.Byline = pick("author", "article:author", "dc.creator", "citation_author", "twitter:creator")
	art.SiteName = pick("og:site_name", "application-name", "twitter:site")
	art.Excerpt = pick("og:description", "description", "twitter:description")
	art.LeadImage = pick("og:image", "twitter:image", "twitter:image:src")

	art.Title = cleanTitle(art.Title, art.SiteName)
}

// cleanTitle removes the trailing site name that most publishers append to
// <title>, e.g. "Real Headline | The Publication".
func cleanTitle(title, site string) string {
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return ""
	}
	for _, sep := range []string{" | ", " - ", " — ", " – ", " :: ", " » ", " · "} {
		i := strings.LastIndex(title, sep)
		if i <= 0 {
			continue
		}
		head, tail := title[:i], title[i+len(sep):]
		// Only trim when the tail looks like a publication name rather than
		// part of the headline: short, and ideally matching the site name.
		if site != "" && strings.EqualFold(strings.TrimSpace(tail), strings.TrimSpace(site)) {
			return strings.TrimSpace(head)
		}
		if len(strings.Fields(tail)) <= 4 && len(strings.Fields(head)) >= 3 {
			return strings.TrimSpace(head)
		}
	}
	return title
}

// onDrop, when non-nil, is called for every node stripNoise discards. It exists
// for the scorer diagnostics in debug_test.go and is nil in normal builds.
var onDrop func(n *html.Node, reason string)

func noteDrop(n *html.Node, reason string) {
	if onDrop != nil {
		onDrop(n, reason)
	}
}

// bulkShare is the fraction of the page's text above which a node is treated as
// structural and exempted from name-based stripping.
const bulkShare = 0.4

// ancestorDepth is how many levels of ancestor receive credit from a scoring
// paragraph. It must be deep enough to reach the common ancestor of an article
// split across sibling wrappers.
const ancestorDepth = 5

// stripNoise removes elements that never carry prose, plus wrappers whose
// class or id marks them as furniture.
//
// Name-based stripping is guarded by how much of the document a node holds.
// Class names describe what a node *contains* as often as what it *is* — the
// BBC wraps articles in "ContainerWithSidebarWrapper" — so deleting a subtree
// because its name matched a pattern can take the whole article with it. A node
// holding a large share of the page's text is structure, whatever it is called.
func stripNoise(root *html.Node) {
	total := textLen(root)
	isBulk := func(n *html.Node) bool {
		return total > 0 && float64(textLen(n))/float64(total) > bulkShare
	}

	// Guard against death by a thousand cuts: many individually small removals
	// can still delete the article, as happens when every paragraph carries a
	// suspicious class. Once name-based stripping has claimed this much of the
	// page, stop trusting the names.
	var stripped int
	overBudget := func() bool {
		return total > 0 && float64(stripped)/float64(total) > 0.6
	}

	var doomed []*html.Node
	walk(root, func(n *html.Node) bool {
		name := tag(n)
		if name == "" {
			return true
		}
		if tagsToDrop[name] {
			noteDrop(n, "tag")
			doomed = append(doomed, n)
			return false
		}
		// Hidden content is not article content.
		if attr(n, "hidden") != "" || strings.Contains(attr(n, "style"), "display:none") ||
			strings.Contains(attr(n, "style"), "display: none") || attr(n, "aria-hidden") == "true" {
			// aria-hidden is common on decorative icons inside real prose, so
			// only drop it when the node holds no meaningful text.
			if attr(n, "aria-hidden") != "true" || textLen(n) < 25 {
				noteDrop(n, "hidden")
				doomed = append(doomed, n)
				return false
			}
		}
		switch name {
		case "nav", "aside", "footer":
			if !isBulk(n) {
				noteDrop(n, "tag:"+name)
				doomed = append(doomed, n)
				return false
			}
		case "header":
			// A <header> inside an <article> may hold the headline; one at page
			// level is site furniture. Drop only text-light ones.
			if textLen(n) < 200 {
				noteDrop(n, "header")
				doomed = append(doomed, n)
				return false
			}
		}
		if id := classID(n); id != "" && reUnlikely.MatchString(id) && !reMaybe.MatchString(id) {
			// A suspicious name only condemns a container. Prose elements are
			// judged on their text, never on their class, because publishers
			// label paragraphs by what happens to them rather than what they are.
			if name != "body" && name != "article" && name != "main" &&
				!isProseLeaf(name) && !isBulk(n) && !overBudget() {
				noteDrop(n, "unlikely:"+reUnlikely.FindString(id))
				stripped += textLen(n)
				doomed = append(doomed, n)
				return false
			}
		}
		return true
	})
	for _, n := range doomed {
		remove(n)
	}
}

// findByline locates an author line in the body when metadata lacks one.
func findByline(root *html.Node) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if tag(n) == "" {
			return true
		}
		if id := classID(n); id != "" && reByline.MatchString(id) {
			if t := textContent(n); len(t) > 3 && len(t) < 120 {
				out = strings.TrimPrefix(strings.TrimSpace(t), "By ")
				return false
			}
		}
		return true
	})
	return out
}

// scored tracks a candidate container and its accumulated score.
type scored struct {
	node  *html.Node
	score float64
}

// selectContent runs the Readability scoring pass and returns the node whose
// subtree is the article. The returned node may be synthetic.
func selectContent(body *html.Node, opts Options) *html.Node {
	scores := map[*html.Node]float64{}
	var order []*html.Node

	addScore := func(n *html.Node, v float64) {
		if n == nil || tag(n) == "" {
			return
		}
		if _, seen := scores[n]; !seen {
			scores[n] = classWeight(n) + tagBonus(n)
			order = append(order, n)
		}
		scores[n] += v
	}

	for _, p := range collect(body, "p", "pre", "td", "blockquote", "article", "section", "div") {
		// Only leaf-ish prose blocks seed the scoring; a div counts only when
		// it holds text directly rather than wrapping other blocks.
		if tag(p) == "div" || tag(p) == "section" || tag(p) == "article" {
			if hasBlockChildren(p) {
				continue
			}
		}
		text := textContent(p)
		if len(text) < 25 {
			continue
		}

		score := 1.0
		score += float64(strings.Count(text, ","))
		score += float64(strings.Count(text, "，")) // full-width comma
		if bonus := len(text) / 100; bonus > 0 {
			if bonus > 3 {
				bonus = 3
			}
			score += float64(bonus)
		}

		// Credit the ancestors, with the contribution decaying by distance, so
		// the container holding the most prose wins. Publishers commonly split
		// an article across several sibling wrappers, and only a propagation
		// deep enough to reach their common ancestor can reunite them.
		ancestor := p.Parent
		for level := 0; ancestor != nil && level < ancestorDepth; level++ {
			if tag(ancestor) == "" {
				break
			}
			divisor := 1.0
			switch level {
			case 0:
				divisor = 1
			case 1:
				divisor = 2
			default:
				divisor = float64(level) * 3
			}
			addScore(ancestor, score/divisor)
			ancestor = ancestor.Parent
		}
	}

	// Rank every candidate. Link-heavy containers are navigation, however much
	// text they hold, so the score is discounted by link density.
	ranked := make([]scored, 0, len(order))
	for _, n := range order {
		ranked = append(ranked, scored{node: n, score: scores[n] * (1 - linkDensity(n))})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	var best *scored
	if len(ranked) > 0 {
		best = &ranked[0]
	}

	if best == nil {
		return fallbackContent(body, opts)
	}

	top := best.node
	top = reuniteChunks(top, best.score, ranked)
	top = climbToBetterAncestor(top, scores)

	// Prefer a semantic <article> ancestor when it does not dilute the score,
	// since it usually includes the headline and dateline too.
	for a := top.Parent; a != nil; a = a.Parent {
		if tag(a) == "article" && textLen(a) < int(float64(textLen(top))*1.6) {
			top = a
			break
		}
	}

	result := gatherSiblings(top, best.score, scores)
	if textLen(result) < opts.MinTextLength {
		if fb := fallbackContent(body, opts); fb != nil && textLen(fb) > textLen(result) {
			return fb
		}
	}
	return result
}

// reuniteChunks handles articles broken into several sibling containers. When
// multiple candidates score close to the winner, their common ancestor is the
// real article — provided it does not drag in much beyond them.
func reuniteChunks(top *html.Node, topScore float64, ranked []scored) *html.Node {
	if topScore <= 0 {
		return top
	}
	peers := []*html.Node{top}
	var peerText int
	for _, r := range ranked {
		if r.node == top || r.score < topScore*0.5 {
			continue
		}
		// Ignore candidates already inside or containing another peer; they
		// describe the same text and would not move the common ancestor.
		if contains(top, r.node) || contains(r.node, top) {
			continue
		}
		peers = append(peers, r.node)
		peerText += textLen(r.node)
	}
	if len(peers) < 2 {
		return top
	}

	anc := commonAncestor(peers)
	if anc == nil || tag(anc) == "body" || tag(anc) == "html" {
		return top
	}
	peerText += textLen(top)
	// Reject an ancestor that is mostly other material, or mostly links.
	if textLen(anc) > peerText*2 || linkDensity(anc) > 0.5 {
		return top
	}
	return anc
}

// climbToBetterAncestor walks up while ancestors score better, which recovers
// the full article when the winning node is one section of it. It mirrors
// Mozilla Readability's parent walk, including the score floor that stops the
// climb before it reaches the page shell.
func climbToBetterAncestor(top *html.Node, scores map[*html.Node]float64) *html.Node {
	last, ok := scores[top]
	if !ok || last <= 0 {
		return top
	}
	floor := last / 3
	best := top
	for p := top.Parent; p != nil; p = p.Parent {
		name := tag(p)
		if name == "" || name == "body" || name == "html" {
			break
		}
		ps, ok := scores[p]
		if !ok {
			continue
		}
		if ps < floor {
			break
		}
		if ps > last && linkDensity(p) < 0.5 {
			best, last = p, ps
		}
	}
	return best
}

// contains reports whether a is an ancestor of b (or the same node).
func contains(a, b *html.Node) bool {
	for n := b; n != nil; n = n.Parent {
		if n == a {
			return true
		}
	}
	return false
}

// commonAncestor returns the lowest node containing all of nodes.
func commonAncestor(nodes []*html.Node) *html.Node {
	if len(nodes) == 0 {
		return nil
	}
	depth := func(n *html.Node) int {
		d := 0
		for p := n.Parent; p != nil; p = p.Parent {
			d++
		}
		return d
	}
	cur := nodes[0]
	for _, n := range nodes[1:] {
		a, b := cur, n
		for depth(a) > depth(b) {
			a = a.Parent
		}
		for depth(b) > depth(a) {
			b = b.Parent
		}
		for a != b && a != nil && b != nil {
			a, b = a.Parent, b.Parent
		}
		if a == nil {
			return nil
		}
		cur = a
	}
	return cur
}

// hasBlockChildren reports whether n contains block-level element children,
// which would make it a wrapper rather than a prose block.
func hasBlockChildren(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if name := tag(c); name != "" && !isPhrasing(name) {
			return true
		}
	}
	return false
}

// tagBonus nudges semantic containers ahead of anonymous divs.
func tagBonus(n *html.Node) float64 {
	switch tag(n) {
	case "article", "main":
		return 12
	case "section":
		return 4
	case "div":
		return 3
	case "blockquote":
		return 2
	case "pre", "td":
		return 1
	case "address", "ol", "ul", "dl", "dd", "dt", "li", "form":
		return -3
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		return -5
	}
	return 0
}

// classWeight rewards or penalises a node by how its class and id read.
func classWeight(n *html.Node) float64 {
	var w float64
	for _, key := range []string{"class", "id"} {
		v := attr(n, key)
		if v == "" {
			continue
		}
		if reNegative.MatchString(v) {
			w -= 25
		}
		if rePositive.MatchString(v) {
			w += 25
		}
	}
	if role := attr(n, "role"); role == "main" || role == "article" {
		w += 25
	}
	if attr(n, "itemprop") == "articleBody" {
		w += 40
	}
	return w
}

// gatherSiblings assembles the article from the top candidate plus adjacent
// blocks that also look like prose, which recovers lead paragraphs and pull
// quotes that live outside the winning container.
func gatherSiblings(top *html.Node, topScore float64, scores map[*html.Node]float64) *html.Node {
	parent := top.Parent
	if parent == nil {
		return top
	}

	threshold := topScore * 0.2
	if threshold < 10 {
		threshold = 10
	}

	out := &html.Node{Type: html.ElementNode, Data: "div"}
	setAttr(out, "class", "article-body")

	for _, sib := range children(parent) {
		keep := sib == top
		if !keep && tag(sib) != "" {
			if s, ok := scores[sib]; ok && s >= threshold {
				keep = true
			} else if tag(sib) == "p" {
				t := textContent(sib)
				switch {
				case len(t) > 80 && linkDensity(sib) < 0.25:
					keep = true
				case len(t) > 0 && len(t) <= 80 && linkDensity(sib) == 0 &&
					strings.HasSuffix(strings.TrimSpace(t), "."):
					keep = true
				}
			} else if tag(sib) == "figure" || tag(sib) == "h1" || tag(sib) == "h2" {
				keep = textLen(sib) > 0
			}
		}
		if !keep {
			continue
		}
		remove(sib)
		out.AppendChild(sib)
	}

	if out.FirstChild == nil {
		return top
	}
	return out
}

// fallbackContent is the last resort: every paragraph in the document, which
// beats returning nothing on pages whose markup defeats the scorer.
func fallbackContent(body *html.Node, opts Options) *html.Node {
	out := &html.Node{Type: html.ElementNode, Data: "div"}
	setAttr(out, "class", "article-body")
	var total int
	for _, p := range collect(body, "p", "h2", "h3", "pre", "blockquote", "ul", "ol") {
		if p.Parent == nil {
			continue
		}
		if tag(p) == "p" && (textLen(p) < 40 || linkDensity(p) > 0.5) {
			continue
		}
		total += textLen(p)
	}
	if total < opts.MinTextLength {
		return nil
	}
	for _, p := range collect(body, "p", "h2", "h3", "pre", "blockquote", "ul", "ol") {
		if p.Parent == nil {
			continue
		}
		if tag(p) == "p" && (textLen(p) < 40 || linkDensity(p) > 0.5) {
			continue
		}
		remove(p)
		out.AppendChild(p)
	}
	if out.FirstChild == nil {
		return nil
	}
	return out
}

// firstSentences trims text to a sentence boundary near max characters.
func firstSentences(text string, max int) string {
	if len(text) <= max {
		return text
	}
	cut := text[:max]
	if i := strings.LastIndexAny(cut, ".!?"); i > max/2 {
		return cut[:i+1]
	}
	if i := strings.LastIndex(cut, " "); i > 0 {
		return cut[:i] + "…"
	}
	return cut
}
