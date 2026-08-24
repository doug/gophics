//go:build !js

package devmedia

import (
	"os"
	"testing"
	"time"

	"github.com/doug/gophics/shell"
)

// TestHardwareRecordAndPlay drives the real device end to end.
//
// Off by default: it needs a microphone and speakers, and on macOS the first
// run raises the permission prompt, which no unattended run can answer. Set
// GOPHICS_AUDIO_HW=1 to include it.
//
// It exercises both devices, because a recording that cannot be played back
// is only half a check.
//
// It exists because everything between the device and the Clip is FFI and
// pointer arithmetic, and the failure modes all produce a well-formed result:
// a clip of exactly the right length full of silence, a player that reports
// playing while the driver was never started.
func TestHardwareRecordAndPlay(t *testing.T) {
	if os.Getenv("GOPHICS_AUDIO_HW") == "" {
		t.Skip("set GOPHICS_AUDIO_HW=1 to run against real audio hardware")
	}
	mic := deviceMic{}
	spk := deviceSpeakers{}

	var rec shell.Recorder
	var err error
	mic.Record(shell.RecordOptions{}, func(r shell.Recorder, e error) { rec, err = r, e })
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	time.Sleep(time.Second)

	if el := rec.Elapsed(); el < 500*time.Millisecond {
		t.Errorf("elapsed = %v after a 1s sleep; the clock is not running", el)
	}

	var clip shell.Clip
	rec.Stop(func(c shell.Clip, e error) { clip, err = c, e })
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	pcm, rate, err := shell.DecodeWAV(clip.Data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	t.Logf("recorded %d samples at %d Hz (%v)", len(pcm), rate, clip.Duration)
	if len(pcm) < rate/2 {
		t.Fatalf("only %d samples for ~1s at %d Hz; the device delivered almost nothing", len(pcm), rate)
	}

	// A room is never digitally silent. An all-zero buffer means the sink was
	// never called or read the wrong memory — which a length check alone
	// cannot distinguish from a successful recording.
	var nonzero int
	for _, s := range pcm {
		if s != 0 {
			nonzero++
		}
	}
	if nonzero*100/len(pcm) < 10 {
		t.Errorf("only %d%% of samples are non-zero; the capture path delivered silence", nonzero*100/len(pcm))
	}

	var pb shell.Playback
	spk.Play(clip, func(p shell.Playback, e error) { pb, err = p, e })
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if pb.Duration() != clip.Duration {
		t.Errorf("playback duration = %v, want %v", pb.Duration(), clip.Duration)
	}
	time.Sleep(200 * time.Millisecond)
	if !pb.Playing() {
		t.Error("Playing() is false 200ms into a 1s clip")
	}
	if at := pb.Position(); at < 100*time.Millisecond {
		t.Errorf("position = %v after 200ms; the cursor is not advancing", at)
	}
	pb.Stop()
	if pb.Playing() {
		t.Error("still playing after Stop")
	}
	t.Logf("played back to %v of %v", pb.Position(), pb.Duration())
}
