package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// The platform locale has to reach the formatters, or intl stays a package
// nothing can use correctly on three of five targets.
//
// intl had the data and the formatters from the start; what it could not do was
// find out which locale to use. Auto() reads LC_ALL and LANG, which do not
// exist on Android, iOS or the web — so the fallback silently formatted as
// en-US on a device set to German, and looked like a successful read.
func TestPlatformLocaleReachesFormatting(t *testing.T) {
	var got string
	root := probe{fn: func(ctx widget.Ctx) {
		got = ctx.IntlLocale().Money("1234.5", "€")
	}}

	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 80, H: 40}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Wire, then rebuild: the tree mounted and built once before the capability
	// existed, which is exactly what shell.CapabilitiesChanged handles at
	// runtime. A test that calls WireCapabilities directly has to do the other
	// half itself.
	h.Owner().WireCapabilities(localeWindow{tag: "de-DE"})
	h.Owner().RebuildAll()
	h.Render()

	// German: comma decimal, dot grouping, symbol after the number.
	if want := "1.234,5\u00a0€"; got != want {
		t.Errorf("de-DE money = %q, want %q", got, want)
	}
}

// An unrecognised tag falls back rather than erroring: a UI that renders
// nothing because the device speaks a language the table lacks is worse than
// one that renders in the default.
func TestUnknownLocaleFallsBack(t *testing.T) {
	var got string
	root := probe{fn: func(ctx widget.Ctx) { got = ctx.IntlLocale().Number("1234") }}

	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 80, H: 40}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Owner().WireCapabilities(localeWindow{tag: "zz-ZZ"})
	h.Owner().RebuildAll()
	h.Render()

	if got == "" {
		t.Error("an unknown locale produced no formatting at all")
	}
}

type probe struct{ fn func(widget.Ctx) }

func (p probe) Build(ctx widget.Ctx) widget.Widget {
	p.fn(ctx)
	return widget.Sized{W: 10, H: 10}
}

type localeWindow struct {
	shell.Window
	tag string
}

func (w localeWindow) Locale() shell.Locale { return fakeLocale{tag: w.tag} }

type fakeLocale struct{ tag string }

func (f fakeLocale) Tag() string         { return f.tag }
func (fakeLocale) OnChange(func(string)) {}

// The framework's own strings have to follow the locale too.
//
// gophics ships a handful of user-visible words — the edit menu's Cut, Copy,
// Paste and Select All — baked into the widget layer, so an app cannot
// translate them however carefully it translates its own. A German user
// long-pressing a text field got an English menu from a framework that had a
// message catalog sitting unused.
func TestBuiltInStringsFollowTheLocale(t *testing.T) {
	for _, tc := range []struct{ tag, copy, selectAll string }{
		{"en-US", "Copy", "Select All"},
		{"de-DE", "Kopieren", "Alles auswählen"},
		{"fr-FR", "Copier", "Tout sélectionner"},
		// A language with no catalog falls through to English rather than to a
		// blank: an untranslated menu is usable, an empty one is not.
		{"is-IS", "Copy", "Select All"},
	} {
		var gotCopy, gotAll string
		root := probe{fn: func(ctx widget.Ctx) {
			gotCopy, gotAll = ctx.Message(widget.MsgCopy), ctx.Message(widget.MsgSelectAll)
		}}
		h, err := app.NewHeadless(root, app.Config{
			Size: geom.Size{W: 80, H: 40}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		h.Owner().WireCapabilities(localeWindow{tag: tc.tag})
		h.Owner().RebuildAll()
		h.Render()

		if gotCopy != tc.copy {
			t.Errorf("%s: Copy = %q, want %q", tc.tag, gotCopy, tc.copy)
		}
		if gotAll != tc.selectAll {
			t.Errorf("%s: Select All = %q, want %q", tc.tag, gotAll, tc.selectAll)
		}
	}
}
