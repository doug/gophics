package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// scopeCtxProbe captures the Ctx it is built with, so a test can open an
// overlay entry from a real position in the tree.
type scopeCtxProbe struct{ got *Ctx }

func (p scopeCtxProbe) Build(ctx Ctx) Widget {
	*p.got = ctx
	return Sized{W: 10, H: 10}
}

// scopeReader records what Of[string] resolves to where it is built.
type scopeReader struct {
	got *string
	ok  *bool
}

func (r scopeReader) Build(ctx Ctx) Widget {
	v, ok := ctx.Of[string]()
	*r.got, *r.ok = v, ok
	return Sized{W: 1, H: 1}
}

// An entry shown with ShowFrom resolves a Provide that sits below the root,
// above its opener; the same entry shown with Show does not. This is the bug
// that made ctx.MustOf panic inside every dialog an app opened.
func TestShowFromSeesProvideAboveOpener(t *testing.T) {
	o := newOwner()
	var opener Ctx
	o.SetRoot(OverlayHost{Child: Provide[string]{Value: "from the page", Child: scopeCtxProbe{&opener}}})
	o.FlushBuilds()
	ov := opener.MustOf[Overlay]()

	var got string
	var ok bool
	tok := ov.ShowFrom(opener, scopeReader{&got, &ok})
	o.FlushBuilds()
	if !ok || got != "from the page" {
		t.Fatalf("scoped entry read (%q, %v), want the opener's Provide", got, ok)
	}

	// Update swaps the content; the scope belongs to the entry and stays.
	got, ok = "", false
	tok.Update(scopeReader{&got, &ok})
	o.FlushBuilds()
	if !ok || got != "from the page" {
		t.Fatalf("after Update: read (%q, %v), want the scope kept", got, ok)
	}

	var plain string
	var plainOK bool
	ov.Show(scopeReader{&plain, &plainOK})
	o.FlushBuilds()
	if plainOK {
		t.Fatalf("unscoped Show read %q; it must see only what is above the host", plain)
	}
}

// scopeFn runs fn with the Ctx it is built with.
type scopeFn struct{ fn func(Ctx) }

func (w scopeFn) Build(ctx Ctx) Widget {
	w.fn(ctx)
	return Sized{W: 1, H: 1}
}

// scopeToggleHost mounts (or not) a provided opener under an OverlayHost so a
// test can unmount the origin of a live overlay entry.
type scopeToggleHost struct{ opener *Ctx }

func (h scopeToggleHost) CreateState() State { return &scopeToggleState{show: true} }

type scopeToggleState struct {
	StateBase[scopeToggleHost]
	show bool
}

func (s *scopeToggleState) Build(Ctx) Widget {
	if !s.show {
		return Sized{W: 10, H: 10}
	}
	return Provide[string]{Value: "while mounted", Child: scopeCtxProbe{s.W().opener}}
}

// Once the opener unmounts, an entry's walk resumes at the opener's nearest
// mounted ancestor: it no longer sees a Provide that lived inside the
// unmounted subtree but still reaches the host's Overlay, and nothing reads
// a provider from the dead subtree.
func TestShowFromFallsBackWhenOpenerUnmounts(t *testing.T) {
	o := newOwner()
	var opener Ctx
	o.SetRoot(OverlayHost{Child: scopeToggleHost{opener: &opener}})
	o.FlushBuilds()
	ov := opener.MustOf[Overlay]()

	var got string
	var ok bool
	var sawOverlay bool
	tok := ov.ShowFrom(opener, scopeReader{&got, &ok})
	o.FlushBuilds()
	if !ok {
		t.Fatal("setup: scoped entry did not see the opener's Provide")
	}

	digState[scopeToggleHost](o.root).(*scopeToggleState).SetState(func() {
		digState[scopeToggleHost](o.root).(*scopeToggleState).show = false
	})
	o.FlushBuilds()
	if opener.el.mounted {
		t.Fatal("setup: opener still mounted after the toggle")
	}

	tok.Update(scopeFn{func(ctx Ctx) {
		got, ok = ctx.Of[string]()
		_, sawOverlay = ctx.Of[Overlay]()
	}})
	o.FlushBuilds()
	if ok {
		t.Fatalf("entry still reads %q through an unmounted opener", got)
	}
	if !sawOverlay {
		t.Fatal("entry lost the host's Overlay after falling back to the unscoped walk")
	}
}

