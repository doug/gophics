//go:build !nogpu

package gpu

import "testing"

// A child context coming out of the pool must not carry frame state from the
// layer it rendered last time.
//
// This is the drag-ghost bug. A layer renders into an offscreen from
// offscreenPool, and the surface pass picks its LoadOp from frameRendered,
// resetting it only when the target view changes:
//
//	if view != s.lastView { s.frameRendered = false; s.lastView = view }
//
// That was sound while every layer got a freshly created texture — a new view
// each frame, so the reset always fired and the pass cleared. Once offscreen
// textures were recycled, a same-size layer gets *the same view* back, and a
// pooled child context still remembers it. The reset never fires,
// frameRendered is still true from last frame, and the pass loads instead of
// clearing — so the recycled texture's previous contents survive and the new
// content composites on top of them.
//
// For a drag preview, which is one constant-size layer redrawn at a new
// position every frame, that is a copy of the preview left at every position
// the pointer has passed through.
//
// A layer's backdrop is transparent by definition — that is what makes group
// opacity mean anything — so a child context must always begin its target
// cleared.
func TestPooledChildContextDoesNotInheritFrameState(t *testing.T) {
	s := &GPUShared{}
	c := s.acquireChildContext()
	if c == nil {
		t.Skip("no render context available")
	}

	// Stand in for "this context rendered a layer last frame".
	c.frameRendered = true
	s.releaseChildContext(c)

	got := s.acquireChildContext()
	if got != c {
		t.Skip("pool did not return the same context; nothing to assert")
	}
	if got.frameRendered {
		t.Error("a pooled child context still reports frameRendered — its next " +
			"layer pass will LoadOpLoad onto a recycled texture and keep the " +
			"previous frame's contents")
	}
	if got.lastView != nil {
		t.Error("a pooled child context still holds lastView — a recycled " +
			"offscreen will compare equal and skip the clear")
	}
}
