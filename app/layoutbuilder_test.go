package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/widget"
)

type respApp struct{ hook func(*respState) }
func (a respApp) CreateState() widget.State { s := &respState{}; s.hook = a.hook; return s }
type respState struct {
	widget.StateBase[respApp]
	hook func(*respState)
	mode string
}
func (s *respState) Init(widget.Ctx) { s.hook(s) }
func (s *respState) Build(widget.Ctx) widget.Widget {
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		if cs.Max.W > 600 {
			s.mode = "wide"
		} else {
			s.mode = "narrow"
		}
		return widget.Sized{W: 10, H: 10}
	}}
}

func TestLayoutBuilderResponsive(t *testing.T) {
	var st *respState
	h, err := NewHeadless(respApp{hook: func(s *respState) { st = s }},
		Config{Size: geom.Size{W: 400, H: 400}, Font: goregular.TTF}, 1)
	if err != nil { t.Fatal(err) }
	// First frame observes constraints; second builds with them.
	h.Render()
	h.Render()
	if st.mode != "narrow" {
		t.Fatalf("400px wide should be narrow, got %q", st.mode)
	}
	// Resize wide.
	h.Resize(geom.Size{W: 900, H: 400})
	h.Render()
	h.Render()
	if st.mode != "wide" {
		t.Fatalf("900px wide should be wide, got %q", st.mode)
	}
}
