package extract

import (
	"bytes"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// allowedTags is the markup vocabulary we emit. Everything else is unwrapped so
// its text survives. The set is chosen to be valid XHTML and to render on
// e-ink readers, which ignore most styling anyway.
var allowedTags = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true,
	"blockquote": true, "br": true, "caption": true, "cite": true, "code": true,
	"col": true, "colgroup": true, "dd": true, "del": true, "div": true,
	"dl": true, "dt": true, "em": true, "figcaption": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"hr": true, "i": true, "img": true, "ins": true, "kbd": true, "li": true,
	"mark": true, "ol": true, "p": true, "pre": true, "q": true, "rp": true,
	"rt": true, "ruby": true, "s": true, "samp": true, "small": true,
	"span": true, "strong": true, "sub": true, "sup": true, "table": true,
	"tbody": true, "td": true, "tfoot": true, "th": true, "thead": true,
	"time": true, "tr": true, "u": true, "ul": true, "var": true, "wbr": true,
}

// renameTags maps presentational and obsolete elements onto modern equivalents.
var renameTags = map[string]string{
	"font": "span", "center": "div", "big": "strong", "strike": "del",
	"tt": "code", "acronym": "abbr", "article": "div", "section": "div",
	"main": "div", "header": "div", "hgroup": "div", "details": "div",
	"summary": "p", "picture": "span", "video": "div", "audio": "div",
}

// attrAllowlist names the attributes kept per tag. Class, id and style are
// deliberately absent: they are useless without the origin stylesheet and they
// bloat the output.
var attrAllowlist = map[string]map[string]bool{
	"a":     {"href": true, "title": true},
	"img":   {"src": true, "alt": true, "title": true, "width": true, "height": true},
	"td":    {"colspan": true, "rowspan": true},
	"th":    {"colspan": true, "rowspan": true, "scope": true},
	"col":   {"span": true},
	"ol":    {"start": true, "reversed": true},
	"time":  {"datetime": true},
	"q":     {"cite": true},
	"del":   {"cite": true},
	"ins":   {"cite": true},
	"bdo":   {"dir": true},
	"table": {},
}

// cleanContent normalises a selected article subtree in place.
func cleanContent(root *html.Node, base *url.URL, opts Options) {
	dropJunk(root)
	fixImages(root, opts)
	absolutize(root, base)
	normalizeTags(root)
	stripAttrs(root)
	dropEmpty(root)
	collapseWrappers(root)
}

