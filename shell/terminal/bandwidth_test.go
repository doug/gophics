package terminal

import (
	"bytes"
	"testing"
)

// TestBandwidthPartialVsFull quantifies the partial-update win over inline
// (SSH-style) transfer: a small change on a large frame should cost a tiny
// fraction of a full-frame retransmit. This is the number that makes gophics
// usable over SSH.
func TestBandwidthPartialVsFull(t *testing.T) {
	const w, h = 960, 600
	base := solid(w, h, 15, 18, 28)
	next := clone(base)
	fillRect(next, 40, 40, 30, 20, 255, 255, 255) // e.g. a button highlight

	// Full-frame cost (forced), inline transfer.
	var fullBuf bytes.Buffer
	full := &termState{out: &fullBuf, imageID: 1, full: true}
	full.present(base)
	fullBuf.Reset()
	full.present(next)
	fullBytes := fullBuf.Len()

	// Partial cost, inline transfer.
	var partBuf bytes.Buffer
	part := &termState{out: &partBuf, imageID: 1}
	part.present(base)
	partBuf.Reset()
	part.present(next)
	partBytes := partBuf.Len()

	ratio := float64(partBytes) / float64(fullBytes)
	t.Logf("inline transfer for a 30×20 change on %dx%d: full=%d B, partial=%d B (%.1f%% of full)",
		w, h, fullBytes, partBytes, ratio*100)

	// The claim being tested is that a partial update costs bytes proportional
	// to the *damage*, not to the screen. So the check is that the same small
	// change costs about the same on a much larger frame, while a full
	// retransmit grows with the area.
	//
	// It used to assert partial/full < 5%, which measured the compressor as
	// much as the feature: Go 1.27's flate shrank the full frame 3.5× — far
	// more than the small patch, which has less redundancy to exploit — so the
	// ratio rose from 3.9% to 7.2% while both numbers *improved*. A ratio
	// against a moving baseline is not the invariant worth defending.
	const w2, h2 = 1920, 1200 // 4× the area
	base2 := solid(w2, h2, 15, 18, 28)
	next2 := clone(base2)
	fillRect(next2, 40, 40, 30, 20, 255, 255, 255) // the identical change

	var fullBuf2, partBuf2 bytes.Buffer
	full2 := &termState{out: &fullBuf2, imageID: 1, full: true}
	full2.present(base2)
	fullBuf2.Reset()
	full2.present(next2)

	part2 := &termState{out: &partBuf2, imageID: 1}
	part2.present(base2)
	partBuf2.Reset()
	part2.present(next2)

	t.Logf("same change on %dx%d: full=%d B, partial=%d B", w2, h2, fullBuf2.Len(), partBuf2.Len())

	// Quadrupling the area must not meaningfully change what the patch costs.
	if partBuf2.Len() > partBytes*2 {
		t.Errorf("partial cost grew from %d B to %d B when the frame got 4× bigger; "+
			"a partial update should scale with the damage, not the screen",
			partBytes, partBuf2.Len())
	}
	// And a full retransmit must, or there is nothing being saved.
	if fullBuf2.Len() <= fullBytes {
		t.Errorf("full retransmit did not grow with the frame (%d B → %d B); "+
			"the comparison is not measuring what it claims", fullBytes, fullBuf2.Len())
	}
	// The patch is still a small fraction, generously bounded so a better
	// compressor cannot fail it again.
	if r := float64(partBuf2.Len()) / float64(fullBuf2.Len()); r > 0.25 {
		t.Errorf("partial update is %.1f%% of a full frame on the larger canvas", r*100)
	}
}
