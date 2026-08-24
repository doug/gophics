package shell

import "testing"

// DecodeWAV parses bytes the program did not produce: a clip arrives from a
// file picker, a download, or a mobile host handing back whatever the platform
// recorder wrote. Its chunk sizes are read from the input and then used as
// slice bounds, which is the shape that panics.
//
// The contract being fuzzed is narrow and total: for any input, DecodeWAV
// returns or errors, and never panics. Nothing is asserted about the samples —
// a malformed file has no correct answer, only a safe one.
func FuzzDecodeWAV(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("RIFF"))
	f.Add([]byte("RIFF\x00\x00\x00\x00WAVE"))
	// A minimal well-formed 16-bit mono file, so the fuzzer starts from
	// something that reaches the sample loop rather than only the guards.
	f.Add(EncodeWAV([]int16{0, 1, -1, 32767, -32768}, 44100))
	// A header claiming far more data than it carries, which is the classic
	// way a length-prefixed parser walks off the end.
	bad := EncodeWAV([]int16{1, 2, 3}, 8000)
	if len(bad) > 44 {
		bad[40], bad[41], bad[42], bad[43] = 0xff, 0xff, 0xff, 0x7f
		f.Add(bad)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		pcm, rate, err := DecodeWAV(b)
		if err != nil {
			return
		}
		// A successful decode has to be self-consistent, or callers that trust
		// it — every backend's Play path — compute the wrong duration.
		if rate <= 0 {
			t.Fatalf("decoded %d samples at a non-positive rate %d", len(pcm), rate)
		}
	})
}

// FuzzWAVRoundTrip checks the pair rather than the parser: whatever EncodeWAV
// writes, DecodeWAV must read back. The two are used across every backend and
// a clip recorded on one platform is played on another, so a disagreement
// between them is a portability bug rather than a local one.
func FuzzWAVRoundTrip(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0}, 44100)
	f.Add([]byte{}, 8000)

	f.Fuzz(func(t *testing.T, raw []byte, rate int) {
		if rate <= 0 || rate > 1<<20 {
			t.Skip()
		}
		pcm := make([]int16, len(raw)/2)
		for i := range pcm {
			pcm[i] = int16(raw[i*2]) | int16(raw[i*2+1])<<8
		}
		got, gotRate, err := DecodeWAV(EncodeWAV(pcm, rate))
		if err != nil {
			t.Fatalf("a clip we encoded does not decode: %v", err)
		}
		if gotRate != rate {
			t.Errorf("rate %d survived encoding as %d", rate, gotRate)
		}
		if len(got) != len(pcm) {
			t.Fatalf("encoded %d samples, decoded %d", len(pcm), len(got))
		}
		for i := range pcm {
			if got[i] != pcm[i] {
				t.Fatalf("sample %d changed: %d → %d", i, pcm[i], got[i])
			}
		}
	})
}
