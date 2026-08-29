//go:build !js

package devmedia

import (
	"errors"
	"sync"
	"time"

	"github.com/doug/gophics/internal/audio"
	"github.com/doug/gophics/internal/wav"
	"github.com/doug/gophics/shell"
)

// Recording and playback over the local audio devices.
//
// This lives below the shells rather than inside one because nothing here
// concerns a window: it adapts internal/audio to the shell.Audio contract, and
// a terminal app wants that adapter exactly as much as a GUI app does. The
// shells are the thin part — each returns these and says whether it has them.
//
// Both halves already existed under internal/audio and were reachable only
// through the sound package, which is not a capability — so an app written
// against shell.Audio worked on Android, iOS and the web but went silent on
// the three desktops. This is the adapter that was missing, not new machinery:
// capture is the same zero-CGo FFI the microphone uses (AudioQueue, pa_simple,
// WASAPI) and output is the same driver set behind sound.
//
// Clips are WAV, encoded by wav.Encode, which is what the mobile and web
// backends produce too. A recording made on one platform therefore plays on
// any other, and the waveform beside it is computed by one shared function.

// Speakers returns audio output.
//
// Non-nil even on a machine with no output device: whether one exists cannot
// be known without opening it, so a missing device surfaces as an error from
// Play, where an app can report it.
func Speakers() shell.Speakers { return deviceSpeakers{} }

type deviceSpeakers struct{}

// chunkSamples bounds how often the audio thread allocates.
//

// --- playback ----------------------------------------------------------------

// One output context for the process, opened on first use.
//
// A context owns the device and the mixer thread, so opening one per Play
// would take a fresh device handle for every sound — audible as a click on
// macOS, and on Linux an outright failure once the server's client limit is
// reached. It is never closed: the driver's own callback is what stops when
// nothing is playing, and closing it would cut off a clip still in flight.
var (
	outOnce sync.Once
	outCtx  *audio.Context
	outErr  error
)

func outputContext() (*audio.Context, error) {
	outOnce.Do(func() { outCtx, outErr = audio.NewContext() })
	return outCtx, outErr
}

func (deviceSpeakers) Play(clip shell.Clip, done func(shell.Playback, error)) {
	if done == nil {
		return
	}
	if len(clip.Data) == 0 {
		done(nil, errors.New("audio: the clip is empty"))
		return
	}
	// Decoded up front rather than handed straight to the driver, because Seek
	// has to restart the player at an offset and the driver has no notion of
	// one. Decoding once here is also what makes a seek cheap.
	pcm, rate, err := wav.Decode(clip.Data)
	if err != nil {
		done(nil, err)
		return
	}
	if rate <= 0 || len(pcm) == 0 {
		done(nil, errors.New("audio: the clip has no samples"))
		return
	}
	ctx, err := outputContext()
	if err != nil {
		done(nil, err)
		return
	}
	p := &devicePlayback{
		ctx:      ctx,
		pcm:      pcm,
		rate:     rate,
		duration: time.Duration(len(pcm)) * time.Second / time.Duration(rate),
	}
	if err := p.playFrom(0); err != nil {
		done(nil, err)
		return
	}
	done(p, nil)
}

type devicePlayback struct {
	ctx      *audio.Context
	pcm      []int16
	rate     int
	duration time.Duration

	mu      sync.Mutex
	player  *audio.Player
	offset  time.Duration // where in the clip the current player started
	started time.Time
	stopped bool
}

// playFrom starts a player at off. The caller must not hold mu.
func (p *devicePlayback) playFrom(off time.Duration) error {
	if off < 0 {
		off = 0
	}
	if off > p.duration {
		off = p.duration
	}
	i := min(int(off*time.Duration(p.rate)/time.Second), len(p.pcm))
	pl, err := p.ctx.PlayWAV(wav.Encode(p.pcm[i:], p.rate))
	if err != nil {
		return err
	}
	p.mu.Lock()
	old := p.player
	p.player, p.offset, p.started, p.stopped = pl, off, time.Now(), false
	p.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	pl.Play()
	return nil
}

// Position is derived from the clock rather than read from the driver, which
// exposes no cursor. It is therefore the position of the audio handed to the
// device, not of the audio leaving the speaker — the two differ by the output
// buffer, a few milliseconds, which no waveform cursor can show.
func (p *devicePlayback) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped || p.player == nil {
		return p.offset
	}
	at := p.offset + time.Since(p.started)
	if at > p.duration {
		return p.duration
	}
	return at
}

func (p *devicePlayback) Duration() time.Duration { return p.duration }

func (p *devicePlayback) Playing() bool {
	p.mu.Lock()
	pl, stopped := p.player, p.stopped
	p.mu.Unlock()
	return !stopped && pl != nil && pl.IsPlaying()
}

func (p *devicePlayback) Seek(t time.Duration) {
	p.mu.Lock()
	stopped := p.stopped
	p.mu.Unlock()
	if stopped {
		// A seek on a stopped playback moves the cursor without resuming;
		// restarting audio the user explicitly stopped would be a surprise.
		p.mu.Lock()
		p.offset = t
		p.mu.Unlock()
		return
	}
	_ = p.playFrom(t)
}

func (p *devicePlayback) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.offset = min(p.offset+time.Since(p.started), p.duration)
	p.stopped = true
	pl := p.player
	p.player = nil
	p.mu.Unlock()
	if pl != nil {
		pl.Stop()
	}
}
