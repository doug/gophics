package ui

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// This file turns an article's markup into something worth reading on a phone.
//
// The body arriving from the store is already sanitised — the extractor
// stripped navigation, adverts, share buttons and scripts, and resolved every
// URL — so the job here is purely typographic: decide what each element is,
// and give it the spacing, size and weight that make a column of prose read
// well. Anything not recognised degrades to a paragraph rather than vanishing.
//
// The result is a flat list of blocks rather than a nested widget tree, because
// a long essay is scrolled through a LazyList: only the blocks near the viewport
// are ever built, so a 20,000-word piece opens as fast as a short one.

type blockKind int

const (
	blockPara blockKind = iota
	blockHeading
	blockImage
	blockQuote
	blockListItem
	blockCode
	blockRule
	blockCaption
)

// block is one laid-out piece of an article.
type block struct {
	kind  blockKind
	spans []layout.RichSpan // text blocks
	text  string            // code blocks, kept verbatim
	url   string            // images
	level int               // heading depth, 1..6
	label string            // list bullet or number
	depth int               // list nesting
}

// styleRun is the inline styling in force while walking the tree.
type styleRun struct {
	bold   bool
	italic bool
	code   bool
	link   string
}

// Palette is the set of colors the renderer paints with. It is taken from the
// theme so the article follows the platform's light and dark modes.
type palette struct {
	text  paint.Color
	muted paint.Color
	link  paint.Color
	code  paint.Color
}

func paletteOf(th theme.Theme) palette {
	return palette{text: th.Text, muted: th.Muted, link: th.Primary, code: th.Text}
}

// parseArticle converts sanitised article markup into blocks.
func parseArticle(fragment string, p palette) []block {
	if strings.TrimSpace(fragment) == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return []block{{kind: blockPara, spans: []layout.RichSpan{{Text: fragment, Color: p.text}}}}
	}
	r := &renderer{p: p}
	r.walk(doc, styleRun{}, 0, "")
	r.flush()
	return r.out
}

type renderer struct {
	p       palette
	out     []block
	pending []layout.RichSpan // inline spans accumulating into the current paragraph
	kind    blockKind
	level   int
	depth   int
	label   string

	// noAltCaption suppresses captions synthesised from an img's alt text while
	// inside a <figure> that has a real <figcaption>. Publishers routinely put
	// the same sentence in both, and rendering it twice under one photograph
	// looks like a bug in the reader rather than in the feed.
	noAltCaption bool
}

// flush closes the paragraph being accumulated.
func (r *renderer) flush() {
	if len(r.pending) == 0 {
		r.reset()
		return
	}
	// Collapse whitespace-only paragraphs, which markup is full of.
	var all strings.Builder
	for _, s := range r.pending {
		all.WriteString(s.Text)
	}
	if strings.TrimSpace(all.String()) == "" {
		r.reset()
		return
	}
	// Trim the leading and trailing whitespace of the paragraph as a whole,
	// leaving the interior spacing between spans intact.
	r.pending[0].Text = strings.TrimLeft(r.pending[0].Text, " \t\n")
	last := len(r.pending) - 1
	r.pending[last].Text = strings.TrimRight(r.pending[last].Text, " \t\n")

	r.out = append(r.out, block{
		kind: r.kind, spans: r.pending, level: r.level, depth: r.depth, label: r.label,
	})
	r.reset()
}

func (r *renderer) reset() {
	r.pending, r.kind, r.level, r.label = nil, blockPara, 0, ""
}

