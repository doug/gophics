package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// tapTarget is a full-window Interactive that records taps and long presses.
type tapTarget struct {
	taps, longs *int
}

func (t tapTarget) Build(ctx widget.Ctx) widget.Widget {
	return widget.Interactive{
		Gestures: widget.Gestures{
			OnTap:       func() { *t.taps++ },
			OnLongPress: func() { *t.longs++ },
		},
		Child: widget.Sized{W: 200, H: 200},
	}
}

func mountTaps(t *testing.T, g shell.GestureTuning) (*Headless, *int, *int) {
	t.Helper()
	taps, longs := 0, 0
	h, err := NewHeadless(tapTarget{&taps, &longs}, Config{Size: geom.Size{W: 200, H: 200}, Gestures: g}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, &taps, &longs
}

// The platform's long-press duration is the platform's: a 0.45s hold is a long
// press on Android (400ms) and not yet one on iOS (500ms). Measured values,
// not the same constant with two names.
func TestLongPressDurationFollowsPlatform(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    shell.GestureTuning
		want int
	}{
		{"android", shell.AndroidGestureTuning(), 1},
		{"ios", shell.IOSGestureTuning(), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _, longs := mountTaps(t, tc.g)
			h.TouchPress(geom.Pt{X: 100, Y: 100})
			for i := 0; i < 27; i++ { // 0.45s at 60Hz
				h.Step(1.0 / 60)
			}
			if *longs != tc.want {
				t.Errorf("after a 0.45s hold on %s: %d long presses, want %d", tc.name, *longs, tc.want)
			}
		})
	}
}

// Touch slop is the platform's too: a finger that rolls 9px through a tap is
// still tapping on iOS (10pt allowable movement) and has started dragging on
// Android (8dp).
func TestTouchSlopFollowsPlatform(t *testing.T) {
	for _, tc := range []struct {
		name string
		g    shell.GestureTuning
		want int
	}{
		{"ios", shell.IOSGestureTuning(), 1},
		{"android", shell.AndroidGestureTuning(), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, taps, _ := mountTaps(t, tc.g)
			h.TouchPress(geom.Pt{X: 100, Y: 100})
			h.Step(1.0 / 60)
			h.TouchMove(geom.Pt{X: 100, Y: 109})
			h.Step(1.0 / 60)
			h.TouchRelease(geom.Pt{X: 100, Y: 109})
			h.Step(shell.GestureTuning{}.Resolved().DoubleTap + 0.05) // let a deferred tap fire
			if *taps != tc.want {
				t.Errorf("a 9px roll on %s: %d taps, want %d", tc.name, *taps, tc.want)
			}
		})
	}
}

// A pinned Config beats the shell, and an unpinned one takes it.
func TestPinnedGesturesBeatTheShell(t *testing.T) {
	core, _ := newCore(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100}, Gestures: shell.IOSGestureTuning()})
	h := &shellHandler{core: core}
	h.wireWindow(gestureWindow{shell.AndroidGestureTuning()})
	if core.Owner.Gestures.LongPress != 0.5 {
		t.Errorf("shell overrode a pinned Config: %+v", core.Owner.Gestures)
	}
	core, _ = newCore(widget.Sized{W: 10, H: 10}, Config{Size: geom.Size{W: 100, H: 100}})
	h = &shellHandler{core: core}
	h.wireWindow(gestureWindow{shell.AndroidGestureTuning()})
	if core.Owner.Gestures.LongPress != 0.4 {
		t.Errorf("an unpinned app did not take the shell's tuning: %+v", core.Owner.Gestures)
	}
}

type gestureWindow struct{ g shell.GestureTuning }

func (w gestureWindow) GestureTuning() shell.GestureTuning { return w.g }
func (gestureWindow) Invalidate()                          {}
func (gestureWindow) SetTitle(string)                      {}
func (gestureWindow) Close()                               {}
func (gestureWindow) DarkMode() bool                       { return false }
func (gestureWindow) OpenURL(string) error                 { return nil }
func (gestureWindow) ClipboardRead() (string, error)       { return "", nil }
func (gestureWindow) ClipboardWrite(string) error          { return nil }
