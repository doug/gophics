package apptest_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Size() holds whichever side of WithConfig it is written on.
//
// Options run in the order they are written and WithConfig replaces Config
// wholesale, so a Size() written before it was erased. The zero size then fell
// back to the 320x240 default and the test carried on against a phone layout
// it never asked for — every assertion running on the wrong tree, nothing
// failing, no warning. That is the whole reason this is worth a test: the
// failure mode is a passing test.
func TestSizeSurvivesWithConfigInEitherOrder(t *testing.T) {
	probe := func(t *testing.T, opts ...apptest.Option) geom.Size {
		t.Helper()
		var got geom.Size
		root := widget.Canvas{Draw: func(_ paint.Canvas, size geom.Size) { got = size }}
		a := apptest.New(t, root, opts...)
		a.Render()
		return got
	}
	cfg := app.Config{Font: goregular.TTF}
	want := geom.Size{W: 1280, H: 900}

	sizeFirst := probe(t, apptest.Size(1280, 900), apptest.WithConfig(cfg))
	if sizeFirst != want {
		t.Errorf("Size() before WithConfig: rendered at %v, want %v", sizeFirst, want)
	}
	sizeLast := probe(t, apptest.WithConfig(cfg), apptest.Size(1280, 900))
	if sizeLast != want {
		t.Errorf("Size() after WithConfig: rendered at %v, want %v", sizeLast, want)
	}

	// And WithConfig's own size still applies when Size() is absent, which is
	// the other half of the documented contract.
	viaConfig := probe(t, apptest.WithConfig(app.Config{
		Font: goregular.TTF, Size: geom.Size{W: 640, H: 480}}))
	if want := (geom.Size{W: 640, H: 480}); viaConfig != want {
		t.Errorf("size via WithConfig alone: rendered at %v, want %v", viaConfig, want)
	}
}
