package feed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
)

// Namespaces we care about beyond the default one.
const (
	nsContent = "http://purl.org/rss/1.0/modules/content/"
	nsDC      = "http://purl.org/dc/elements/1.1/"
	nsMedia   = "http://search.yahoo.com/mrss/"
	nsAtom    = "http://www.w3.org/2005/Atom"
)

// maxDepth bounds recursion so a pathological document cannot exhaust the stack.
const maxDepth = 64

// Parse parses a syndication document. It returns an error only when the input
// could not be recognised as a feed at all; a well-formed feed with no entries
// yields a Feed with an empty Items slice.
func Parse(data []byte) (*Feed, error) {
	data = trimPreamble(data)
	if len(data) == 0 {
		return nil, errors.New("feed: empty document")
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false // invent end tags, leave unknown entities alone
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = charsetReader

	f := &Feed{}
	// skipContainers hold feed-level metadata we must not mistake for the
	// feed's own title/link (an <image><title> would otherwise clobber it).
	skipContainers := map[string]bool{"image": true, "textinput": true, "textInput": true}
	skipDepth := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Salvage whatever we decoded before the breakage. Truncated feeds
			// are common enough that partial success beats total failure.
			if f.Format != "" {
				break
			}
			return nil, fmt.Errorf("feed: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local

			if f.Format == "" {
				switch strings.ToLower(name) {
				case "rss":
					f.Format = "rss"
				case "feed":
					f.Format = "atom"
				case "rdf":
					f.Format = "rdf"
				default:
					// Not a feed root. Keep scanning: some servers wrap feeds.
				}
			}

			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if skipContainers[name] {
				skipDepth = 1
				continue
			}

			if isItemElement(name) {
				n, err := decodeNode(dec, t, 0)
				if err != nil {
					// Give up on this entry but keep the ones already parsed.
					if errors.Is(err, io.EOF) {
						return finish(f)
					}
					continue
				}
				f.Items = append(f.Items, mapItem(n))
				continue
			}

			// Feed-level scalars. Only the first occurrence wins.
			switch {
			case name == "title" && f.Title == "":
				n, err := decodeNode(dec, t, 0)
				if err == nil {
					f.Title = cleanText(innerText(n))
				}
			case name == "subtitle" || name == "description" || name == "tagline":
				if f.Description == "" {
					n, err := decodeNode(dec, t, 0)
					if err == nil {
						f.Description = cleanText(innerText(n))
					}
				}
			case name == "link" && f.Link == "":
				n, err := decodeNode(dec, t, 0)
				if err == nil {
					if l := pickLink(n); l != "" {
						f.Link = l
					}
				}
			case name == "lastBuildDate" || name == "updated" || name == "pubDate":
				n, err := decodeNode(dec, t, 0)
				if err == nil && f.Updated.IsZero() {
					f.Updated, _ = ParseDate(innerText(n))
				}
			}

		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
			}
		}
	}

	return finish(f)
}

func finish(f *Feed) (*Feed, error) {
	if f.Format == "" {
		return nil, errors.New("feed: not a recognised RSS, RDF or Atom document")
	}
	for i := range f.Items {
		if f.Items[i].GUID == "" {
			f.Items[i].GUID = syntheticGUID(f.Items[i])
		}
	}
	return f, nil
}

func isItemElement(local string) bool {
	switch local {
	case "item", "entry":
		return true
	}
	return false
}

// node is a decoded element subtree.
type node struct {
	Name xml.Name
	Attr []xml.Attr
	Text string
	Kids []*node
}

func decodeNode(dec *xml.Decoder, start xml.StartElement, depth int) (*node, error) {
	n := &node{Name: start.Name, Attr: start.Attr}
	if depth >= maxDepth {
		return n, dec.Skip()
	}
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			n.Text = sb.String()
			return n, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			kid, err := decodeNode(dec, t, depth+1)
			if kid != nil {
				n.Kids = append(n.Kids, kid)
			}
			if err != nil {
				n.Text = sb.String()
				return n, err
			}
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			n.Text = sb.String()
			return n, nil
		}
	}
}

// child returns the first child with the given local name, ignoring namespace.
func (n *node) child(local string) *node {
	for _, k := range n.Kids {
		if k.Name.Local == local {
			return k
		}
	}
	return nil
}

// childNS returns the first child matching both namespace and local name.
func (n *node) childNS(space, local string) *node {
	for _, k := range n.Kids {
		if k.Name.Local == local && k.Name.Space == space {
			return k
		}
	}
	return nil
}

func (n *node) children(local string) []*node {
	var out []*node
	for _, k := range n.Kids {
		if k.Name.Local == local {
			out = append(out, k)
		}
	}
	return out
}

