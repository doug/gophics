package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// Rows fill the list's width rather than being centred on it.
//
// A plain Column centres its children, so rows of different widths came out
// ragged — and worse, the drag maths assumes a row occupies the full cross
// extent, so the index a finger maps to stopped matching where the row was
// drawn. Every behavioural test still passed: the defect only exists in the
// geometry.
func TestReorderableRowsFillTheWidth(t *testing.T) {
	labels := []string{"short", "a considerably longer row", "mid"}
	a := apptest.New(t, widget.Reorderable{
		Count:      len(labels),
		ItemExtent: 40,
		Build: func(i int) widget.Widget {
			return widget.Sized{H: 40, Child: widget.Semantics{
				Role:  layout.RoleListItem,
				Label: labels[i],
				Child: widget.Text{Value: labels[i]},
			}}
		},
	}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 400}, Font: goregular.TTF,
	}))
	a.Render()

	var rows []layout.SemNode
	for _, n := range a.Nodes() {
		if n.Role == layout.RoleListItem {
			rows = append(rows, n)
		}
	}
	if len(rows) != len(labels) {
		t.Fatalf("laid out %d rows, want %d", len(rows), len(labels))
	}
	for i, n := range rows {
		if n.Rect.Min.X != rows[0].Rect.Min.X {
			t.Errorf("row %d (%q) starts at x=%g but row 0 starts at x=%g — rows "+
				"of different widths are being centred instead of filling the list",
				i, n.Label, n.Rect.Min.X, rows[0].Rect.Min.X)
		}
	}
}
