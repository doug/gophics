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
// See design/spec-media-journal.md for the still/clip half, and the live
// capture section at the bottom of this file for the streaming one.

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

// --- Live capture ------------------------------------------------------------

// Live capture is a separate capability from MediaWindow on purpose. Camera
// takes one still and Audio records one clip; both are one-shot transactions
// that end with a result. A preview and a microphone monitor are neither — they
// are continuous, they retain nothing, and a platform can perfectly well offer
// one pair and not the other. Splitting them means the shells that haven't
// implemented streaming yet simply don't publish it, and ctx.CameraPreview()
// is nil, instead of every shell carrying a method that returns "unsupported".

// LiveMediaWindow is implemented by a Window that can stream live capture. The
// app runner type-asserts the Window to it and, when present, publishes
// CameraPreview()/Microphone() to the widget tree.
type LiveMediaWindow interface {
	// CameraPreview returns live camera capture, or nil if unavailable.
	CameraPreview() CameraPreview
	// Microphone returns live input monitoring, or nil if unavailable.
	Microphone() Microphone
}

// PreviewOptions configures a live camera preview.
type PreviewOptions struct {
	Facing Facing
	// Width is the requested frame width in pixels (0 → a platform default).
	// It is a hint: the camera picks the nearest mode it has, and the frames
	// that arrive may be any size — read it off the image, don't assume it.
	//
	// It is worth setting. Every frame is copied out of the platform's buffer
	// into Go memory, so the cost of a preview scales with its area: a 1280-wide
	// frame is four times the copy of a 640-wide one, every frame, forever.
	Width int
}

// CameraPreview is live camera capture — a stream of frames an app can draw
// each time it paints, as opposed to Camera's single still.
type CameraPreview interface {
	// Authorize requests capture permission, reporting the outcome.
	Authorize(func(Permission))
	// Start opens the camera; the Frames handle arrives on the callback once
	// the stream is live (permission may prompt first). Stop it when done —
	// the camera stays on, and lit, until you do.
	Start(PreviewOptions, func(Frames, error))
}

// Frames is a running camera preview. Poll it whenever you draw.
type Frames interface {
	// Frame returns the most recent camera frame, or nil before the first one
	// has arrived. Polling faster than the camera produces frames is free: the
	// same image comes back until there is genuinely a new one.
	//
	// Successive frames are distinct image values — implementations rotate a
	// small pool of buffers rather than repainting one in place. That is not an
	// implementation detail to rely on loosely: the scene compares images by
	// identity, so a canvas handed the same *image.RGBA with different pixels
	// inside it would never repaint. Because they rotate, a frame's memory is
	// reused after a few more; draw or copy it now, don't retain it.
	Frame() *image.RGBA
	// Stop ends the preview and releases the camera.
	Stop()
}

// Microphone is live input monitoring: the analysis half of recording, with
// nothing kept, so it can run for as long as a visualization needs it to.
type Microphone interface {
	// Authorize requests microphone permission, reporting the outcome.
	Authorize(func(Permission))
	// Listen opens the microphone; the Monitor arrives on the callback once
	// input is live (permission may prompt first).
	Listen(func(Monitor, error))
}

// Monitor is a running microphone monitor. Poll it whenever you draw.
type Monitor interface {
	// Level is the current input level, 0..1 — the peak of the most recent
	// block, the same quantity Recorder.Level reports.
	Level() float32
	// Bands fills dst with the input spectrum, lowest frequency first, as
	// magnitudes in 0..1, and returns how many it wrote (at most len(dst)).
	//
	// The caller's slice length *is* the band count: the implementation folds
	// its own analysis resolution onto whatever is asked for, grouping
	// logarithmically so the bands are musically even rather than linear in
	// hertz. A visualization therefore never has to know or care about the FFT
	// size behind it — ask for as many bars as you intend to draw.
	Bands(dst []float32) int
	// Samples fills dst with the most recent window of time-domain PCM in
	// [-1,1], oldest sample first, and returns how many it wrote (at most
	// len(dst)). Successive calls overlap: the window slides with real time
	// rather than advancing by whole buffers, so a caller polling once a frame
	// sees the newest audio without having to keep up with the capture rate.
	//
	// This is the raw signal, deliberately unlike Bands. Bands is folded for
	// display and throws away the resolution that pitch analysis needs; a tuner
	// or a note-matching exercise must run its own autocorrelation over real
	// samples (see sound/pitch). Ask for at most WindowSize samples — a longer
	// dst is short-filled, and the return value, not len(dst), is the count.
	Samples(dst []float32) int
	// WindowSize is the largest number of samples Samples can return, and
	// SampleRate is the capture rate in Hz. Together they bound the lowest
	// frequency the input can resolve, which for pitch detection is the
	// difference between hearing a bass note and reporting silence.
	WindowSize() int
	SampleRate() int
	// Stop ends monitoring and releases the microphone.
	Stop()
}
