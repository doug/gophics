package terminal

import (
	"bytes"
	"testing"
)

// TestBandwidthPartialVsFull quantifies the partial-update win over inline
// (SSH-style) transfer: a small change on a large frame should cost a tiny
// fraction of a full-frame retransmit. This is the number that makes gossamer
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
	if ratio > 0.05 {
		t.Errorf("partial update is %.1f%% of a full frame; expected <5%% for a small change", ratio*100)
	}
}
