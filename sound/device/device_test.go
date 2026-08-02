package device

import (
	"os"
	"testing"
	"time"

	"github.com/doug/gossamer/sound"
)

// TestPlayTone opens the real audio device and plays a short tone.
// Run: SOUND_PLAY=1 go test -run TestPlayTone ./sound/device
func TestPlayTone(t *testing.T) {
	if os.Getenv("SOUND_PLAY") == "" {
		t.Skip("set SOUND_PLAY=1 to open the device and hear a tone")
	}
	m := sound.NewMixer(2)
	c, err := Open(m)
	if err != nil {
		t.Fatalf("open audio device: %v", err)
	}
	defer c.Close()
	m.Play(sound.Coin(), 0.6)
	m.Play(sound.Blip(440, 0.5), 0.5)
	time.Sleep(700 * time.Millisecond)
}
