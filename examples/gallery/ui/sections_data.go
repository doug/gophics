package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Tree ---------------------------------------------------------------------

// treeSection folds a small source tree. What is on show is the folding and
// the indentation: row content is ordinary widgets, which is the point of the
// widget owning expansion and nothing else.
//
// It also demonstrates what the tree publishes to assistive technology — each
// row is a treeitem carrying its expanded state, and the disclosure glyph is
// hidden so a screen reader reads the row rather than the triangle.
type treeSection struct{}

func (treeSection) CreateState() widget.State { return &treeState2{} }

type treeState2 struct {
	widget.StateBase[treeSection]
	lastToggled string
	open        bool
}

func (s *treeState2) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	leaf := func(id, name string) widget.TreeNode {
		return widget.TreeNode{ID: id, Child: widget.Text{
			Value: name, Size: th.Type.Body, Color: th.Text,
		}}
	}
	branch := func(id, name string, kids ...widget.TreeNode) widget.TreeNode {
		return widget.TreeNode{ID: id, Children: kids, Child: widget.Text{
			Value: name, Font: theme.FontBold, Size: th.Type.Body, Color: th.Text,
		}}
	}

	status := "tap a folder to fold it"
	if s.lastToggled != "" {
		verb := "collapsed"
		if s.open {
			verb = "expanded"
		}
		status = fmt.Sprintf("%s %s", s.lastToggled, verb)
	}

	return sectionColumn(
		groupLabel("A source tree"),
		theme.Card{Child: widget.Padding{
			Insets: geom.Insets{Top: 8, Bottom: 8, Left: 4, Right: 4},
			Child: widget.Tree{
				InitiallyExpanded: []string{"widget"},
				OnToggle: func(id string, expanded bool) {
					s.SetState(func() { s.lastToggled, s.open = id, expanded })
				},
				Nodes: []widget.TreeNode{
					branch("widget", "widget",
						leaf("w-tree", "tree.go"),
						leaf("w-list", "lazylist.go"),
						branch("w-internal", "internal",
							leaf("w-recon", "reconcile.go"),
						),
					),
					branch("theme", "theme",
						leaf("t-controls", "controls.go"),
					),
					leaf("readme", "README.md"),
				},
			},
		}},
		widget.Sized{H: 8},
		widget.Text{Value: status, Size: th.Type.Caption, Color: th.Muted},
	)
}

// --- Autocomplete --------------------------------------------------------------

// autocompleteSection filters an in-memory list as you type. Suggest runs
// during build, so the demo keeps it to a substring match over a small slice —
// which is exactly the advice in the widget's own documentation.
type autocompleteSection struct{}

func (autocompleteSection) CreateState() widget.State { return &autocompleteDemo{} }

type autocompleteDemo struct {
	widget.StateBase[autocompleteSection]
	value  string
	picked string
}

var timezones = []string{
	"Africa/Cairo", "America/Chicago", "America/New_York", "America/Sao_Paulo",
	"Asia/Kolkata", "Asia/Singapore", "Asia/Tokyo", "Australia/Sydney",
	"Europe/Berlin", "Europe/Lisbon", "Europe/London", "Pacific/Auckland",
}

func (s *autocompleteDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	field := widget.Autocomplete{
		Value:       s.value,
		Placeholder: "Start typing — try \"eu\" or \"asia\"…",
		MaxVisible:  6,
		Suggest: func(in string) []string {
			if in == "" {
				return nil
			}
			var out []string
			for _, z := range timezones {
				if strings.Contains(strings.ToLower(z), strings.ToLower(in)) {
					out = append(out, z)
				}
			}
			return out
		},
		OnChange: func(v string) { s.SetState(func() { s.value = v }) },
		OnPick:   func(v string) { s.SetState(func() { s.picked = v }) },
		Row: func(item string, highlighted bool) widget.Widget {
			bg := paint.Color{}
			if highlighted {
				bg = th.Primary.WithAlpha(0.14)
			}
			return widget.Decorated{
				Color: bg,
				Child: widget.Padding{
					Insets: geom.Insets{Top: 7, Bottom: 7, Left: 10, Right: 10},
					Child:  widget.Text{Value: item, Size: th.Type.Body, Color: th.Text},
				},
			}
		},
	}

	return sectionColumn(
		groupLabel("Type a timezone"),
		// Autocomplete is a widget-layer control and carries no styling of its
		// own, exactly like the raw TextField that theme.Field wraps. The field
		// chrome is the app's to supply.
		widget.Decorated{
			BorderColor: th.Outline,
			BorderWidth: 1,
			Radius:      8,
			Child: widget.Padding{
				Insets: geom.Insets{Top: 10, Bottom: 10, Left: 12, Right: 12},
				Child:  field,
			},
		},
		widget.Sized{H: 10},
		widget.Text{
			Value: "Picked: " + orDash(s.picked),
			Size:  th.Type.Caption,
			Color: th.Muted,
		},
	)
}

