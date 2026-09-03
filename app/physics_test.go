package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Config.ScrollPhysics reaches the widget tree, and the zero value resolves to
// the curve the constants had before platforms could differ.
func TestConfigScrollPhysicsReachesTheOwner(t *testing.T) {
	h, err := NewHeadless(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.core.Owner.ScrollPhysics.Resolved(); got.Model != shell.FlingExponential || got.Tau != 0.5 {
		t.Errorf("default physics resolved to %+v, want exponential τ=0.5", got)
	}

	h, err = NewHeadless(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100},
		ScrollPhysics: shell.AndroidScrollPhysics()}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := h.core.Owner.ScrollPhysics; got.Model != shell.FlingSpline {
		t.Errorf("Config.ScrollPhysics did not reach the owner: %+v", got)
	}
}

// A pinned Config wins over whatever a shell reports: the app that chose one
// identity everywhere must get it on every platform.
func TestPinnedPhysicsBeatsTheShell(t *testing.T) {
	core, err := newCore(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100},
		ScrollPhysics: shell.IOSScrollPhysics()})
	if err != nil {
		t.Fatal(err)
	}
	h := &shellHandler{core: core}
	h.wireWindow(physicsWindow{shell.AndroidScrollPhysics()})
	if got := core.Owner.ScrollPhysics.Model; got != shell.FlingExponential {
		t.Errorf("the shell's physics overrode a pinned Config: model %v", got)
	}

	core, _ = newCore(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100}})
	h = &shellHandler{core: core}
	h.wireWindow(physicsWindow{shell.AndroidScrollPhysics()})
	if got := core.Owner.ScrollPhysics.Model; got != shell.FlingSpline {
		t.Errorf("an unpinned app did not take the shell's physics: model %v", got)
	}
}

// physicsWindow is the smallest shell.Window that also provides physics.
type physicsWindow struct{ p shell.ScrollPhysics }

func (w physicsWindow) ScrollPhysics() shell.ScrollPhysics { return w.p }
func (physicsWindow) Invalidate()                          {}
func (physicsWindow) SetTitle(string)                      {}
func (physicsWindow) Close()                               {}
func (physicsWindow) DarkMode() bool                       { return false }
func (physicsWindow) OpenURL(string) error                 { return nil }
func (physicsWindow) ClipboardRead() (string, error)       { return "", nil }
func (physicsWindow) ClipboardWrite(string) error          { return nil }
