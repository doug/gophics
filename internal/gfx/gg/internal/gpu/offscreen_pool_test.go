package gpu

import "testing"

// The pool holds a bounded number of bytes, not a bounded number of textures.
//
// A count is the wrong shape when the sizes differ by orders of magnitude. The
// full-surface capture a backdrop blur takes is about 11MB at phone
// resolution; the third downsample of it is under 200KB. "Four of each" sounds
// modest and reserves 44MB of the large ones — permanently, on a device, where
// before the pool they were handed back every frame. That is not a trade worth
// making silently, and it is the kind of change that looks like a pure win
// until something is killed for memory.
func TestThePoolStopsAtItsByteBudget(t *testing.T) {
	var p offscreenPool
	// Textures the size of a phone's full surface: ~11MB each.
	const w, h = 1120, 2432
	if got, want := bytesFor(w, h), int64(1120*2432*4); got != want {
		t.Fatalf("bytesFor = %d, want %d", got, want)
	}

	kept := 0
	for range 8 {
		if p.put(w, h, offscreenTex{}) {
			kept++
		}
	}
	if kept == 0 {
		t.Fatal("the pool kept nothing at all; reuse cannot happen")
	}
	if p.bytes > poolByteBudget {
		t.Errorf("pool holds %d bytes, past its %d budget", p.bytes, poolByteBudget)
	}
	// At ~11MB each, a 24MB budget must refuse well before the per-size cap of
	// four would have — which is the whole point of counting bytes.
	if kept >= perSizeCap {
		t.Errorf("kept %d full-surface textures; the byte budget should bind first", kept)
	}
}

// Taking one back frees its bytes, or the budget would ratchet shut.
func TestTakingFromThePoolReleasesItsBytes(t *testing.T) {
	var p offscreenPool
	const w, h = 256, 256
	if !p.put(w, h, offscreenTex{}) {
		t.Fatal("a small texture should fit an empty pool")
	}
	held := p.bytes
	if held != bytesFor(w, h) {
		t.Fatalf("pool holds %d bytes after one put, want %d", held, bytesFor(w, h))
	}
	if _, ok := p.take(w, h); !ok {
		t.Fatal("what was just put should come back")
	}
	if p.bytes != 0 {
		t.Errorf("pool still accounts for %d bytes after handing its only texture out; "+
			"the budget would close over time", p.bytes)
	}
}

// Small textures still pool freely — the budget must not defeat the point.
func TestSmallTexturesStillPool(t *testing.T) {
	var p offscreenPool
	kept := 0
	for range perSizeCap + 2 {
		if p.put(140, 304, offscreenTex{}) {
			kept++
		}
	}
	if kept != perSizeCap {
		t.Errorf("kept %d small textures, want the per-size cap of %d", kept, perSizeCap)
	}
}
