package widget_test

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Copy has to be reachable without a keyboard.
//
// It was not. Cut, Copy and Paste were bound to Cmd+X/C/V and to nothing else,
// which is complete on a desktop and leaves a phone — where there is no Command
// key and no hardware keyboard — able to select text and unable to copy it. The
// clipboard plumbing reached the system pasteboard; no gesture reached the
// clipboard.
//
// So the test drives touch only: no key events anywhere in it.
func TestLongPressCopiesWithoutAKeyboard(t *testing.T) {
	var value = "hello world"
	root := widget.OverlayHost{Child: widget.Center(widget.Sized{
		W: 240, H: 44,
		Child: widget.TextField{
			Value:    value,
			OnChange: func(v string) { value = v },
		},
	})}

	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 320, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	clip := h.Clipboard()
	h.Render()

	// Long-press in the middle of the field: selects a word and raises the menu.
	at := geom.Pt{X: 160, Y: 100}
	h.TouchPress(at)
	for range 60 { // past the long-press threshold
		h.Step(1.0 / 60)
	}
	h.TouchRelease(at)
	h.Render()

	if !hasLabel(h, "Copy") {
		t.Fatalf("no Copy in the edit menu after a long press; labels: %v", labels(h))
	}

	tapLabel(t, h, "Copy")
	h.Render()

	if clip.S == "" {
		t.Fatal("Copy did not reach the clipboard — on a phone this text cannot " +
			"be copied at all")
	}
	if !strings.Contains(value, clip.S) {
		t.Errorf("clipboard holds %q, which is not part of the field %q", clip.S, value)
	}
}

// A read-only selection offers Copy and nothing that would be a lie: no Cut and
// no Paste, because there is nothing to edit.
func TestReadOnlySelectionOffersOnlyCopy(t *testing.T) {
	root := widget.OverlayHost{Child: widget.Center(widget.Sized{
		W: 240, H: 60,
		Child: widget.SelectableText{S: "selectable prose here"},
	})}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 320, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Clipboard().S = "something pasteable" // so Paste would be offered if it applied
	h.Render()

	at := geom.Pt{X: 130, Y: 100}
	h.TouchPress(at)
	for range 60 {
		h.Step(1.0 / 60)
	}
	h.TouchRelease(at)
	h.Render()

	got := labels(h)
	if !contains(got, "Copy") {
		t.Fatalf("read-only selection offered no Copy; labels: %v", got)
	}
	for _, forbidden := range []string{"Cut", "Paste"} {
		if contains(got, forbidden) {
			t.Errorf("read-only selection offered %q, which cannot do anything", forbidden)
		}
	}
}

// helpers over the semantics tree, which is how the menu is observable without
// reaching into widget internals.

func labels(h *app.Headless) []string {
	var out []string
	for _, n := range h.Semantics() {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

func hasLabel(h *app.Headless, s string) bool { return contains(labels(h), s) }

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func tapLabel(t *testing.T, h *app.Headless, label string) {
	t.Helper()
	for _, n := range h.Semantics() {
		if n.Label == label {
			c := geom.Pt{X: (n.Rect.Min.X + n.Rect.Max.X) / 2, Y: (n.Rect.Min.Y + n.Rect.Max.Y) / 2}
			h.TouchPress(c)
			h.Step(1.0 / 60)
			h.TouchRelease(c)
			return
		}
	}
	t.Fatalf("no node labelled %q", label)
}
