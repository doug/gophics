package theme

import (
	"strconv"
	"unicode"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Chip is a compact pill: a filter, a tag, a selected value. With OnTap it is
// a toggle (Selected fills it with the accent); without, it is a static label.
//
// A chip that can be turned on and off announces itself as a checkbox, because
// that is what it behaves like — a screen-reader user needs to hear the state,
// not the shape.
type Chip struct {
	Label string
	// Selected fills the chip with the accent color.
	Selected bool
	// OnTap makes the chip interactive; nil leaves it a static tag.
	OnTap func()
	// Color overrides the selected fill (e.g. a per-category accent from the
	// chart palette). Zero → the theme's Primary.
	Color paint.Color
}

func (ch Chip) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	accent := ch.Color
	if accent == (paint.Color{}) {
		accent = th.Primary
	}

	fill, text, border := paint.Color{}, th.Text, th.Outline
	if ch.Selected {
		fill, text, border = accent, th.OnPrimary, accent
	}

	body := widget.Decorated{
		Color: fill, Radius: 999, BorderColor: border, BorderWidth: 1,
		Child: widget.Padding{
			Insets: geom.Insets{Left: 10, Right: 10, Top: 5, Bottom: 5},
			Child:  widget.Text{S: ch.Label, Size: th.Type.Label, Color: text, MaxLines: 1},
		},
	}
	if ch.OnTap == nil {
		return body
	}
	selected := ch.Selected
	return widget.Interactive{
		Sem: &layout.SemInfo{
			Role: layout.RoleCheckbox, Label: ch.Label, Checked: &selected,
		},
		Handler: widget.Handler{OnTap: ch.OnTap},
		Child:   body,
	}
}

// Badge is a small count or status dot, usually overlaid on an icon. Zero
// renders nothing unless Dot is set, so a caller can bind it straight to an
// unread count without branching.
type Badge struct {
	// Count is rendered inside the badge; values above Max show as "Max+".
	Count int
	// Max caps the displayed number. 0 → 99.
	Max int
	// Dot draws a bare dot with no number — "something changed" without
	// claiming how much.
	Dot bool
	// Color overrides the fill. Zero → the theme's Danger, the conventional
	// "needs attention" color.
	Color paint.Color
	// Label overrides the spoken text ("3 unread messages"). Without it the
	// count alone is announced, which is rarely enough on its own.
	Label string
}

func (b Badge) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	if !b.Dot && b.Count <= 0 {
		return widget.Sized{}
	}
	fill := b.Color
	if fill == (paint.Color{}) {
		fill = th.Danger
	}

	if b.Dot {
		return widget.Semantics{
			Label: b.Label,
			Child: widget.Decorated{Color: fill, Radius: 4, Child: widget.Sized{W: 8, H: 8}},
		}
	}

	max := b.Max
	if max <= 0 {
		max = 99
	}
	text := strconv.Itoa(b.Count)
	if b.Count > max {
		text = strconv.Itoa(max) + "+"
	}
	label := b.Label
	if label == "" {
		label = text
	}
	return widget.Semantics{
		Label: label,
		Child: widget.Decorated{
			Color: fill, Radius: 999,
			Child: widget.Padding{
				Insets: geom.Insets{Left: 5, Right: 5, Top: 1, Bottom: 1},
				Child: widget.Sized{H: 16, Child: widget.Center(
					widget.Text{S: text, Size: th.Type.Caption, Color: th.OnPrimary, MaxLines: 1})},
			},
		},
	}
}

// Avatar is a circular representation of a person or thing: an image when
// there is one, and otherwise initials on a color derived from the name — so
// a list of people is still visually distinguishable with no images loaded.
type Avatar struct {
	// Name supplies the initials and, when Color is unset, the fill.
	Name string
	// Image, when non-nil, replaces the initials (clipped to the circle).
	Image widget.Widget
	// Size is the diameter. 0 → 36.
	Size float32
	// Color overrides the derived fill.
	Color paint.Color
}

func (a Avatar) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	d := a.Size
	if d <= 0 {
		d = 36
	}
	fill := a.Color
	if fill == (paint.Color{}) {
		// Deriving the color from the name keeps one person the same color
		// everywhere in the app without anyone storing a color.
		fill = th.ChartAt(nameHash(a.Name))
	}

	var child widget.Widget
	if a.Image != nil {
		child = a.Image
	} else {
		child = widget.Center(widget.Text{
			S: initials(a.Name), Size: d * 0.36, Font: FontBold, Color: th.OnPrimary, MaxLines: 1,
		})
	}
	return widget.Semantics{
		Role:  layout.RoleImage,
		Label: a.Name,
		Child: widget.Decorated{
			Color:  fill,
			Radius: d / 2,
			Child:  widget.Sized{W: d, H: d, Child: child},
		},
	}
}

// initials takes up to two leading letters from the name's words. Non-letters
// are skipped so "  (dr.) ada lovelace" still yields "AL".
func initials(name string) string {
	var out []rune
	inWord := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				out = append(out, unicode.ToUpper(r))
				inWord = true
				if len(out) == 2 {
					return string(out)
				}
			}
		case unicode.IsSpace(r):
			inWord = false
		}
	}
	return string(out)
}

// nameHash maps a name to a stable palette index (FNV-1a, truncated).
func nameHash(name string) int {
	h := uint32(2166136261)
	for i := 0; i < len(name); i++ {
		h ^= uint32(name[i])
		h *= 16777619
	}
	return int(h % 6)
}
