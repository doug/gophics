package app_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// The runner insets the root by default, and an app that also wraps its own
// screen must not be padded twice.
//
// Both halves of this exist because of what a phone showed: four of the five
// examples here had never wrapped anything, so their titles sat under the
// Dynamic Island — which says the default was wrong. The fifth had wrapped its
// root exactly as the documentation advised, so making the default inset would
// have doubled its padding if nesting were not idempotent.
func TestRootIsInsetOnceEvenWhenTheAppWrapsItToo(t *testing.T) {
	const inset = 40
	body := widget.Interactive{
		Sem:   &layout.SemInfo{Role: layout.RoleButton, Label: "body"},
		Child: widget.Fill{Color: theme.Light().Primary},
	}

	measure := func(t *testing.T, root widget.Widget) float32 {
		t.Helper()
		a := apptest.New(t, root, apptest.WithConfig(app.Config{
			Size: geom.Size{W: 200, H: 400}, Font: goregular.TTF,
		}), apptest.WithSafeInsets(geom.Insets{Top: inset}))
		n := a.Role(layout.RoleButton)
		if n == nil {
			t.Fatalf("no body in the tree; labels=%v", a.Labels())
		}
		return n.Rect.Min.Y
	}

	bare := measure(t, body)
	if bare < inset {
		t.Errorf("an unwrapped root starts at y=%.0f, want it pushed to %d by the "+
			"platform inset — the runner is not applying a safe area", bare, inset)
	}

	wrapped := measure(t, widget.SafeArea{Child: body})
	if wrapped != bare {
		t.Errorf("wrapping the root in SafeArea moved it from y=%.0f to y=%.0f; "+
			"nesting must be idempotent or every app that followed the advice "+
			"gets padded twice", bare, wrapped)
	}
}

// EdgeToEdge is the opt-out for content meant to run under the hardware.
func TestEdgeToEdgeLeavesTheRootAlone(t *testing.T) {
	body := widget.Interactive{
		Sem:   &layout.SemInfo{Role: layout.RoleButton, Label: "body"},
		Child: widget.Fill{Color: theme.Light().Primary},
	}
	a := apptest.New(t, body, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 200, H: 400}, Font: goregular.TTF, EdgeToEdge: true,
	}), apptest.WithSafeInsets(geom.Insets{Top: 40}))

	n := a.Role(layout.RoleButton)
	if n == nil {
		t.Fatal("no body in the tree")
	}
	if y := n.Rect.Min.Y; y != 0 {
		t.Errorf("EdgeToEdge root starts at y=%.0f, want 0 — the opt-out did not apply", y)
	}
}
