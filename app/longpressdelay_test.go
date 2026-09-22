package app

import (
	"testing"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

// A handler's own LongPressDelay decides when its long press fires; a handler
// without one keeps the platform time. The drag handle that motivated it lifts
// after a short hold without turning every context-menu press hair-trigger.
func TestLongPressDelayPerHandler(t *testing.T) {
	fired := map[string]bool{}
	row := func(name string, d time.Duration) widget.Widget {
		return widget.Interactive{Gestures: widget.Gestures{
			OnLongPress:    func() { fired[name] = true },
			LongPressDelay: d,
		}, Child: widget.Sized{W: 200, H: 40}}
	}
	h, err := NewHeadless(widget.Column(row("quick", 100*time.Millisecond), row("platform", 0)),
		Config{Size: geom.Size{W: 200, H: 80}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	hold := func(p geom.Pt, secs float64) {
		h.TouchPress(p)
		for elapsed := 0.0; elapsed < secs; elapsed += 1.0 / 60 {
			h.Step(1.0 / 60)
		}
		h.TouchRelease(p)
	}
	hold(geom.Pt{X: 100, Y: 20}, 0.2)
	if !fired["quick"] {
		t.Fatal("a 100 ms LongPressDelay did not fire within a 200 ms hold")
	}
	hold(geom.Pt{X: 100, Y: 60}, 0.2)
	if fired["platform"] {
		t.Fatal("a handler without LongPressDelay fired before the platform long-press time")
	}
	hold(geom.Pt{X: 100, Y: 60}, 1.0)
	if !fired["platform"] {
		t.Fatal("the platform long press never fired on a 1 s hold")
	}
}
