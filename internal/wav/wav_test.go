package wav

import "testing"

func TestWAVRoundTrip(t *testing.T) {
	pcm := make([]int16, 4000)
	for i := range pcm {
		// a couple of cycles of a triangle-ish signal spanning the range
		pcm[i] = int16((i%200)*300 - 30000)
	}
	wav := Encode(pcm, 48000)
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("bad RIFF/WAVE header")
	}
	if len(wav) != 44+len(pcm)*2 {
		t.Fatalf("length = %d, want %d", len(wav), 44+len(pcm)*2)
	}
	got, rate, err := Decode(wav)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 48000 {
		t.Fatalf("sample rate = %d, want 48000", rate)
	}
	if len(got) != len(pcm) {
		t.Fatalf("sample count = %d, want %d", len(got), len(pcm))
	}
	for i := range pcm {
		if got[i] != pcm[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], pcm[i])
		}
	}
}

func TestWAVDefaultRateAndErrors(t *testing.T) {
	if _, rate, _ := Decode(Encode([]int16{0, 1, -1}, 0)); rate != 44100 {
		t.Fatalf("default sample rate = %d, want 44100", rate)
	}
	if _, _, err := Decode([]byte("not a wav")); err == nil {
		t.Fatalf("expected error for non-WAV input")
	}
}

func TestWAVStereoDownmix(t *testing.T) {
	// Hand-build a tiny 2-channel WAV: L,R frames → mono average.
	frames := []struct{ l, r int16 }{{100, 300}, {-200, 0}, {1000, -1000}}
	pcm := make([]int16, 0, len(frames)*2)
	for _, f := range frames {
		pcm = append(pcm, f.l, f.r)
	}
	// Reuse the encoder's layout but stamp 2 channels into the fmt chunk.
	wav := Encode(pcm, 44100)
	wav[22] = 2 // channels = 2
	got, _, err := Decode(wav)
	if err != nil {
		t.Fatal(err)
	}
	want := []int16{200, -100, 0} // per-frame averages
	if len(got) != len(want) {
		t.Fatalf("mono frames = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("frame %d = %d, want %d", i, got[i], want[i])
		}
	}
}

// A chunk that declares more bytes than the file holds is truncated to what
// is there. The guard used to compare an int-converted size, which on a
// 32-bit target turns 2³¹ and above negative, passes the bound, and panics
// slicing past the end; the fuzz test only ever ran on 64-bit hosts. The
// comparison is now in 64 bits on every target.
func TestWAVHugeDeclaredChunkIsTruncated(t *testing.T) {
	wav := Encode([]int16{1, 2, 3}, 44100)
	wav[40], wav[41], wav[42], wav[43] = 0xFF, 0xFF, 0xFF, 0xFF // data chunk size
	got, _, err := Decode(wav)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("decoded %d samples, want the 3 actually present", len(got))
	}
}
