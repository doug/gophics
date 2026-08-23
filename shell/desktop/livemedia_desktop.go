//go:build !js

package desktop

import (
	"errors"
	"sync"

	"github.com/doug/gophics/internal/audio"
	"github.com/doug/gophics/internal/mic"
	"github.com/doug/gophics/shell"
)

// The window opts into live capture by implementing shell.LiveMediaWindow;
// this is the compile-time check that it still does.
var _ shell.LiveMediaWindow = (*window)(nil)

// Live microphone capture on the desktop, over the same zero-CGo FFI the audio
// output drivers use: CoreAudio's AudioQueue on macOS, pa_simple on Linux,
// WASAPI on Windows (see internal/audio/capture_*.go). Everything above the
// device — the ring buffer, the level, the FFT bands — is the shared analyzer
// in internal/mic, so a desktop monitor and an Android one answer identically.

// CameraPreview reports that live camera preview is unavailable on desktop.
//
// shell.LiveMediaWindow pairs the preview with the microphone, but they are
// independent capabilities and a platform may have one without the other.
// There is no desktop camera path yet; nil is the contract's way of saying so,
// and an app hides the affordance rather than failing.
func (w *window) CameraPreview() shell.CameraPreview { return nil }

// Microphone returns live input monitoring.
//
// It is non-nil even on a machine with no input device, because whether one
// exists cannot be known without opening it — and on macOS, opening it is what
// triggers the permission prompt. A missing or refused device surfaces as an
// error from Listen, which is where an app can actually report it.
func (w *window) Microphone() shell.Microphone { return desktopMic{} }

type desktopMic struct{}

// Authorize is a no-op that reports granted.
//
// None of the three desktop platforms has a permission API that can be asked
// ahead of time the way getUserMedia or Android's runtime permissions can be.
// macOS prompts on first capture (and needs NSMicrophoneUsageDescription in a
// bundled app's Info.plist); Linux and Windows gate at the OS settings level.
// So the honest answer is "try it and see", and Listen is what actually finds
// out.
func (desktopMic) Authorize(cb func(shell.Permission)) {
	if cb != nil {
		cb(shell.PermissionGranted)
	}
}

func (desktopMic) Listen(done func(shell.Monitor, error)) {
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
	m := &desktopMonitor{cap: cap, an: mic.New(rate, mic.DefaultWindow)}
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

type desktopMonitor struct {
	cap audio.Capture
	an  *mic.Analyzer

	mu      sync.Mutex
	stopped bool
}

func (m *desktopMonitor) write(pcm []float32) {
	if m.done() {
		return
	}
	m.an.Write(pcm)
}

func (m *desktopMonitor) done() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func (m *desktopMonitor) Level() float32 {
	if m.done() {
		return 0
	}
	return m.an.Level()
}

func (m *desktopMonitor) Bands(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Bands(dst)
}

func (m *desktopMonitor) Samples(dst []float32) int {
	if m.done() {
		return 0
	}
	return m.an.Samples(dst)
}

func (m *desktopMonitor) WindowSize() int { return m.an.WindowSize() }
func (m *desktopMonitor) SampleRate() int { return m.an.SampleRate() }

func (m *desktopMonitor) Stop() {
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
