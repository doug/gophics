package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// A service an app builds once and uses everywhere.
type demoAPI interface{ Name() string }

type httpDemoAPI struct{ name string }

func (a *httpDemoAPI) Name() string { return a.name }

// deepWidget reads the API from a widget several levels down, the way a page
// would, and records what it found.
type deepWidget struct{ got *string }

func (d deepWidget) Build(ctx widget.Ctx) widget.Widget {
	if api, ok := ctx.Of[demoAPI](); ok {
		*d.got = api.Name()
	} else {
		*d.got = "<missing>"
	}
	return widget.Sized{W: 10, H: 10}
}

func nestDeep(w widget.Widget) widget.Widget {
	for range 5 {
		w = widget.Padding{All: 1, Child: w}
	}
	return w
}

func runWith(t *testing.T, provide []any, root widget.Widget) {
	t.Helper()
	h, err := NewHeadless(root, Config{
		Size: geom.Size{W: 100, H: 100}, Provide: provide,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
}

// A value in Config.Provide reaches a widget anywhere in the app, with no
// Provide in the app's own tree.
//
// This is the case the whole feature is for: an API client is built once at
// startup and never varies by position, so requiring the tree to nest a
// Provide for it is ceremony that can be forgotten — and forgetting it is a
// panic at runtime, on whichever screen happens to need it.
func TestConfigProvideReachesTheWholeApp(t *testing.T) {
	var got string
	runWith(t, []any{&httpDemoAPI{name: "root"}}, nestDeep(deepWidget{got: &got}))
	if got != "root" {
		t.Errorf("deep widget saw %q, want the API from Config.Provide", got)
	}
}

// The key is the value's dynamic type, so a concrete answers the interface it
// satisfies — the same behaviour Provide[T] already has.
func TestConfigProvideIsFoundByInterface(t *testing.T) {
	var got string
	// Stored as a *httpDemoAPI; asked for as demoAPI.
	runWith(t, []any{&httpDemoAPI{name: "concrete"}}, deepWidget{got: &got})
	if got != "concrete" {
		t.Errorf("saw %q — a concrete value must answer the interface it implements", got)
	}
}

// A Provide inside the tree overrides one from Config for its own subtree.
//
// There is one lookup and one rule — nearest ancestor wins — because the root
// values are wrapped around the tree rather than consulted as a separate
// fallback. Overriding for a subtree needs no new concept.
func TestTreeProvideOverridesConfigProvide(t *testing.T) {
	var got string
	runWith(t, []any{&httpDemoAPI{name: "root"}},
		widget.Provide[demoAPI]{
			Value: &httpDemoAPI{name: "subtree"},
			Child: nestDeep(deepWidget{got: &got}),
		})
	if got != "subtree" {
		t.Errorf("saw %q, want the subtree's own Provide to win", got)
	}
}

// Later entries sit nearer the app, so a later one wins.
func TestLaterConfigProvideWins(t *testing.T) {
	var got string
	runWith(t, []any{&httpDemoAPI{name: "first"}, &httpDemoAPI{name: "second"}},
		deepWidget{got: &got})
	if got != "second" {
		t.Errorf("saw %q, want the later entry to win", got)
	}
}

// No Provide at all still builds; the widget simply does not find one.
func TestNoConfigProvideIsFine(t *testing.T) {
	var got string
	runWith(t, nil, deepWidget{got: &got})
	if got != "<missing>" {
		t.Errorf("saw %q with nothing provided", got)
	}
}
