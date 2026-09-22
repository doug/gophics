package widget

// Scoped returns w wrapped so that, wherever it is mounted, Of/MustOf lookups
// from inside it continue through ctx's ancestors once they leave w's own
// subtree. It is the mechanism behind Overlay.ShowFrom: an overlay entry is a
// sibling of the app, mounted under the OverlayHost's Stack, so without it a
// dialog cannot see the Nav, the theme, or any Provide of the page that opened
// it. Custom overlay-like hosts that mount content away from its opener wrap
// it in Scoped to give it the opener's scope.
//
// The bridge borrows scope; it does not move the element. w still lays out,
// paints and hit-tests wherever its host puts it, and its own subtree still
// shadows the origin's — a Provide inside w wins over one above ctx, just as
// it would in the tree.
//
// If the origin element unmounts while w is still shown (the page that opened
// a dialog was popped), lookups skip the dead part of the chain: the walk
// resumes at the origin's nearest ancestor that is still mounted — the
// Navigator, the app's root providers — so a dialog keeps its Nav and theme
// across the pop of its opener and only ever reads live providers. What it
// loses is any Provide that lived inside the unmounted subtree itself; a
// widget that depends on one of those and must survive its opener — a
// snackbar, a sheet — should capture it at show time, which is why the
// theme's Show* functions still pin the Theme they read in an explicit
// Provide.
//
// The bridge assumes w is placed against the window, not inside an already
// inset area: it resets the SafeArea marker so a SafeArea inside w pads
// again. A custom host that mounts scoped content inside its own inset
// region gets that padding twice.
func Scoped(ctx Ctx, w Widget) Widget {
	if ctx.el == nil || w == nil {
		return w
	}
	return scopeBridge{origin: ctx.el, child: w}
}

// scopeBridge is the element that redirects the Of walk. It is stateless and
// its element sits in the tree normally; only Ctx.Of treats it specially, via
// element.scopeParent.
type scopeBridge struct {
	origin *element
	child  Widget
}

func (b scopeBridge) Build(Ctx) Widget {
	// An overlay entry is placed against the whole window, not inside the
	// opener's inset content, so the origin's "already inside a SafeArea"
	// marker must not leak across the bridge: a SafeArea in a bottom sheet
	// would otherwise skip its padding and sit under the home indicator.
	return Provide[safeAreaApplied]{Value: safeAreaApplied{}, Child: b.child}
}

// scopeParent is the next element the Of walk visits above el: the parent,
// or, for a scope bridge, the origin — or, when the origin has unmounted,
// the origin's nearest ancestor that is still mounted (see Scoped). unmount
// leaves parent pointers intact, so the climb is well defined; every element
// it returns is mounted, so only live providers are read. With no mounted
// ancestor at all (the whole origin tree is gone) the bridge behaves as an
// ordinary element.
//
// The redirect cannot loop. A cycle needs the target to be a descendant of
// the bridge, and the origin and its ancestors are always elements that
// existed before the bridge was built; elements never move under a newly
// inserted ancestor (reconciliation keeps them in place or remounts them),
// and a remounted origin is unmounted, so the climb passes over it.
func (el *element) scopeParent() *element {
	if b, ok := el.widget.(scopeBridge); ok {
		for o := b.origin; o != nil; o = o.parent {
			if o.mounted {
				return o
			}
		}
	}
	return el.parent
}
