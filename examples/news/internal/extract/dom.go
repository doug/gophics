package extract

import (
	"strings"

	"golang.org/x/net/html"
)

// tag returns the lowercase tag name, or "" for non-element nodes.
func tag(n *html.Node) string {
	if n == nil || n.Type != html.ElementNode {
		return ""
	}
	return n.Data
}

func attr(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// keepAttrs drops every attribute except those named.
func keepAttrs(n *html.Node, allowed map[string]bool) {
	if len(n.Attr) == 0 {
		return
	}
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		if allowed[a.Key] {
			kept = append(kept, a)
		}
	}
	n.Attr = kept
}

// remove detaches n from its parent.
func remove(n *html.Node) {
	if n != nil && n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}

// unwrap replaces n with its children, preserving order.
func unwrap(n *html.Node) {
	parent := n.Parent
	if parent == nil {
		return
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		parent.InsertBefore(c, n)
		c = next
	}
	parent.RemoveChild(n)
}

// children returns a snapshot of n's children, safe to mutate the tree while
// iterating.
func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

// walk visits n and its descendants in document order. Returning false from fn
// skips the node's children.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if n == nil {
		return
	}
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

// collect returns every descendant element whose tag is in names.
func collect(root *html.Node, names ...string) []*html.Node {
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []*html.Node
	walk(root, func(n *html.Node) bool {
		if want[tag(n)] {
			out = append(out, n)
		}
		return true
	})
	return out
}

// first returns the first descendant element with the given tag.
func first(root *html.Node, name string) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if tag(n) == name {
			found = n
			return false
		}
		return true
	})
	return found
}

// textContent concatenates the text of a subtree, normalising whitespace.
func textContent(n *html.Node) string {
	var sb strings.Builder
	walk(n, func(x *html.Node) bool {
		switch x.Type {
		case html.TextNode:
			sb.WriteString(x.Data)
		case html.ElementNode:
			switch x.Data {
			case "script", "style", "noscript", "template":
				return false
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6":
				sb.WriteByte(' ')
			}
		}
		return true
	})
	return strings.Join(strings.Fields(sb.String()), " ")
}

func textLen(n *html.Node) int { return len(textContent(n)) }

// linkDensity is the fraction of a node's text that sits inside anchors. A high
// value marks navigation, related-links blocks and tag clouds.
func linkDensity(n *html.Node) float64 {
	total := textLen(n)
	if total == 0 {
		return 0
	}
	var linked int
	for _, a := range collect(n, "a") {
		t := textLen(a)
		// A bare "#" anchor or an in-page jump is not really navigation away.
		if href := attr(a, "href"); strings.HasPrefix(href, "#") {
			t /= 2
		}
		linked += t
	}
	return float64(linked) / float64(total)
}

// classID concatenates a node's class and id for pattern matching.
func classID(n *html.Node) string {
	c, i := attr(n, "class"), attr(n, "id")
	switch {
	case c == "":
		return i
	case i == "":
		return c
	}
	return c + " " + i
}

// isProseLeaf reports whether a tag holds article text directly rather than
// wrapping other blocks. Such elements are never discarded on the strength of
// their class name.
func isProseLeaf(name string) bool {
	switch name {
	case "p", "li", "dd", "dt", "blockquote", "pre", "figcaption", "caption",
		"td", "th", "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return isPhrasing(name)
}

// isPhrasing reports whether a tag is inline content, which affects whether a
// wrapper can be safely unwrapped.
func isPhrasing(name string) bool {
	switch name {
	case "a", "abbr", "b", "bdi", "bdo", "br", "cite", "code", "data", "dfn",
		"em", "i", "img", "kbd", "mark", "q", "rp", "rt", "ruby", "s", "samp",
		"small", "span", "strong", "sub", "sup", "time", "u", "var", "wbr":
		return true
	}
	return false
}
