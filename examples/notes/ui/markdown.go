package ui

import (
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A small CommonMark subset renderer: markdown text → a list of block widgets
// (headings, paragraphs, fenced code, bullet lists), with inline bold/italic/
// code/links/[[wikilinks]]. Hand-rolled in the spirit of HN's parseSpans — no
// markdown dependency. Good enough to read notes; not a spec-complete parser.

type mdStyle struct {
	Text    paint.Color
	Heading paint.Color
	Code    paint.Color
	CodeBG  paint.Color
	Link    paint.Color
	Meta    paint.Color
	Size    float32 // base body size
}

// seg is the base style inline spans inherit (bold/italic/code override it).
type seg struct {
	font  string
	color paint.Color
}

// renderMarkdown turns src into block widgets. onLink is invoked when a link or
// [[wikilink]] span is tapped (url is the raw href, or "note:Name" for a wikilink).
func renderMarkdown(src string, sty mdStyle, onLink func(url string)) []widget.Widget {
	lines := strings.Split(src, "\n")
	var blocks []widget.Widget
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "":
			i++
		case strings.HasPrefix(t, "```"):
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				code = append(code, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume closing fence
			}
			blocks = append(blocks, codeBlock(strings.Join(code, "\n"), sty))
		case headingLevel(t) > 0:
			lvl := headingLevel(t)
			blocks = append(blocks, heading(strings.TrimSpace(t[lvl:]), lvl, sty, onLink))
			i++
		case isBullet(t):
			var items []widget.Widget
			for i < len(lines) && isBullet(strings.TrimSpace(lines[i])) {
				items = append(items, bulletItem(strings.TrimSpace(lines[i])[2:], sty, onLink))
				i++
			}
			blocks = append(blocks, block(widget.Column(items...)))
		case isIndentedCode(lines[i]):
			// A run of lines indented by a tab or 4+ spaces is a code block.
			var code []string
			for i < len(lines) && (isIndentedCode(lines[i]) || strings.TrimSpace(lines[i]) == "") {
				code = append(code, dedentCode(lines[i]))
				i++
			}
			blocks = append(blocks, codeBlock(strings.TrimRight(strings.Join(code, "\n"), "\n"), sty))
		default:
			var para []string
			for i < len(lines) {
				lt := strings.TrimSpace(lines[i])
				if lt == "" || headingLevel(lt) > 0 || isBullet(lt) || strings.HasPrefix(lt, "```") {
					break
				}
				para = append(para, lt)
				i++
			}
			blocks = append(blocks, paragraph(strings.Join(para, " "), sty, onLink))
		}
	}
	return blocks
}

// headingItem is one entry in a note's outline.
type headingItem struct {
	Level int
	Text  string
}

// extractHeadings returns the note's ATX headings in order, skipping fenced
// code blocks (so a "# comment" inside code is not mistaken for a heading).
func extractHeadings(src string) []headingItem {
	var out []headingItem
	inFence := false
	for line := range strings.SplitSeq(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if lvl := headingLevel(t); lvl > 0 {
			out = append(out, headingItem{Level: lvl, Text: strings.TrimSpace(t[lvl:])})
		}
	}
	return out
}

// wikilinkTargets returns the names referenced by [[wikilinks]] in src.
func wikilinkTargets(src string) []string {
	var out []string
	for i := 0; ; {
		j := strings.Index(src[i:], "[[")
		if j < 0 {
			break
		}
		start := i + j + 2
		end := strings.Index(src[start:], "]]")
		if end < 0 {
			break
		}
		out = append(out, strings.TrimSpace(src[start:start+end]))
		i = start + end + 2
	}
	return out
}

func headingLevel(t string) int {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n >= 1 && n <= 6 && n < len(t) && t[n] == ' ' {
		return n
	}
	return 0
}

