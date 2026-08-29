package app_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// The debug hooks the docs promised.
//
// Config.Debug's comment cited core.SetDebugPaint and Render's cited
// core.Skipped, both on an unexported struct — named in documentation and
// reachable by nobody. Skipped is the assertion a damage-tracking test wants:
// not that the pixels match, but that the work was skipped.
func TestSkippedReportsRetainedFrames(t *testing.T) {
	h, err := app.NewHeadless(widget.Sized{W: 40, H: 40}, app.Config{
		Size: geom.Size{W: 80, H: 80}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if h.Skipped() {
		t.Error("the first frame cannot be a skip; there was nothing retained to reuse")
	}
	h.Render()
	if !h.Skipped() {
		t.Error("an unchanged scene rasterized again; the retained surface was not reused")
	}
	// The toggle has to work on a running app — an embedded host cannot restart
	// with a different Config.
	h.SetDebugPaint(true)
	h.SetDebugPaint(false)
}
