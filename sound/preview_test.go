package sound

import (
	"os"
	"testing"
	"time"

	"github.com/doug/gossamer/shell"
)

func clip16(v float32) int16 {
	if v > 1 {
		v = 1
	} else if v < -1 {
		v = -1
	}
	return int16(v * 32767)
}

// TestRenderPreviewWAV renders the dungeon music plus a few panned SFX to a mono
// WAV, so the generated audio can actually be heard.
// Run: SOUND_WAV=<path> go test -run TestRenderPreviewWAV ./sound
func TestRenderPreviewWAV(t *testing.T) {
	out := os.Getenv("SOUND_WAV")
	if out == "" {
		t.Skip("set SOUND_WAV=<path> to render a preview")
	}
	m := NewMixer()
	music := m.PlaySource(DungeonMusic(1), PlayOptions{Volume: 0.5, FadeIn: 2 * time.Second})

	events := []struct {
		at  float64
		s   *Sample
		pan float64
	}{
		{1.0, Coin(), -0.7}, {2.2, Hit(), 0.6}, {3.4, Coin(), 0.8},
		{4.6, Hit(), -0.5}, {6.0, Thud(), 0}, {7.5, Coin(), 0.3},
		{9.0, Hit(), -0.8}, {10.5, Blip(720, 0.14), 0.4},
	}

	const secs = 12.0
	total := int(secs * SampleRate)
	pcm := make([]int16, total)
	const block = 2048
	buf := make([]float32, block*2)
	ei, frame := 0, 0
	faded := false
	for frame < total {
		n := block
		if frame+n > total {
			n = total - frame
		}
		for ei < len(events) && float64(frame)/SampleRate >= events[ei].at {
			m.Play(events[ei].s, PlayOptions{Volume: 0.7, Pan: events[ei].pan})
			ei++
		}
		if float64(frame)/SampleRate >= secs-2 && !faded {
			music.FadeOut(2 * time.Second) // fade the bed out at the end
			faded = true
		}
		b := buf[:n*2]
		m.ReadFloat32s(b)
		for i := 0; i < n; i++ {
			pcm[frame+i] = clip16((b[2*i] + b[2*i+1]) * 0.5) // downmix to mono
		}
		frame += n
	}
	if err := os.WriteFile(out, shell.EncodeWAV(pcm, SampleRate), 0o644); err != nil {
		t.Fatal(err)
	}
}
