package shell

import (
	"image"
	"time"
)

// The camera, the microphone and the speakers.
//
// Each is an optional platform feature a Window opts into by implementing the
// matching <Cap>Window interface; callers reach them through the widget layer
// (ctx.Camera(), ctx.Microphone(), ctx.Speakers()), which is nil where the
// platform cannot provide them. All callbacks fire on the UI goroutine.
//
// One capability per Window interface, which is how every other capability in
// this package is declared. Media used to be the exception: MediaWindow paired
// the camera with audio and LiveMediaWindow paired the preview with the
// microphone, grouping by lifecycle rather than by device. It made a shell
// implement a device it does not have in order to publish one it does — the
// terminal carried a nil Camera purely to reach the speakers, and three files
// carried comments apologising for the pairing.
//
// The lifecycle distinction is real, but it lives *within* a device: a camera
// takes a still and streams a preview, a microphone records a clip and streams
// a monitor. So it is a pair of methods, not a pair of interfaces.
//
// See design/spec-media-journal.md for the still/clip half.

// CameraWindow is implemented by a Window that can take a still photo.
type CameraWindow interface {
	// Camera returns still capture, or nil if unavailable.
	Camera() Camera
}

// CameraPreviewWindow is implemented by a Window that can stream the camera.
type CameraPreviewWindow interface {
	// CameraPreview returns live preview, or nil if unavailable.
	CameraPreview() CameraPreview
}

// MicrophoneWindow is implemented by a Window that can capture audio input.
type MicrophoneWindow interface {
	// Microphone returns audio input, or nil if unavailable.
	Microphone() Microphone
}

// SpeakersWindow is implemented by a Window that can play audio.
type SpeakersWindow interface {
	// Speakers returns audio output, or nil if unavailable.
	Speakers() Speakers
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

// Speakers plays audio.
//
// Output only, and named for the device rather than for the medium. The
// interface it replaced was called Audio and carried Record alongside Play,
// which put the microphone and the speakers behind one name and left the
// speakers with no name at all.
//
// There is no Authorize: no platform gates playback.
type Speakers interface {
	// Play decodes and plays clip; the Playback control arrives on the callback
	// once playback starts.
	Play(Clip, func(Playback, error))
}

// --- Live capture ------------------------------------------------------------

// A still and a preview stay separate capabilities even though they are one
// device, because a platform really can have one without the other: the mobile
// hosts implement capture and preview through different native APIs and may
// register either. The microphone's two shapes come from one device open, so
// they are one interface; the camera's two do not.

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

// Microphone is audio input, in both of the shapes an app asks for it.
//
// Record is a one-shot transaction ending in a Clip; Listen is an open stream
// that keeps nothing and can run as long as a visualization needs. Same device,
// same permission, so one interface — a platform that has the microphone has
// both, and one that does not has neither.
type Microphone interface {
	// Authorize requests microphone permission, reporting the outcome.
	Authorize(func(Permission))
	// Listen opens the microphone; the Monitor arrives on the callback once
	// input is live (permission may prompt first).
	Listen(func(Monitor, error))
	// Record starts capturing to a clip; the Recorder arrives on the callback
	// once the mic is live (permission may prompt first).
	Record(RecordOptions, func(Recorder, error))
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