// A dialog opened from a page keeps the page's scope across the pop of that
// page: its next rebuild still resolves the Nav and a Provide above the
// Navigator, because the walk resumes at the opener's first mounted ancestor
// rather than dropping to the bare host chain — where MustOf[Nav] would
// panic in a confirm dialog whose page was popped underneath it.
func TestShowFromKeepsNavAndProvidersAfterOpenerPopped(t *testing.T) {
	o := newOwner()
	var home, page Ctx
	o.SetRoot(OverlayHost{Child: Provide[string]{Value: "app theme", Child: Navigator{Home: scopeCtxProbe{&home}}}})
	o.FlushBuilds()
	s := digState[Navigator](o.root).(*navState)

	home.MustOf[Nav]().Push(scopeCtxProbe{&page})
	pumpNav(t, o, s)

	var got string
	var ok, sawNav bool
	read := scopeFn{func(ctx Ctx) {
		got, ok = ctx.Of[string]()
		_, sawNav = ctx.Of[Nav]()
	}}
	tok := page.MustOf[Overlay]().ShowFrom(page, read)
	o.FlushBuilds()
	if !ok || !sawNav {
		t.Fatalf("setup: dialog read (%q, %v, nav=%v) from its opener", got, ok, sawNav)
	}

	page.MustOf[Nav]().Pop()
	pumpNav(t, o, s)
	if page.el.mounted {
		t.Fatal("setup: popped page still mounted")
	}

	got, ok, sawNav = "", false, false
	tok.Update(read)
	o.FlushBuilds()
	if !sawNav {
		t.Fatal("dialog lost the Nav once its opener was popped")
	}
	if !ok || got != "app theme" {
		t.Fatalf("dialog read (%q, %v); the Provide above the Navigator must survive the pop", got, ok)
	}
}

// navPopper is dialog content that pops the navigator of the page that
// opened it — the "Cancel" button of a confirm dialog on a detail page.
type navPopper struct{ nav *Nav }

func (p navPopper) Build(ctx Ctx) Widget {
	*p.nav = ctx.MustOf[Nav]()
	return Sized{W: 1, H: 1}
}

func TestOverlayFromNavigatorPageCanPop(t *testing.T) {
	o := newOwner()
	var home, page Ctx
	o.SetRoot(OverlayHost{Child: Navigator{Home: scopeCtxProbe{&home}}})
	o.FlushBuilds()
	s := digState[Navigator](o.root).(*navState)

	home.MustOf[Nav]().Push(scopeCtxProbe{&page})
	pumpNav(t, o, s)
	if d := (Nav{s: s}).Depth(); d != 2 {
		t.Fatalf("setup: depth = %d, want 2", d)
	}

	var nav Nav
	tok := page.MustOf[Overlay]().ShowFrom(page, navPopper{&nav})
	o.FlushBuilds()
	if nav.s == nil {
		t.Fatal("dialog content could not resolve Nav from the page that opened it")
	}
	tok.Dismiss()
	nav.Pop()
	pumpNav(t, o, s)
	if d := (Nav{s: s}).Depth(); d != 1 {
		t.Fatalf("depth after popping from the dialog = %d, want 1", d)
	}
}

// customHost mounts a reader beside its opener rather than under it, the way
// a bespoke overlay-like host would, and wraps it with Scoped on the second
// build once it has an origin to lend.
type customHost struct {
	got *string
	ok  *bool
}

func (h customHost) CreateState() State { return &customHostState{} }

type customHostState struct {
	StateBase[customHost]
	origin Ctx
}

func (s *customHostState) Build(Ctx) Widget {
	kids := []Widget{Provide[string]{Value: "sibling's scope", Child: scopeCtxProbe{&s.origin}}}
	if s.origin.el != nil {
		kids = append(kids, Scoped(s.origin, scopeReader{s.W().got, s.W().ok}))
	}
	return Stack{Children: kids}
}

func TestScopedLendsASiblingScope(t *testing.T) {
	o := newOwner()
	var got string
	var ok bool
	o.SetRoot(customHost{&got, &ok})
	o.FlushBuilds()
	st := digState[customHost](o.root).(*customHostState)
	st.SetState(nil)
	o.FlushBuilds()
	if !ok || got != "sibling's scope" {
		t.Fatalf("Scoped sibling read (%q, %v), want the origin's Provide", got, ok)
	}
}

// A SafeArea inside a scoped entry pads even though its opener sits inside
// the app's SafeArea: the entry is placed against the whole window, so the
// opener's "already inset" marker must not carry across the bridge.
func TestScopedEntrySafeAreaPadsAgain(t *testing.T) {
	o := newOwner()
	o.SafeInsets = geom.Insets{Top: 59, Bottom: 34}
	var opener Ctx
	o.SetRoot(OverlayHost{Child: SafeArea{Child: scopeCtxProbe{&opener}}})
	o.FlushBuilds()

	var inner Ctx
	opener.MustOf[Overlay]().ShowFrom(opener, SafeArea{Child: scopeCtxProbe{&inner}})
	o.FlushBuilds()

	// The nearest marker above the inner probe is the one its own SafeArea
	// provided (inset), not the bridge's reset — proof the inner SafeArea did
	// not pass through.
	a, ok := inner.Of[safeAreaApplied]()
	if !ok || !a.inset {
		t.Fatalf("inner SafeArea passed through: marker = (%+v, %v)", a, ok)
	}
	// And a nested SafeArea without one of its own in between sees the reset.
	var bare Ctx
	opener.MustOf[Overlay]().ShowFrom(opener, scopeCtxProbe{&bare})
	o.FlushBuilds()
	if a, ok := bare.Of[safeAreaApplied](); !ok || a.inset {
		t.Fatalf("scoped entry inherited the opener's inset marker: (%+v, %v)", a, ok)
	}
}
