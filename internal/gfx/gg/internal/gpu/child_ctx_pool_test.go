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

// Close must take the pooled child contexts with it.
//
// The mobile shell closes the accelerator on every surface rebuild (rotation,
// background→foreground) and then hands SetDeviceProvider a new device. Close
// released the device and left childCtxPool alone, and SetDeviceProvider
// flushed the pool only when it still held a device that differed from the
// incoming one — which, after Close, it never did. The first opacity/blend
// layer after a rotation acquired a context whose session was bound to the
// released device and its destroyed pipelines.
func TestCloseEmptiesChildContextPool(t *testing.T) {
	s := &GPUShared{}
	c := s.acquireChildContext()
	if c == nil {
		t.Skip("no render context available")
	}
	s.releaseChildContext(c)
	if n := len(s.childCtxPool); n != 1 {
		t.Fatalf("pool holds %d contexts after release, want 1", n)
	}

	s.Close()
	if n := len(s.childCtxPool); n != 0 {
		t.Fatalf("pool holds %d contexts after Close; the next SetDeviceProvider "+
			"would hand out a context bound to the released device", n)
	}
	if got := s.acquireChildContext(); got == c {
		t.Error("acquire after Close returned the closed context")
	}
}

// SetDeviceProvider drops the pool on any device change, and that includes
// the change from "no device" — the case Close leaves behind. There is no
// device to hand a provider here, so this pins the flush it calls.
func TestDeviceChangeFlushEmptiesChildContextPool(t *testing.T) {
	s := &GPUShared{}
	c := s.acquireChildContext()
	if c == nil {
		t.Skip("no render context available")
	}
	s.releaseChildContext(c)

	s.mu.Lock()
	s.closeChildCtxPoolLocked()
	n := len(s.childCtxPool)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("pool holds %d contexts after the flush, want 0", n)
	}
	if c.session != nil {
		t.Error("the flushed context still holds its session")
	}
}
