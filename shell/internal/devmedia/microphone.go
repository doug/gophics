//go:build !js

package devmedia

import (
	"errors"
	"sync"
	"time"

	"github.com/doug/gophics/internal/audio"
	"github.com/doug/gophics/internal/dsp"
	"github.com/doug/gophics/shell"
)

// Live microphone capture over the same zero-CGo FFI the audio
// output drivers use: CoreAudio's AudioQueue on macOS, pa_simple on Linux,
// WASAPI on Windows (see internal/audio/capture_*.go). Everything above the
// device — the ring buffer, the level, the FFT bands — is the shared analyzer
// in internal/dsp, so a desktop monitor and an Android one answer identically.

// Microphone returns audio input: Listen for a live monitor, Record for a clip.
//
// Record lives in audio.go, beside the playback it shares its encoding with.
//
// It is non-nil even on a machine with no input device, because whether one
// exists cannot be known without opening it — and on macOS, opening it is what
// triggers the permission prompt. A missing or refused device surfaces as an
// error from Listen, which is where an app can actually report it.
func Microphone() shell.Microphone { return deviceMic{} }

type deviceMic struct{}

// Authorize is a no-op that reports granted.
//
// macOS can answer this — it is the same AVCaptureDevice query the camera
// uses, with the media type changed — so it is asked, in permission_darwin.go.
// Linux and Windows have no per-application microphone permission to query;
// both gate at the OS settings level and a refusal surfaces as a device that
// will not open, so there Granted means "try it and see" and Listen is what
// finds out.
//
// It still does not prompt. On macOS the prompt is raised by opening the
// device, so PermissionPrompt means asking will happen on Listen rather than
// now — reporting it separately is what lets an app distinguish a refusal from
// a silent room, which sound alone cannot.
func (deviceMic) Authorize(cb func(shell.Permission)) {
	if cb != nil {
		cb(micPermission())
	}
}

func (deviceMic) Listen(done func(shell.Monitor, error)) {
	if done == nil {
		return
	}
	cap := audio.DefaultCapture()
	rate, err := cap.Open(44100)
	if err != nil {
		done(nil, err)
		return
	}
	if rate <= 0 {
		done(nil, errors.New("audio: capture device reported no sample rate"))
		return
	}
	m := &deviceMonitor{cap: cap, an: dsp.New(rate, dsp.DefaultWindow)}
	// The sink runs on the platform's audio thread and does nothing but copy
	// into the mutex-guarded ring buffer, which is the whole reason the
	// analyzer is written to be safe there.
	if err := cap.Start(func(pcm []float32) { m.write(pcm) }); err != nil {
		cap.Close()
		done(nil, err)
		return
	}
	done(m, nil)
}

type deviceMonitor struct {
	cap audio.Capture
	an  *dsp.Analyzer

	mu      sync.Mutex
	stopped bool
}

func (m *deviceMonitor) write(pcm []float32) {
	if m.done() {
		return
	}
	m.an.Write(pcm)
}

func (m *deviceMonitor) done() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *deviceMonitor) Level() float32 {
	if m.done() {
		return 0
	}
	return m.an.Level()
}

func (m *deviceMonitor) Bands(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Bands(dst)
}

func (m *deviceMonitor) Samples(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Samples(dst)
}

func (m *deviceMonitor) WindowSize() int { return m.an.WindowSize() }
func (m *deviceMonitor) SampleRate() int { return m.an.SampleRate() }

func (m *deviceMonitor) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	m.mu.Unlock()

	// Closing releases the device, which is what turns off the recording
	// indicator macOS shows in the menu bar. Leaving it open after the app
	// stops listening is the kind of bug a user notices and does not forgive.
	m.cap.Close()
}

// --- Recording ---------------------------------------------------------------

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
