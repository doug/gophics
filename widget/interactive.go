package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Interactive makes its child respond to input via Gestures callbacks.
// It adds no visuals and takes its child's size.
type Interactive struct {
	Gestures Gestures
	// Sem overrides the semantics Interactive would otherwise infer from
	// Gestures. Set it whenever the control is not the plain button its
	// handlers imply — a checkbox, a switch, a slider, a tab. Declaring it
	// here rather than wrapping the control in Semantics keeps it as one
	// node: a checkbox nested inside a button is worse for a screen-reader
	// user than either alone, because it has to be stepped through twice.
	//
	// OnActivate defaults to Gestures.OnTap when left nil.
	Sem *layout.SemInfo
	// Autofocus takes keyboard focus when this widget mounts, even if
	// something else already holds it.
	//
	// A key-only widget (OnKey without OnText) already claims focus when
	// nothing has it, so a canvas that reads arrows works from the first
	// frame without this. A text-capturing widget never does: a focused
	// field raises the soft keyboard, and no platform focuses a field the
	// user has not tapped. So Autofocus is the only way a field starts
	// focused — the one field a screen opens into, an edit-in-place field
	// appearing on a double click, a dialog focusing its first input, a
	// "press / to search" box. Without it the field appears with no caret
	// and the keystrokes go to whatever was focused before it, which reads
	// as the field being broken.
	//
	// It fires once per mount, not on every rebuild, so a field that stays
	// on screen does not steal focus back from wherever the user moved it.
	Autofocus bool
	Child     Widget
}

func (iw Interactive) createBox(ctx Ctx) layout.Box { return &InteractiveBox{} }
func (iw Interactive) updateBox(ctx Ctx, b layout.Box) {
	ib := b.(*InteractiveBox)
	wasFocus := ib.Gestures.OnFocus // the handlers about to be overwritten
	ib.Gestures = iw.Gestures
	ib.sem = iw.Sem
	owner := ctx.el.owner
	if !ib.Gestures.focusable() {
		// A widget that stops being focusable while it holds focus lets go
		// of it, the way unmounting does. A TextField turning Disabled swaps
		// in empty Gestures; leaving KeyboardTarget on them kept the caret
		// drawing, reported Focused and Disabled together to a screen
		// reader, never hid the soft keyboard (the field's OnFocus(false)
		// is what hides it), and blocked key-only widgets from autofocusing
		// because the target was never nil again.
		if owner.KeyboardTarget == &ib.Gestures {
			owner.KeyboardTarget = nil
			if wasFocus != nil {
				wasFocus(false)
			}
		}
		return
	}

	// Both autofocus rules are one-shot at mount, and the shot is spent here
	// whether or not it is taken.
	//
	// Marking it only on success was a bug with a specific shape: the *second*
	// field on a screen finds focus already held by the first, declines, and
	// stays unspent — so the moment anything released focus, it grabbed it.
	// Releasing focus lasted one frame, and on a phone the keyboard came
	// straight back up because a field nobody had touched re-took it.
	firstUpdate := !ib.autofocused
	ib.autofocused = true

	// &ib.Gestures is stable across rebuilds — the struct is overwritten in
	// place above — so this compares identity, not contents.
	if owner.KeyboardTarget == &ib.Gestures {
		return // already ours
	}
	switch {
	case iw.Autofocus && firstUpdate:
		// Explicitly asked for: takes focus from whatever holds it.
	case owner.KeyboardTarget == nil && firstUpdate && ib.Gestures.OnText == nil:
		// "A focusable widget mounted while nothing has focus takes it" — for
		// key-only widgets (a canvas that reads arrows, a menu that reads
		// Escape), which is invisible and costs nothing. A text field is
		// excluded on purpose: a focused field raises the soft keyboard, so
		// the rule made every page containing a field open with the keyboard
		// covering half the screen, and the raise cancelled whatever touch
		// gesture was in progress. No platform focuses a field the user has
		// not tapped; a field that should start focused says so with
		// Autofocus.
	default:
		return
	}

	old := owner.KeyboardTarget
	owner.KeyboardTarget = &ib.Gestures
	if old != nil && old.OnFocus != nil {
		old.OnFocus(false)
	}
	if ib.Gestures.OnFocus != nil {
		ib.Gestures.OnFocus(true)
	}
}
func (iw Interactive) childWidgets() []Widget { return []Widget{iw.Child} }
func (iw Interactive) soleChild() Widget      { return iw.Child }
func (iw Interactive) attach(b layout.Box, kids []layout.Box) {
	b.(*InteractiveBox).Child = first(kids)
}

// InteractiveBox is the render object behind Interactive. The app runner
// dispatches pointer events to it through the sealed GestureTarget interface.
type InteractiveBox struct {
	Gestures Gestures
	Child    layout.Box
	sem      *layout.SemInfo
	size     geom.Size
	// autofocused latches Interactive.Autofocus to the first update after
	// mount, so a rebuild does not yank focus back every frame.
	autofocused bool
}

// GestureHandler implements GestureTarget: the app runner reaches this box's
// interaction callbacks through it.
func (b *InteractiveBox) GestureHandler() *Gestures { return &b.Gestures }

func (b *InteractiveBox) sealedGestureTarget() {}

func (b *InteractiveBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *InteractiveBox) Size() geom.Size { return b.size }

func (b *InteractiveBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

// Semantics uses the caller's declared description when there is one, and
// otherwise derives a role from the handlers: keyboard handlers make a text
// field, tap handlers a button whose activation runs OnTap.
func (b *InteractiveBox) Semantics() layout.SemInfo {
	if b.sem != nil {
		info := *b.sem
		if info.OnActivate == nil {
			info.OnActivate = b.Gestures.OnTap
		}
		return info
	}
	switch {
	case b.Gestures.OnText != nil:
		// Only text *entry* makes a text field. Taking key events does not:
		// scrollers, list navigation and game surfaces all consume keys, and
		// treating them as inputs put a full-screen "textfield" over the HN
		// feed on Android whose label was every headline concatenated — one
		// stop that reads the entire list and no way past it.
		// The child knows more than the wrapper: a text field reports its
		// value (bullets, for a password), placeholder as label, focus, and
		// whether it is disabled. Without this every field read as an
		// unlabeled, empty text field.
		if p, ok := b.Child.(interface{ Semantics() layout.SemInfo }); ok {
			if info := p.Semantics(); info.Role == layout.RoleTextField {
				return info
			}
		}
		return layout.SemInfo{Role: layout.RoleTextField}
	case b.Gestures.OnTap != nil:
		return layout.SemInfo{Role: layout.RoleButton, OnActivate: b.Gestures.OnTap}
	}
	return layout.SemInfo{}
}

func (b *InteractiveBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (b *InteractiveBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X < 0 || p.Y < 0 || p.X >= b.size.W || p.Y >= b.size.H {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func first(kids []layout.Box) layout.Box {
	if len(kids) > 0 {
		return kids[0]
	}
	return nil
}