// --- Reorderable ----------------------------------------------------------------

// reorderSection drags rows into a new order. Rows are a uniform height
// because Reorderable requires it — with variable extents the drop index
// depends on the sizes being crossed, which are not known mid-drag.
type reorderSection struct{}

func (reorderSection) CreateState() widget.State { return &reorderDemo{} }

type reorderDemo struct {
	widget.StateBase[reorderSection]
	items []string
	moves int
}

func (s *reorderDemo) Init(widget.Ctx) {
	s.items = []string{"Mercury", "Venus", "Earth", "Mars", "Jupiter"}
}

func (s *reorderDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	const rowH = 44

	return sectionColumn(
		groupLabel("Drag a row by any part of it"),
		widget.Sized{H: rowH * 5, Child: widget.Reorderable{
			Count:      len(s.items),
			ItemExtent: rowH,
			// The built row has to be exactly ItemExtent tall. The list maps
			// finger position to index by multiplying that number, so a row
			// that renders taller overflows its own list and drops in the
			// wrong place.
			Build: func(i int) widget.Widget {
				// The row's height is fixed at ItemExtent, so its padding has
				// to fit inside it. Card already pads by 12, and adding
				// another 8 on top left the content with less height than
				// nothing (44 - 6 - 24 - 16 = -2): the handle and the label
				// spilled out of the bottom of the card instead of sitting in
				// the middle of it. One padding, and Align centres what is
				// left over.
				return widget.Sized{H: rowH, Child: widget.Padding{
					Insets: geom.Insets{Bottom: 6},
					Child: theme.Card{Pad: 8, Child: widget.Align{
						X: 0, Y: 0.5, Directional: true,
						Child: widget.Row(
							widget.Text{Value: "=", Font: theme.FontBold, Size: th.Type.Body, Color: th.Muted},
							widget.Sized{W: 10},
							widget.Text{Value: s.items[i], Size: th.Type.Body, Color: th.Text},
						),
					}},
				}}
			},
			OnReorder: func(from, to int) {
				s.SetState(func() {
					item := s.items[from]
					rest := append(s.items[:from:from], s.items[from+1:]...)
					s.items = append(rest[:to:to], append([]string{item}, rest[to:]...)...)
					s.moves++
				})
			},
		}},
		widget.Sized{H: 6},
		widget.Text{
			Value: fmt.Sprintf("%d reorders · %s", s.moves, strings.Join(s.items, " → ")),
			Size:  th.Type.Caption,
			Color: th.Muted,
			Wrap:  true,
		},
	)
}

// --- Drag and drop ---------------------------------------------------------------

// dragDropSection moves chips between two bins. DragHost is what paints the
// in-flight preview above everything else, so the demo wraps itself in one
// rather than relying on an ancestor to have done it.
type dragDropSection struct{}

func (dragDropSection) CreateState() widget.State { return &dragDropDemo{} }

type dragDropDemo struct {
	widget.StateBase[dragDropSection]
	todo, done []string
}

func (s *dragDropDemo) Init(widget.Ctx) {
	s.todo = []string{"Sketch", "Draft", "Review"}
	s.done = []string{"Outline"}
}

func (s *dragDropDemo) move(item string, toDone bool) {
	s.SetState(func() {
		drop := func(from []string) []string {
			out := from[:0:0]
			for _, v := range from {
				if v != item {
					out = append(out, v)
				}
			}
			return out
		}
		s.todo, s.done = drop(s.todo), drop(s.done)
		if toDone {
			s.done = append(s.done, item)
		} else {
			s.todo = append(s.todo, item)
		}
	})
}

