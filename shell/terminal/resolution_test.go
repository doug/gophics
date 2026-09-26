package terminal

import (
	"io"
	"testing"
	"time"
)

// sizedTTY reports a fixed pixel size and nothing else.
type sizedTTY struct {
	io.Writer
	w, h int
}

func (sizedTTY) Read([]byte) (int, error) { return 0, io.EOF }
func (s sizedTTY) Size() (int, int)       { return s.w, s.h }
func (sizedTTY) Resize() <-chan struct{}  { return nil }

// The image cap holds under a content-scale override. GOPHICS_TERM_SCALE=1 on
// a 5000 px terminal wants a render scale of 0.4; the floor of 1 that suits
// the auto-derived scale clamped it up and rendered the full physical size
// the cap exists to avoid.
func TestRenderScaleHonoursCapUnderOverride(t *testing.T) {
	t.Setenv("GOPHICS_TERM_SCALE", "1")
	ts := &termState{out: io.Discard, imageID: 1}
	ts.applySize(sizedTTY{w: 5000, h: 3000})
	long := float32(5000) / ts.scale * ts.renderFull
	if long > float32(maxImageLong()) {
		t.Errorf("long edge renders at %.0f px under GOPHICS_TERM_SCALE=1, want <= %d", long, maxImageLong())
	}
	if ts.renderMotion > ts.renderFull {
		t.Errorf("motion scale %v above full %v", ts.renderMotion, ts.renderFull)
	}
}

// Without an override the auto-derived scale keeps its floor of 1.
func TestRenderScaleAutoFloor(t *testing.T) {
	t.Setenv("GOPHICS_TERM_SCALE", "")
	ts := &termState{out: io.Discard, imageID: 1}
	ts.applySize(sizedTTY{w: 800, h: 480})
	if ts.renderFull != 1 {
		t.Errorf("render scale %v on a small terminal, want 1", ts.renderFull)
	}
}

// Only frames the pointer caused count as motion. An app that invalidates on
// a timer paints at a steady rate with nothing moving under the pointer, and
// treating those frames as motion kept it at half resolution for good.
func TestMotionOnlyFromInput(t *testing.T) {
	var m motionTracker
	now := time.Now()
	for i := 0; i < 30; i++ {
		if m.frame(now.Add(time.Duration(i)*16*time.Millisecond), false) {
			t.Fatalf("timer-driven frame %d counted as motion", i)
		}
	}
	// A pointer drag: consecutive input-caused frames are motion after the first.
	if m.frame(now.Add(time.Second), true) {
		t.Fatal("the first input frame is not yet motion")
	}
	if !m.frame(now.Add(time.Second+16*time.Millisecond), true) {
		t.Fatal("a second input frame 16 ms later is motion")
	}
	// A timer frame in between does not extend the motion.
	if m.frame(now.Add(time.Second+32*time.Millisecond), false) {
		t.Fatal("timer frame during a drag counted as motion")
	}
	// Input after a pause starts over.
	if m.frame(now.Add(3*time.Second), true) {
		t.Fatal("input after a pause is a fresh start, not motion")
	}
}