func isBullet(t string) bool {
	return len(t) >= 2 && (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' '
}

// isIndentedCode reports whether a raw line is an indented code line: a leading
// tab or 4+ spaces, with actual content.
func isIndentedCode(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")
}

// dedentCode strips one level of code indentation (a tab or four spaces).
func dedentCode(line string) string {
	switch {
	case strings.HasPrefix(line, "\t"):
		return line[1:]
	case strings.HasPrefix(line, "    "):
		return line[4:]
	}
	return line
}

// block adds the standard gap beneath a rendered block.
func block(w widget.Widget) widget.Widget {
	return widget.Padding{Insets: geom.Insets{Bottom: 10}, Child: w}
}

func heading(text string, lvl int, sty mdStyle, onLink func(string)) widget.Widget {
	size := []float32{30, 24, 19, 16, 15, 14}[lvl-1]
	return block(widget.Rich{
		Spans:  inlineSpans(text, seg{font: "bold", color: sty.Heading}, sty),
		Size:   size,
		OnLink: onLink,
	})
}

func paragraph(text string, sty mdStyle, onLink func(string)) widget.Widget {
	return block(widget.Rich{
		Spans:  inlineSpans(text, seg{color: sty.Text}, sty),
		Size:   sty.Size,
		OnLink: onLink,
	})
}

func codeBlock(code string, sty mdStyle) widget.Widget {
	return block(widget.Decorated{Color: sty.CodeBG, Radius: 6, Child: widget.Padding{
		Insets: geom.InsetsSymmetric(12, 10),
		Child:  widget.Rich{Spans: []layout.RichSpan{{Text: code, Font: "mono", Color: sty.Code}}, Size: sty.Size},
	}})
}

func bulletItem(text string, sty mdStyle, onLink func(string)) widget.Widget {
	row := widget.Row(
		widget.Sized{W: 18, Child: widget.Text{Value: "•", Size: sty.Size, Color: sty.Meta}},
		widget.Expand(widget.Rich{Spans: inlineSpans(text, seg{color: sty.Text}, sty), Size: sty.Size, OnLink: onLink}),
	)
	row.CrossAlign = layout.CrossStart
	return block(row)
}

// inlineSpans scans text for inline markers, producing styled spans. base is the
// inherited style; bold/italic recurse with an overridden font.
func inlineSpans(text string, base seg, sty mdStyle) []layout.RichSpan {
	var out []layout.RichSpan
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, layout.RichSpan{Text: buf.String(), Font: base.font, Color: base.color})
			buf.Reset()
		}
	}
	i := 0
	for i < len(text) {
		rest := text[i:]
		switch {
		case strings.HasPrefix(rest, "[["):
			if end := strings.Index(rest[2:], "]]"); end >= 0 {
				name := rest[2 : 2+end]
				flush()
				out = append(out, layout.RichSpan{Text: name, Font: base.font, Color: sty.Link, Underline: true, Link: "note:" + name})
				i += 2 + end + 2
				continue
			}
		case rest[0] == '[':
			if label, url, n, ok := parseLink(rest); ok {
				flush()
				out = append(out, layout.RichSpan{Text: label, Font: base.font, Color: sty.Link, Underline: true, Link: url})
				i += n
				continue
			}
		case rest[0] == '`':
			if end := strings.IndexByte(rest[1:], '`'); end >= 0 {
				flush()
				out = append(out, layout.RichSpan{Text: rest[1 : 1+end], Font: "mono", Color: sty.Code})
				i += 1 + end + 1
				continue
			}
		case strings.HasPrefix(rest, "**"), strings.HasPrefix(rest, "__"):
			d := rest[:2]
			if inner := closeDelim(rest, d); inner > 0 {
				flush()
				out = append(out, inlineSpans(rest[2:inner], seg{font: "bold", color: base.color}, sty)...)
				i += inner + 2
				continue
			}
		case rest[0] == '*', rest[0] == '_':
			d := rest[:1]
			if inner := closeDelim(rest, d); inner > 0 {
				flush()
				out = append(out, inlineSpans(rest[1:inner], seg{font: "italic", color: base.color}, sty)...)
				i += inner + 1
				continue
			}
		}
		buf.WriteByte(text[i])
		i++
	}
	flush()
	return out
}

// closeDelim returns the index in s where the closing delimiter d begins, given
// s starts with d, or 0 if there is no non-empty closing match.
func closeDelim(s, d string) int {
	if idx := strings.Index(s[len(d):], d); idx > 0 {
		return len(d) + idx
	}
	return 0
}

// parseLink parses a leading "[label](url)"; n is the bytes consumed.
func parseLink(s string) (label, url string, n int, ok bool) {
	if !strings.HasPrefix(s, "[") {
		return
	}
	close := strings.IndexByte(s, ']')
	if close < 0 || close+1 >= len(s) || s[close+1] != '(' {
		return
	}
	end := strings.IndexByte(s[close+2:], ')')
	if end < 0 {
		return
	}
	return s[1:close], s[close+2 : close+2+end], close + 2 + end + 1, true
}