func (n *node) attr(name string) string {
	for _, a := range n.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// text returns the trimmed character data of a child element, or "".
func (n *node) text(local string) string {
	if c := n.child(local); c != nil {
		return cleanText(innerText(c))
	}
	return ""
}

// innerText concatenates character data across the subtree.
func innerText(n *node) string {
	if len(n.Kids) == 0 {
		return n.Text
	}
	var sb strings.Builder
	sb.WriteString(n.Text)
	for _, k := range n.Kids {
		sb.WriteString(innerText(k))
	}
	return sb.String()
}

// innerHTML returns the element's body as HTML. Feeds escape markup as
// character data (directly or in CDATA), in which case Text already holds the
// HTML; Atom's type="xhtml" instead nests real elements, which we re-serialise.
func innerHTML(n *node) string {
	if len(n.Kids) == 0 {
		return n.Text
	}
	var sb strings.Builder
	sb.WriteString(n.Text)
	for _, k := range n.Kids {
		serialize(&sb, k)
	}
	return sb.String()
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

func serialize(sb *strings.Builder, n *node) {
	name := n.Name.Local
	sb.WriteByte('<')
	sb.WriteString(name)
	for _, a := range n.Attr {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		sb.WriteByte(' ')
		sb.WriteString(a.Name.Local)
		sb.WriteString(`="`)
		sb.WriteString(escapeAttr(a.Value))
		sb.WriteByte('"')
	}
	if voidElements[name] && len(n.Kids) == 0 && strings.TrimSpace(n.Text) == "" {
		sb.WriteString("/>")
		return
	}
	sb.WriteByte('>')
	sb.WriteString(escapeText(n.Text))
	for _, k := range n.Kids {
		serialize(sb, k)
	}
	sb.WriteString("</")
	sb.WriteString(name)
	sb.WriteByte('>')
}

func mapItem(n *node) Item {
	it := Item{

		Title: cleanText(stripTagsIfMarkup(n.text("title")))}

	// Identity. Atom uses <id>, RSS uses <guid>.
	if g := n.child("guid"); g != nil {
		it.GUID = cleanText(innerText(g))
	}
	if it.GUID == "" {
		it.GUID = n.text("id")
	}

	it.Link = itemLink(n)
	if it.GUID == "" {
		it.GUID = it.Link
	}

	it.Author = itemAuthor(n)

	for _, key := range []string{"pubDate", "published", "issued"} {
		if v := n.text(key); v != "" {
			if d, ok := ParseDate(v); ok {
				it.Published = d
				break
			}
		}
	}
	if it.Published.IsZero() {
		// dc:date, or RDF's bare <date>.
		if c := n.childNS(nsDC, "date"); c != nil {
			it.Published, _ = ParseDate(cleanText(innerText(c)))
		} else if v := n.text("date"); v != "" {
			it.Published, _ = ParseDate(v)
		}
	}
	if v := n.text("updated"); v != "" {
		it.Updated, _ = ParseDate(v)
	}
	if it.Published.IsZero() {
		it.Published = it.Updated
	}

	it.Content, it.Summary = itemBody(n)

	for _, c := range n.children("category") {
		// Atom puts the label in @term; RSS uses character data.
		v := c.attr("term")
		if v == "" {
			v = cleanText(innerText(c))
		}
		if v != "" && !containsFold(it.Categories, v) {
			it.Categories = append(it.Categories, v)
		}
	}
	for _, c := range n.children("subject") { // dc:subject
		if v := cleanText(innerText(c)); v != "" && !containsFold(it.Categories, v) {
			it.Categories = append(it.Categories, v)
		}
	}

	for _, e := range n.children("enclosure") {
		length, _ := strconv.ParseInt(e.attr("length"), 10, 64)
		if u := e.attr("url"); u != "" {
			it.Enclosures = append(it.Enclosures, Enclosure{URL: u, Type: e.attr("type"), Length: length})
		}
	}
	for _, k := range n.Kids {
		if k.Name.Space == nsMedia && (k.Name.Local == "content" || k.Name.Local == "thumbnail") {
			if u := k.attr("url"); u != "" {
				it.Enclosures = append(it.Enclosures, Enclosure{URL: u, Type: k.attr("type")})
			}
		}
	}

	return it
}

// itemLink resolves the entry's canonical URL across the three dialects.
func itemLink(n *node) string {
	var fallback string
	for _, l := range n.children("link") {
		href := l.attr("href")
		if href == "" {
			// RSS/RDF style: <link>https://...</link>
			if v := cleanText(innerText(l)); v != "" && fallback == "" {
				fallback = v
			}
			continue
		}
		rel := l.attr("rel")
		if rel == "" || rel == "alternate" {
			return href
		}
		if fallback == "" && rel != "self" && rel != "hub" && rel != "replies" {
			fallback = href
		}
	}
	if fallback != "" {
		return fallback
	}
	// Some feeds only offer a permalink-flavoured guid.
	if g := n.child("guid"); g != nil {
		if !strings.EqualFold(g.attr("isPermaLink"), "false") {
			if v := cleanText(innerText(g)); strings.HasPrefix(v, "http") {
				return v
			}
		}
	}
	if v := n.text("id"); strings.HasPrefix(v, "http") {
		return v
	}
	return ""
}

// pickLink resolves a single <link> element, which may carry its target in an
// href attribute (Atom) or as character data (RSS and RDF).
func pickLink(n *node) string {
	if href := n.attr("href"); href != "" {
		switch n.attr("rel") {
		case "", "alternate":
			return href
		default:
			return ""
		}
	}
	return cleanText(innerText(n))
}

func itemAuthor(n *node) string {
	// Atom: <author><name>
	if a := n.child("author"); a != nil {
		if nm := a.child("name"); nm != nil {
			return cleanText(innerText(nm))
		}
		if v := cleanText(innerText(a)); v != "" {
			return v
		}
	}
	if c := n.childNS(nsDC, "creator"); c != nil {
		return cleanText(innerText(c))
	}
	if v := n.text("creator"); v != "" {
		return v
	}
	return ""
}

// itemBody picks the richest body HTML available and a shorter summary.
// Preference: content:encoded > atom content > description > summary.
func itemBody(n *node) (content, summary string) {
	var encoded, atomContent, description, sum string

	if c := n.childNS(nsContent, "encoded"); c != nil {
		encoded = innerHTML(c)
	}
	for _, c := range n.children("content") {
		if c.Name.Space == nsContent {
			continue // already handled as content:encoded
		}
		if c.attr("src") != "" && strings.TrimSpace(innerHTML(c)) == "" {
			continue // out-of-line content, nothing to read
		}
		atomContent = innerHTML(c)
		break
	}
	if c := n.child("description"); c != nil {
		description = innerHTML(c)
	}
	if c := n.child("summary"); c != nil {
		sum = innerHTML(c)
	}

	for _, cand := range []string{encoded, atomContent, description, sum} {
		if strings.TrimSpace(cand) != "" {
			content = cand
			break
		}
	}
	// Summary is whichever short field we did not promote to content.
	for _, cand := range []string{sum, description} {
		if strings.TrimSpace(cand) != "" && cand != content {
			summary = cand
			break
		}
	}
	if summary == "" && content != "" {
		summary = content
	}
	return content, summary
}

func syntheticGUID(it Item) string {
	h := sha256.New()
	h.Write([]byte(it.Title))
	h.Write([]byte{0})
	h.Write([]byte(it.Link))
	h.Write([]byte{0})
	h.Write([]byte(it.Date().Format("2006-01-02T15:04:05Z07:00")))
	return "synthetic:" + hex.EncodeToString(h.Sum(nil)[:12])
}

func containsFold(xs []string, v string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// trimPreamble drops a UTF-8 BOM and any junk preceding the first XML tag.
func trimPreamble(b []byte) []byte {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if i := bytes.IndexByte(b, '<'); i > 0 {
		b = b[i:]
	}
	return bytes.TrimSpace(b)
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	// Collapse runs of whitespace; feed titles are frequently pretty-printed
	// across multiple lines.
	if strings.ContainsAny(s, "\n\r\t") {
		s = strings.Join(strings.Fields(s), " ")
	}
	return s
}

// stripTagsIfMarkup removes tags from fields that should be plain text but
// occasionally arrive with markup (titles, mostly).
func stripTagsIfMarkup(s string) string {
	if strings.Contains(s, "<") {
		return StripTags(s)
	}
	return s
}

func escapeAttr(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func escapeText(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// StripTags removes anything that looks like an HTML tag, leaving text.
func StripTags(s string) string {
	if !strings.Contains(s, "<") {
		return cleanText(html.UnescapeString(s))
	}
	var out strings.Builder
	out.Grow(len(s))
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
			out.WriteByte(' ')
		case depth == 0:
			out.WriteRune(r)
		}
	}
	// Entities must be decoded as well as tags removed. Every caller is asking
	// for text to show a person — a queue teaser, a preview line — and a feed
	// that writes "I&rsquo;ve" renders that literally otherwise.
	return cleanText(html.UnescapeString(out.String()))
}