// dropJunk removes leftover furniture that survived into the chosen subtree:
// link farms, share bars, and anything explicitly marked as noise.
func dropJunk(root *html.Node) {
	total := textLen(root)
	isBulk := func(n *html.Node) bool {
		return total > 0 && float64(textLen(n))/float64(total) > bulkShare
	}

	var doomed []*html.Node
	walk(root, func(n *html.Node) bool {
		name := tag(n)
		if name == "" || n == root {
			return true
		}
		if tagsToDrop[name] {
			doomed = append(doomed, n)
			return false
		}
		if name == "iframe" {
			// Embeds cannot render on an e-reader; keep a link if there is one.
			doomed = append(doomed, n)
			return false
		}
		if name == "nav" || name == "aside" || name == "footer" {
			if !isBulk(n) {
				doomed = append(doomed, n)
				return false
			}
		}
		// As in stripNoise, a name-based match must not be allowed to delete the
		// bulk of the content it was meant to trim, and never condemns prose.
		if id := classID(n); id != "" && reUnlikely.MatchString(id) &&
			!reMaybe.MatchString(id) && !isProseLeaf(name) && !isBulk(n) {
			doomed = append(doomed, n)
			return false
		}
		// A list or div that is almost entirely links is a related-articles box.
		switch name {
		case "ul", "ol", "div", "p", "table":
			if t := textLen(n); t > 40 && linkDensity(n) > 0.8 && !isBulk(n) {
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

// fixImages resolves lazy-loading attributes into a plain src and discards
// tracking pixels. Publishers routinely leave src empty and put the real URL in
// data-src, so skipping this step loses most images.
func fixImages(root *html.Node, opts Options) {
	var doomed []*html.Node
	for _, img := range collect(root, "img", "source") {
		if !opts.KeepImages {
			doomed = append(doomed, img)
			continue
		}
		src := attr(img, "src")
		if src == "" || strings.HasPrefix(src, "data:image/gif") || strings.HasPrefix(src, "data:image/svg") {
			for _, key := range []string{"data-src", "data-original", "data-lazy-src", "data-hi-res-src"} {
				if v := attr(img, key); v != "" {
					src = v
					break
				}
			}
		}
		if src == "" {
			if v := attr(img, "srcset"); v != "" {
				src = largestFromSrcset(v)
			}
		}
		if src == "" {
			if v := attr(img, "data-srcset"); v != "" {
				src = largestFromSrcset(v)
			}
		}
		if src == "" || isTrackingPixel(img, src) {
			doomed = append(doomed, img)
			continue
		}
		setAttr(img, "src", src)
	}
	for _, n := range doomed {
		remove(n)
	}
}

// largestFromSrcset picks the highest-width entry from a srcset attribute.
func largestFromSrcset(v string) string {
	best, bestW := "", -1
	for part := range strings.SplitSeq(v, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		w := 0
		if len(fields) > 1 {
			d := strings.TrimSuffix(fields[1], "w")
			if d == fields[1] {
				d = strings.TrimSuffix(fields[1], "x")
			}
			w, _ = strconv.Atoi(d)
		}
		if w > bestW {
			best, bestW = fields[0], w
		}
	}
	return best
}

func isTrackingPixel(img *html.Node, src string) bool {
	for _, key := range []string{"width", "height"} {
		if v := attr(img, key); v != "" {
			if n, err := strconv.Atoi(strings.TrimSuffix(v, "px")); err == nil && n > 0 && n <= 2 {
				return true
			}
		}
	}
	lower := strings.ToLower(src)
	return strings.Contains(lower, "1x1.") || strings.Contains(lower, "/pixel") ||
		strings.Contains(lower, "tracking") || strings.Contains(lower, "/beacon")
}

// absolutize rewrites relative URLs against the page URL, since an EPUB has no
// meaningful base of its own.
func absolutize(root *html.Node, base *url.URL) {
	if base == nil {
		return
	}
	walk(root, func(n *html.Node) bool {
		switch tag(n) {
		case "a":
			if href := attr(n, "href"); href != "" {
				if u, err := base.Parse(href); err == nil {
					setAttr(n, "href", u.String())
				}
			}
		case "img":
			if src := attr(n, "src"); src != "" {
				if u, err := base.Parse(src); err == nil {
					setAttr(n, "src", u.String())
				}
			}
		}
		return true
	})
}

// normalizeTags renames legacy elements and unwraps anything outside the
// allowlist, keeping the text.
func normalizeTags(root *html.Node) {
	var toUnwrap []*html.Node
	walk(root, func(n *html.Node) bool {
		name := tag(n)
		if name == "" {
			return true
		}
		if to, ok := renameTags[name]; ok {
			n.Data = to
			n.DataAtom = 0
			return true
		}
		if !allowedTags[name] && n != root {
			toUnwrap = append(toUnwrap, n)
		}
		return true
	})
	// Unwrap deepest-last so parents are still attached when children move.
	for _, t := range slices.Backward(toUnwrap) {
		unwrap(t)
	}
}

func stripAttrs(root *html.Node) {
	walk(root, func(n *html.Node) bool {
		if tag(n) == "" {
			return true
		}
		allowed, ok := attrAllowlist[n.Data]
		if !ok {
			n.Attr = nil
			return true
		}
		keepAttrs(n, allowed)
		// An anchor with a javascript: or empty target is not a link.
		if n.Data == "a" {
			href := strings.ToLower(strings.TrimSpace(attr(n, "href")))
			if href == "" || strings.HasPrefix(href, "javascript:") {
				n.Attr = nil
			}
		}
		return true
	})
}

// dropEmpty removes elements left with neither text nor meaningful children.
func dropEmpty(root *html.Node) {
	// Repeat until stable: emptying a child can empty its parent.
	for range 4 {
		var doomed []*html.Node
		walk(root, func(n *html.Node) bool {
			name := tag(n)
			if name == "" || n == root {
				return true
			}
			switch name {
			case "br", "hr", "img", "wbr", "col", "td", "th":
				return true
			}
			if textLen(n) > 0 {
				return true
			}
			// No text: keep only if it still contains an image.
			if len(collect(n, "img")) > 0 {
				return true
			}
			doomed = append(doomed, n)
			return false
		})
		if len(doomed) == 0 {
			return
		}
		for _, n := range doomed {
			remove(n)
		}
	}
}

// collapseWrappers unwraps divs and spans that add a nesting level without
// adding meaning, which keeps the EPUB markup shallow and readable.
func collapseWrappers(root *html.Node) {
	var toUnwrap []*html.Node
	walk(root, func(n *html.Node) bool {
		if n == root {
			return true
		}
		switch tag(n) {
		case "div":
			// A div whose only child is a block element is pure wrapping.
			kids := children(n)
			elems := 0
			var only *html.Node
			for _, k := range kids {
				switch k.Type {
				case html.ElementNode:
					elems++
					only = k
				case html.TextNode:
					if strings.TrimSpace(k.Data) != "" {
						elems = 99
					}
				}
			}
			if elems == 1 && only != nil && !isPhrasing(tag(only)) {
				toUnwrap = append(toUnwrap, n)
			}
		case "span":
			if len(n.Attr) == 0 {
				toUnwrap = append(toUnwrap, n)
			}
		}
		return true
	})
	for _, t := range slices.Backward(toUnwrap) {
		unwrap(t)
	}
}

// uiLabels are interface strings that survive extraction because they sit in
// ordinary elements next to the prose. Matching is exact after normalisation,
// so an article discussing "Advertisement" in a sentence is unaffected.
var uiLabels = map[string]bool{
	"read later": true, "save": true, "share": true, "share this": true,
	"advertisement": true, "advertisements": true, "sign in": true,
	"sign up": true, "subscribe": true, "log in": true, "comment": true,
	"comments": true, "print": true, "email": true, "copy link": true,
	"listen": true, "watch": true, "follow": true, "menu": true,
	"skip to content": true, "related": true, "most popular": true,
	"support our journalism": true, "read more": true, "view comments": true,
}

// dropBoilerplate removes the residue that scoring cannot see: a repeat of the
// headline, a duplicated dateline, and stray interface labels. Publishers put
// the title inside the article container as well as in <head>, so without this
// every chapter begins by stating its own title twice.
func dropBoilerplate(root *html.Node, title string) {
	wantTitle := normalizeLabel(title)

	// Only the opening of the article can hold a duplicate headline; a matching
	// heading later on is a real section.
	headings := collect(root, "h1", "h2", "h3", "h4", "h5", "h6")
	for i, h := range headings {
		if i >= 3 {
			break
		}
		if wantTitle == "" {
			break
		}
		if t := normalizeLabel(textContent(h)); t != "" && titlesMatch(t, wantTitle) {
			remove(h)
		}
	}

	var doomed []*html.Node
	walk(root, func(n *html.Node) bool {
		name := tag(n)
		if name == "" || n == root {
			return true
		}
		switch name {
		case "p", "div", "span", "li", "button", "a", "h1", "h2", "h3", "h4", "h5", "h6":
			t := textContent(n)
			if len(t) > 40 {
				return true
			}
			if uiLabels[normalizeLabel(t)] {
				doomed = append(doomed, n)
				return false
			}
		}
		return true
	})
	for _, n := range doomed {
		remove(n)
	}

	dropEmpty(root)
}

// titlesMatch compares a heading with the article title, tolerating a trailing
// or leading fragment such as an appended subtitle.
func titlesMatch(a, b string) bool {
	if a == b {
		return true
	}
	long, short := a, b
	if len(short) > len(long) {
		long, short = short, long
	}
	// Require the shorter to be a substantial prefix of the longer, so a short
	// section heading cannot match a long title by accident.
	return len(short) >= 12 && strings.HasPrefix(long, short)
}

// normalizeLabel lowercases, collapses whitespace and strips surrounding
// punctuation so that comparisons are about words rather than formatting.
func normalizeLabel(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.Trim(s, " \t.:;!?·|—–- ")
}

// renderChildren serialises a node's children. x/net/html writes void elements
// self-closed and always quotes attribute values, so the output is valid XHTML
// once embedded in an XHTML document.
func renderChildren(n *html.Node) string {
	var buf bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			continue
		}
	}
	return strings.TrimSpace(buf.String())
}

// Sanitize cleans a fragment of untrusted HTML — typically the body a feed
// supplied directly — so it can be embedded in an EPUB. It applies the same
// normalisation as extraction but performs no content selection.
func Sanitize(fragment, pageURL string, opts Options) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	body := first(doc, "body")
	if body == nil {
		return ""
	}
	base, _ := url.Parse(pageURL)
	cleanContent(body, base, opts)
	return renderChildren(body)
}

// PlainText renders a fragment of HTML as text, for summaries and word counts.
func PlainText(fragment string) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return ""
	}
	return textContent(doc)
}
