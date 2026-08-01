package shell

import "testing"

func TestWAVRoundTrip(t *testing.T) {
	pcm := make([]int16, 4000)
	for i := range pcm {
		// a couple of cycles of a triangle-ish signal spanning the range
		pcm[i] = int16((i%200)*300 - 30000)
	}
	wav := EncodeWAV(pcm, 48000)
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("bad RIFF/WAVE header")
	}
	if len(wav) != 44+len(pcm)*2 {
		t.Fatalf("length = %d, want %d", len(wav), 44+len(pcm)*2)
	}
	got, rate, err := DecodeWAV(wav)
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
	if _, rate, _ := DecodeWAV(EncodeWAV([]int16{0, 1, -1}, 0)); rate != 44100 {
		t.Fatalf("default sample rate = %d, want 44100", rate)
	}
	if _, _, err := DecodeWAV([]byte("not a wav")); err == nil {
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
	wav := EncodeWAV(pcm, 44100)
	wav[22] = 2 // channels = 2
	got, _, err := DecodeWAV(wav)
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
