package apptest_test

import (
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// A plain tap on something inside a Scroll must leave the app idle: the
// scroll's fling sampler used to keep ticking after any tap that was not a
// drag, so a page with a scroll never settled once touched.
func TestTapInsideScrollSettles(t *testing.T) {
	root := widget.Navigator{Home: widget.Scroll{Child: widget.Column(
		theme.Button{Label: "Tap me", OnTap: func() {}},
		widget.Sized{H: 2000},
	)}}
	a := apptest.New(t, root, apptest.Size(300, 400))
	a.TapLabel("Tap me")
	a.Settle() // fails the test if anything is still animating after 5 s
}
