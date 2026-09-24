package app

import (
	"image"
	"runtime"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Post is called from arbitrary goroutines — a NetworkImage finishing, the
// dev-mode signal goroutine — and reaches the window through the
// RequestFrame hook, while Frame records the window on the UI goroutine.
// Under -race this reported the hook's read of the window against Frame's
// write of it. There is nothing to assert beyond the detector staying quiet;
// fakeA11yWindow's Invalidate is a no-op, so the only shared state is the
// handler's.
func TestPostFromGoroutineDoesNotRaceFrame(t *testing.T) {
	h, err := NewHandler(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	sh := h.(*shellHandler)
	w := fakeA11yWindow{}
	f := &fakeFrame{size: geom.Size{W: 100, H: 100}, scale: 1,
		tgt: shell.PixelTarget{Put: func(*image.RGBA, geom.Rect) {}}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			sh.core.Owner.Post(func() {})
			runtime.Gosched()
		}
	}()
	for i := 0; i < 200; i++ {
		sh.Frame(w, f, 1.0/60)
		runtime.Gosched()
	}
	<-done
}