func (s *dragDropDemo) chip(th theme.Theme, item string) widget.Widget {
	face := widget.Decorated{
		Color:  th.Primary.WithAlpha(0.16),
		Radius: 8,
		Child: widget.Padding{
			Insets: geom.Insets{Top: 6, Bottom: 6, Left: 10, Right: 10},
			Child:  widget.Text{Value: item, Size: th.Type.Body, Color: th.Text},
		},
	}
	return widget.Padding{
		Insets: geom.Insets{Right: 6, Bottom: 6},
		Child: widget.Draggable{
			Payload: item,
			// Inside a scrollable page a plain drag means scroll, so the
			// gesture has to be claimed deliberately.
			LongPressToStart: true,
			Child:            face,
		},
	}
}

func (s *dragDropDemo) bin(th theme.Theme, title string, items []string, toDone bool) widget.Widget {
	chips := make([]widget.Widget, 0, len(items))
	for _, it := range items {
		chips = append(chips, s.chip(th, it))
	}
	var body widget.Widget = widget.Wrap{Spacing: 6, RunSpacing: 6, Children: chips}
	if len(items) == 0 {
		body = widget.Text{Value: "empty", Size: th.Type.Caption, Color: th.Muted}
	}

	return widget.DropTarget{
		Accept: func(p any) bool {
			item, ok := p.(string)
			if !ok {
				return false
			}
			// A bin does not accept what it already holds: dropping a chip back
			// where it started should read as a no-op, not a move.
			return !slices.Contains(items, item)
		},
		OnDrop: func(p any, _ geom.Pt) {
			if item, ok := p.(string); ok {
				s.move(item, toDone)
			}
		},
		Builder: func(hovering bool) widget.Widget {
			border := th.Border
			fill := paint.Color{}
			if hovering {
				border, fill = th.Primary, th.Primary.WithAlpha(0.08)
			}
			return widget.Decorated{
				Color: fill, BorderColor: border, BorderWidth: 1.5, Radius: 10,
				Child: widget.Padding{
					Insets: geom.Insets{Top: 10, Bottom: 10, Left: 10, Right: 10},
					Child: widget.Column(
						widget.Text{Value: title, Font: theme.FontBold, Size: th.Type.Label, Color: th.Muted},
						widget.Sized{H: 8},
						body,
					),
				},
			}
		},
	}
}

func (s *dragDropDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Drag a chip across — long-press first on touch"),
		widget.DragHost{Child: widget.Column(
			s.bin(th, "TO DO", s.todo, false),
			widget.Sized{H: 10},
			s.bin(th, "DONE", s.done, true),
		)},
		widget.Sized{H: 8},
		widget.Text{
			Value: "A bin refuses a chip it already holds — the border only lights for a drop it will take.",
			Size:  th.Type.Caption,
			Color: th.Muted,
			Wrap:  true,
		},
	)
}

// --- Rich text and selection -------------------------------------------------------

// richTextSection shows styled spans with a tappable link, wrapped in a
// SelectionArea so the text can be dragged over and copied — the two text
// capabilities that are not part of a plain Text.
type richTextSection struct{}

func (richTextSection) CreateState() widget.State { return &richTextDemo{} }

type richTextDemo struct {
	widget.StateBase[richTextSection]
	tapped string
}

func (s *richTextDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Styled spans, one of them a link"),
		theme.Card{Child: widget.Padding{
			Insets: geom.Insets{Top: 12, Bottom: 12, Left: 12, Right: 12},
			Child: widget.SelectionArea{Child: widget.Rich{
				Size: th.Type.Body,
				Spans: []layout.RichSpan{
					{Text: "Rich text runs ", Color: th.Text},
					{Text: "bold", Font: theme.FontBold, Color: th.Text},
					{Text: " and ", Color: th.Text},
					{Text: "coloured", Color: th.Primary},
					{Text: " spans in one paragraph, wraps them together, and can carry a ", Color: th.Text},
					{Text: "link", Color: th.Primary, Underline: true, Link: "https://gophics.com"},
					{Text: ". Drag across any of it to select.", Color: th.Text},
				},
				OnLink: func(url string) { s.SetState(func() { s.tapped = url }) },
			}},
		}},
		widget.Sized{H: 10},
		widget.Text{
			Value: "Link tapped: " + orDash(s.tapped),
			Size:  th.Type.Caption,
			Color: th.Muted,
		},
	)
}