func (r *renderer) walk(n *html.Node, st styleRun, listDepth int, listLabel string) {
	switch n.Type {
	case html.TextNode:
		r.addText(n.Data, st)
		return
	case html.CommentNode, html.DoctypeNode:
		return
	}

	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "noscript", "iframe", "svg", "form", "button", "head":
			return

		case "br":
			r.addText("\n", st)
			return

		case "hr":
			r.flush()
			r.out = append(r.out, block{kind: blockRule})
			return

		case "img":
			r.flush()
			if src := attr(n, "src"); src != "" {
				r.out = append(r.out, block{kind: blockImage, url: src})
				if alt := strings.TrimSpace(attr(n, "alt")); !r.noAltCaption && alt != "" && len(alt) > 12 {
					// A long alt text is a caption someone wrote; a short one is
					// usually a filename or "image" and helps nobody.
					r.out = append(r.out, block{kind: blockCaption,
						spans: []layout.RichSpan{{Text: alt, Color: r.p.muted}}})
				}
			}
			return

		case "figcaption":
			r.flush()
			r.walkChildren(n, st, listDepth, listLabel)
			r.kind = blockCaption
			r.flush()
			return

		case "pre":
			r.flush()
			r.out = append(r.out, block{kind: blockCode, text: strings.Trim(textOf(n), "\n")})
			return

		case "blockquote":
			r.flush()
			// Render the quote's own children, then mark everything they
			// produced as quoted, so a multi-paragraph quotation stays one.
			start := len(r.out)
			r.walkChildren(n, st, listDepth, listLabel)
			r.flush()
			for i := start; i < len(r.out); i++ {
				if r.out[i].kind == blockPara {
					r.out[i].kind = blockQuote
				}
			}
			return

		case "h1", "h2", "h3", "h4", "h5", "h6":
			r.flush()
			r.walkChildren(n, styleRun{bold: true, link: st.link}, listDepth, listLabel)
			r.kind = blockHeading
			r.level = int(n.Data[1] - '0')
			r.flush()
			return

		case "ul", "ol":
			r.flush()
			ordered := n.Data == "ol"
			i := 0
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type != html.ElementNode || c.Data != "li" {
					continue
				}
				i++
				label := "•"
				if ordered {
					label = itoa(i) + "."
				}
				start := len(r.out)
				r.walkChildren(c, st, listDepth+1, label)
				r.kind = blockListItem
				r.depth = listDepth + 1
				r.label = label
				r.flush()

				// An <li> whose content is wrapped — <li><p>text</p></li>, or one
				// holding a nested list — has already had its text flushed by the
				// child's own handler, so the flush above emits nothing and the
				// item would lose its bullet and its indent entirely. Both shapes
				// are ordinary in extracted article markup. Fold whatever the
				// children produced back into this item, the same way blockquote
				// re-marks what it contains.
				bulleted := false
				for j := start; j < len(r.out); j++ {
					b := &r.out[j]
					if b.kind == blockListItem && b.depth == listDepth+1 && b.label == label {
						bulleted = true // the flush above already made the item
						continue
					}
					if b.kind != blockPara {
						continue // a nested list's own items, already marked
					}
					b.kind = blockListItem
					b.depth = listDepth + 1
					if !bulleted {
						// Only the first block of an item is bulleted; the rest
						// are continuation lines, indented to match.
						b.label, bulleted = label, true
					}
				}
			}
			r.depth = listDepth
			return

		case "figure":
			r.flush()
			// A figure's own caption wins over the alt text of the image inside
			// it; see noAltCaption.
			prev := r.noAltCaption
			r.noAltCaption = hasChild(n, "figcaption")
			r.walkChildren(n, st, listDepth, listLabel)
			r.noAltCaption = prev
			r.flush()
			return

		case "p", "div", "section", "article", "header", "footer",
			"table", "tr", "dl", "dt", "dd", "main", "aside":
			r.flush()
			r.walkChildren(n, st, listDepth, listLabel)
			r.flush()
			return

		case "a":
			if href := attr(n, "href"); href != "" {
				st.link = href
			}
		case "b", "strong":
			st.bold = true
		case "i", "em", "cite":
			st.italic = true
		case "code", "kbd", "samp", "tt":
			st.code = true
		case "td", "th":
			// A table cell has no column layout here; separate cells with a gap
			// so a simple table at least reads as a row of values.
			r.walkChildren(n, st, listDepth, listLabel)
			r.addText("  ", st)
			return
		}
	}
	r.walkChildren(n, st, listDepth, listLabel)
}

func (r *renderer) walkChildren(n *html.Node, st styleRun, listDepth int, listLabel string) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		r.walk(c, st, listDepth, listLabel)
	}
}

// addText appends a styled run to the paragraph being built, collapsing the
// runs of whitespace that markup indentation leaves everywhere.
func (r *renderer) addText(s string, st styleRun) {
	s = collapseSpace(s)
	if s == "" {
		return
	}
	// Two adjacent text nodes must neither run together nor double their
	// spacing: markup puts the gap in "the <b>idea</b> and links to" at the
	// start of the text node following the bold run.
	if strings.HasPrefix(s, " ") && len(r.pending) > 0 {
		if prev := r.pending[len(r.pending)-1].Text; strings.HasSuffix(prev, " ") {
			s = s[1:]
		}
	}
	if s == "" || (s == " " && len(r.pending) == 0) {
		return
	}
	sp := layout.RichSpan{Text: s, Color: r.p.text}
	switch {
	case st.bold && st.italic:
		sp.Font = "bolditalic"
	case st.bold:
		sp.Font = "bold"
	case st.italic:
		sp.Font = "italic"
	case st.code:
		sp.Font = "mono"
		sp.Color = r.p.code
	}
	if st.link != "" {
		sp.Color = r.p.link
		sp.Link = st.link
		sp.Underline = true
	}
	r.pending = append(r.pending, sp)
}

