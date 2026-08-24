//go:build !js

package devmedia

import (
	"errors"
	"sync"
	"time"

	"github.com/doug/gophics/internal/audio"
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
// Clips are WAV, encoded by shell.EncodeWAV, which is what the mobile and web
// backends produce too. A recording made on one platform therefore plays on
// any other, and the waveform beside it is computed by one shared function.

// Speakers returns audio output.
//
// Non-nil even on a machine with no output device: whether one exists cannot
// be known without opening it, so a missing device surfaces as an error from
// Play, where an app can report it.
func Speakers() shell.Speakers { return deviceSpeakers{} }

type deviceSpeakers struct{}

// --- recording ---------------------------------------------------------------

const recordRate = 44100

func (deviceMic) Record(_ shell.RecordOptions, done func(shell.Recorder, error)) {
	if done == nil {
		return
	}
	cap := audio.DefaultCapture()
	rate, err := cap.Open(recordRate)
	if err != nil {
		done(nil, err)
		return
	}
	if rate <= 0 {
		cap.Close()
		done(nil, errors.New("audio: capture device reported no sample rate"))
		return
	}
	r := &deviceRecorder{cap: cap, rate: rate, start: time.Now()}
	if err := cap.Start(r.write); err != nil {
		cap.Close()
		done(nil, err)
		return
	}
	done(r, nil)
}

// chunkSamples bounds how often the audio thread allocates.
//
// The sink is called on the platform's audio thread, where a growing append
// would eventually copy the whole recording — hundreds of milliseconds of
// memcpy inside a callback with a hard deadline, producing a dropout precisely
// in the long recordings that can least afford one. Fixed chunks allocate once
// per chunk and never copy what is already captured.
const chunkSamples = 8192

type deviceRecorder struct {
	cap   audio.Capture
	rate  int
	start time.Time

	mu       sync.Mutex
	chunks   [][]int16
	cur      []int16
	level    float32
	stopped  bool
	finished bool
}

// write runs on the audio thread: copy, measure, return.
func (r *deviceRecorder) write(pcm []float32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return
	}
	var peak float32
	for _, s := range pcm {
		if s > peak {
			peak = s
		} else if -s > peak {
			peak = -s
		}
		if r.cur == nil {
			r.cur = make([]int16, 0, chunkSamples)
		}
		v := s * 32767
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		r.cur = append(r.cur, int16(v))
		if len(r.cur) == chunkSamples {
			r.chunks = append(r.chunks, r.cur)
			r.cur = nil
		}
	}
	// Decay rather than track instantaneously, so a level meter reads as a
	// meter rather than flickering at the block rate.
	if peak > r.level {
		r.level = peak
	} else {
		r.level += (peak - r.level) * 0.3
	}
}

func (r *deviceRecorder) Level() float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.level
}

func (r *deviceRecorder) Elapsed() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopped {
		return 0
	}
	return time.Since(r.start)
}

func (r *deviceRecorder) Stop(done func(shell.Clip, error)) {
	pcm, rate, ok := r.finish()
	if done == nil {
		return
	}
	if !ok {
		done(shell.Clip{}, errors.New("audio: recording already finished"))
		return
	}
	if len(pcm) == 0 {
		done(shell.Clip{}, errors.New("audio: nothing was recorded"))
		return
	}
	done(shell.Clip{
		Data:     shell.EncodeWAV(pcm, rate),
		Mime:     "audio/wav",
		Duration: time.Duration(len(pcm)) * time.Second / time.Duration(rate),
		Envelope: shell.Envelope(pcm, 120),
	}, nil)
}

func (r *deviceRecorder) Cancel() { r.finish() }

// finish releases the device and returns what was captured. It reports false
// on a second call, so Stop-then-Cancel cannot double-close the device.
func (r *deviceRecorder) finish() ([]int16, int, bool) {
	r.mu.Lock()
	if r.finished {
		r.mu.Unlock()
		return nil, 0, false
	}
	r.finished = true
	r.stopped = true
	chunks, cur := r.chunks, r.cur
	r.chunks, r.cur = nil, nil
	r.mu.Unlock()

	// Releasing the device is what clears the recording indicator macOS shows
	// in the menu bar; holding it after the app stopped is a bug users notice.
	r.cap.Close()

	n := len(cur)
	for _, c := range chunks {
		n += len(c)
	}
	pcm := make([]int16, 0, n)
	for _, c := range chunks {
		pcm = append(pcm, c...)
	}
	return append(pcm, cur...), r.rate, true
}

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
	pcm, rate, err := shell.DecodeWAV(clip.Data)
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
	i := int(off * time.Duration(p.rate) / time.Second)
	if i > len(p.pcm) {
		i = len(p.pcm)
	}
	pl, err := p.ctx.PlayWAV(shell.EncodeWAV(p.pcm[i:], p.rate))
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
	p.offset = p.offset + time.Since(p.started)
	if p.offset > p.duration {
		p.offset = p.duration
	}
	p.stopped = true
	pl := p.player
	p.player = nil
	p.mu.Unlock()
	if pl != nil {
		pl.Stop()
	}
}
