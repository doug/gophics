package shell

import (
	"image"
	"time"
)

// Media capture capabilities. These are optional platform features: a Window
// exposes them by implementing MediaWindow, and callers reach them through the
// widget layer (ctx.Camera()/ctx.Audio()), which returns nil when the running
// platform can't provide them. Only the web shell implements them today; the
// same interfaces are the contract the mobile (gomobile) and desktop shells
// will satisfy. All callbacks fire on the UI goroutine.
//
// See docs/spec-media-journal.md.

// MediaWindow is implemented by a Window that can capture media. The app runner
// type-asserts the Window to it and, when present, publishes Camera()/Audio()
// to the widget tree.
type MediaWindow interface {
	// Camera returns a still-image capture capability, or nil if unavailable.
	Camera() Camera
	// Audio returns an audio record/playback capability, or nil if unavailable.
	Audio() Audio
}

// Permission is the outcome of a capability authorization request.
type Permission uint8

const (
	PermissionPrompt  Permission = iota // not yet decided
	PermissionGranted                   // the user allowed it
	PermissionDenied                    // the user refused it
)

// Facing selects which camera to use on a device that has more than one.
type Facing uint8

const (
	FacingBack  Facing = iota // rear / environment camera (default)
	FacingFront               // selfie / user camera
)

// CaptureOptions configures a still capture.
type CaptureOptions struct {
	Facing Facing
	MaxDim int // longest-edge cap in pixels to bound the result (0 = device default)
}

// Camera captures still images from a device camera (or a file/gallery fallback
// where a live camera isn't available).
type Camera interface {
	// Authorize requests capture permission, reporting the outcome.
	Authorize(func(Permission))
	// Capture takes one still photo; on success img is a decoded image.
	Capture(CaptureOptions, func(img image.Image, err error))
}

// RecordOptions configures a recording (reserved for sample rate etc.).
type RecordOptions struct{}

// Clip is a finished audio recording: the encoded bytes plus a display envelope.
type Clip struct {
	Data     []byte        // encoded audio bytes (format given by Mime)
	Mime     string        // e.g. "audio/webm", "audio/wav"
	Duration time.Duration // total length
	Envelope []float32     // downsampled 0..1 peaks, for the waveform view
}

// Recorder is an in-progress microphone recording. Poll Level each frame for a
// live meter; call Stop to finalize (the clip arrives on the callback).
type Recorder interface {
	Level() float32         // current input level, 0..1
	Elapsed() time.Duration // time since recording began
	Stop(func(Clip, error)) // finalize; the recorded clip arrives on the callback
	Cancel()                // discard the recording
}

// Playback controls a playing Clip.
type Playback interface {
	Position() time.Duration
	Duration() time.Duration
	Playing() bool
	Seek(time.Duration)
	Stop()
}

// Audio plays and records audio.
type Audio interface {
	// Authorize requests microphone permission (for Record).
	Authorize(func(Permission))
	// Record starts capturing the microphone; the Recorder arrives on the
	// callback once the mic is live (permission may prompt first).
	Record(RecordOptions, func(Recorder, error))
	// Play decodes and plays clip; the Playback control arrives on the callback
	// once playback starts.
	Play(Clip, func(Playback, error))
}