// collapseSpace turns every run of whitespace into a single space, but keeps a
// deliberate newline from <br>.
func collapseSpace(s string) string {
	if s == "\n" {
		return "\n"
	}
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', ' ':
			space = true
		default:
			// Emit the space even at the start of the run: a text node beginning
			// with whitespace is how markup separates it from the styled span
			// before it, and dropping that runs the two words together.
			if space {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		}
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

// hasChild reports whether an element contains the named element anywhere
// beneath it.
func hasChild(n *html.Node, name string) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == name {
			return true
		}
		if hasChild(c, name) {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// buildBlock renders one block. size is the reader's body text size, already
// scaled by the reading-size preference; every other size is derived from it so
// the whole article scales as one.
func buildBlock(th theme.Theme, b block, size float32, onLink func(string)) widget.Widget {
	switch b.kind {
	case blockRule:
		return widget.Padding{Insets: padV(18),
			Child: widget.Decorated{Color: th.Border, Child: widget.Sized{H: 1}}}

	case blockImage:
		return widget.Padding{Insets: padV(12), Child: Img{URL: b.url, MaxH: 520}}

	case blockCaption:
		return widget.Padding{Insets: padV(2),
			Child: widget.Rich{Spans: recolor(b.spans, th.Muted), Size: size * 0.82, OnLink: onLink}}

	case blockHeading:
		// h1 is the article's own title, already shown in the header, so the
		// body's headings start one step down.
		scale := map[int]float32{1: 1.5, 2: 1.32, 3: 1.16, 4: 1.06, 5: 1, 6: 1}[b.level]
		if scale == 0 {
			scale = 1.16
		}
		return widget.Padding{Insets: padTB(20, 6),
			Child: widget.Rich{Spans: bolden(b.spans, th.Text), Size: size * scale, OnLink: onLink}}

	case blockQuote:
		bar := widget.Decorated{Color: th.Primary, Child: widget.Sized{W: 3}}
		row := widget.Row(
			bar,
			widget.Sized{W: 12},
			widget.Expand(widget.Rich{Spans: recolor(b.spans, th.Muted), Size: size, OnLink: onLink}),
		)
		row.CrossAlign = layout.CrossStretch
		return widget.Padding{Insets: padV(10), Child: row}

	case blockCode:
		// Code scrolls sideways rather than wrapping, since a wrapped program
		// is harder to read than a scrolled one. The height has to be stated
		// rather than measured: a horizontal Scroll in a Column gets an
		// unbounded cross axis, and its scrollbar overlay fills that (see
		// chipBarHeight), so the block would measure as infinitely tall and
		// take the rest of the article off screen with it.
		codeSize := size * 0.85
		text, lines := clampCode(b.text)
		return widget.Padding{Insets: padV(10),
			Child: widget.Decorated{Color: th.Surface, Radius: th.Radius,
				Child: widget.Padding{All: 12,
					Child: widget.Sized{H: float32(lines) * codeSize * codeLineHeight,
						Child: widget.Scroll{Axis: layout.Horizontal,
							Child: widget.Text{S: text, Font: "mono", Size: codeSize, Color: th.Text}}},
				},
			},
		}

	case blockListItem:
		indent := float32(4 + (b.depth-1)*16)
		row := widget.Row(
			widget.Sized{W: indent},
			widget.Sized{W: 22, Child: widget.Text{S: b.label, Size: size, Color: th.Muted}},
			widget.Expand(widget.Rich{Spans: b.spans, Size: size, OnLink: onLink}),
		)
		row.CrossAlign = layout.CrossStart
		return widget.Padding{Insets: padV(4), Child: row}

	default:
		return widget.Padding{Insets: padV(8),
			Child: widget.Rich{Spans: b.spans, Size: size, OnLink: onLink}}
	}
}

// codeLineHeight is the monospace line advance as a multiple of the type size,
// used to reserve a code block's height; maxCodeLines caps how much of one
// listing is shown.
//
// The cap truncates the text itself, not just the reserved height. Nothing here
// clips, so a block given too little room paints straight over the paragraphs
// below it — a height cap alone would trade one broken layout for another.
const (
	codeLineHeight = 1.35
	maxCodeLines   = 40
)

// clampCode shortens an over-long listing and says so, rather than letting a
// thousand-line file bury the essay it was quoted in.
func clampCode(text string) (string, int) {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxCodeLines {
		return text, len(lines)
	}
	kept := lines[:maxCodeLines]
	kept = append(kept, "", "… "+itoa(len(lines)-maxCodeLines)+" more lines — open the original to read it all")
	return strings.Join(kept, "\n"), len(kept)
}

// padV is vertical padding only: article blocks are inset horizontally once by
// the reader, not per block.
func padV(v float32) geom.Insets { return geom.Insets{Top: v, Bottom: v} }

// padTB is asymmetric vertical padding, for headings that need more space above
// them than below.
func padTB(t, b float32) geom.Insets { return geom.Insets{Top: t, Bottom: b} }

// recolor restyles spans that carry the default text color, leaving links and
// other deliberate colors alone.
func recolor(spans []layout.RichSpan, c paint.Color) []layout.RichSpan {
	out := make([]layout.RichSpan, len(spans))
	copy(out, spans)
	for i := range out {
		if out[i].Link == "" {
			out[i].Color = c
		}
	}
	return out
}

// bolden makes every span in a heading bold, since markup often marks only part
// of a heading and a half-bold heading reads as a mistake.
func bolden(spans []layout.RichSpan, c paint.Color) []layout.RichSpan {
	out := make([]layout.RichSpan, len(spans))
	copy(out, spans)
	for i := range out {
		if out[i].Font == "" || out[i].Font == "italic" {
			out[i].Font = "bold"
		}
		if out[i].Link == "" {
			out[i].Color = c
		}
	}
	return out
}