// --- Transform ---------------------------------------------------------------------

// transformSection applies a 2D transform to a live widget, not a picture:
// the button underneath stays tappable through the rotation and scale, which
// is the part worth showing.
type transformSection struct{}

func (transformSection) CreateState() widget.State { return &transformDemo{} }

type transformDemo struct {
	widget.StateBase[transformSection]
	angle float32
	scale float32
	taps  int
}

func (s *transformDemo) Init(widget.Ctx) { s.scale = 1 }

func (s *transformDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	if s.scale == 0 {
		s.scale = 1
	}
	return sectionColumn(
		groupLabel("A transformed, still-live widget"),
		widget.Center(widget.Sized{H: 120, Child: widget.Center(
			widget.Transform{
				T:      paint.Transform{Rotation: s.angle, SX: s.scale, SY: s.scale},
				Center: true,
				Child: theme.Button{
					Label: fmt.Sprintf("Tapped %d×", s.taps),
					OnTap: func() { s.SetState(func() { s.taps++ }) },
				},
			},
		)}),
		widget.Sized{H: 8},
		widget.Row(
			theme.Button{Label: "Rotate", OnTap: func() {
				s.SetState(func() { s.angle += 0.20 })
			}},
			widget.Sized{W: 8},
			theme.Button{Label: "Grow", OnTap: func() {
				s.SetState(func() { s.scale = clampF(s.scale+0.15, 0.5, 1.8) })
			}},
			widget.Sized{W: 8},
			// Reset returns the demo to how it was found, tap count included:
			// leaving "Tapped 7×" under a reset transform reads as the button
			// having missed the reset rather than as a deliberate exclusion.
			theme.Button{Label: "Reset", OnTap: func() {
				s.SetState(func() { s.angle, s.scale, s.taps = 0, 1, 0 })
			}},
		),
		widget.Sized{H: 8},
		widget.Text{
			Value: "Hit testing follows the transform — the button still takes taps where it is drawn.",
			Size:  th.Type.Caption,
			Color: th.Muted,
			Wrap:  true,
		},
	)
}

func clampF(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- Directionality ------------------------------------------------------------------

// rtlSection mirrors a layout rather than a string. Everything inside flips —
// row order, alignment, padding — which is what makes right-to-left a layout
// property rather than a text one.
type rtlSection struct{}

func (rtlSection) CreateState() widget.State { return &rtlDemo{} }

type rtlDemo struct {
	widget.StateBase[rtlSection]
	rtl bool
}

func (s *rtlDemo) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	dir := widget.DirLTR
	if s.rtl {
		dir = widget.DirRTL
	}
	sample := theme.Card{Child: widget.Padding{
		Insets: geom.Insets{Top: 12, Bottom: 12, Left: 12, Right: 12},
		Child: widget.Column(
			widget.Row(
				widget.Text{Value: "<", Font: theme.FontBold, Size: th.Type.Body, Color: th.Primary},
				widget.Sized{W: 8},
				widget.Text{Value: "Leading", Font: theme.FontBold, Size: th.Type.Body, Color: th.Text},
				widget.Spacer(),
				widget.Text{Value: "Trailing", Size: th.Type.Body, Color: th.Muted},
			),
			widget.Sized{H: 8},
			widget.Text{
				Value: "Padding, row order and alignment all mirror; the glyphs do not.",
				Size:  th.Type.Caption,
				Color: th.Muted,
				Wrap:  true,
			},
		),
	}}

	return sectionColumn(
		groupLabel("Layout mirroring"),
		widget.Directionality{Dir: dir, Child: sample},
		widget.Sized{H: 10},
		widget.Row(
			theme.Switch{
				On:       s.rtl,
				Label:    "Right to left",
				OnChange: func(v bool) { s.SetState(func() { s.rtl = v }) },
			},
			widget.Sized{W: 10},
			widget.Text{Value: "Right to left", Size: th.Type.Body, Color: th.Text},
		),
	)
}
