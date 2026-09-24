package mobile

import (
	"image"
	_ "image/jpeg" // register decoders for captured photos
	_ "image/png"

	"github.com/doug/gophics/shell"
)

// MediaHost is implemented by the native host (iOS/Android) and registered via
// Bridge.SetMediaHost. Go calls these to start asynchronous native operations —
// camera capture, mic recording, playback — and the host reports results back
// through the Bridge.Deliver…, Set… and Fail… methods, correlating by reqID.
//
// Every Deliver…, Set… and Fail… call MUST be made on the host's UI thread (like the
// rest of the Bridge), because delivering a result runs app callbacks that
// mutate the widget tree. Native capture callbacks that arrive on a background
// thread must be marshaled to the UI thread first.
//
// Recording delivers raw PCM (16-bit little-endian mono) plus its sample rate;
// Go encodes the portable WAV Clip with wav.Encode, so the format matches
// the web shell exactly. Playback is given that WAV via PlayClip.
//
// facing, wherever it appears, is the shell's encoding — int(shell.Facing):
// 0 for the back camera, 1 for the front. PreviewHost uses the same one, so a
// host that implements both never has to translate.
type MediaHost interface {
	AuthorizeCamera(reqID int)          // → DeliverPermission(reqID, granted)
	CapturePhoto(reqID int, facing int) // → DeliverPhoto(reqID, jpeg) | FailCapture(reqID, msg); facing: 0 back, 1 front
	AuthorizeMic(reqID int)             // → DeliverPermission(reqID, granted)
	StartRecording(reqID int)           // → DeliverRecorderReady(reqID) | FailRecording(reqID, msg); SetAudioLevel while live
	StopRecording(reqID int)            // → DeliverPCM(reqID, pcm, sampleRate, durationMs)
	PlayClip(reqID int, wav []byte)     // → DeliverPlaybackReady(reqID); SetPlaybackPosition; PlaybackEnded
	SeekPlayback(reqID int, ms int)
	StopPlayback(reqID int)
}

// SetMediaHost registers the native media backend. Until it is set,
// Camera()/Speakers() return nil (the app degrades to text-only) and
// Microphone() has no recording half.
//
// It announces the change like every other host setter: the runtime reads
// what a Window offers once, so a host that registers media after the first
// frame would otherwise leave the camera and speakers nil for the life of the
// window.
func (b *Bridge) SetMediaHost(h MediaHost) { b.media.host = h; b.capabilitiesChanged() }

// mediaBridge holds the pending-request bookkeeping for media capture.
type mediaBridge struct {
	b    *Bridge
	host MediaHost
	next int

	perm   map[int]func(shell.Permission)
	photo  map[int]func(image.Image, error)
	recCb  map[int]func(shell.Recorder, error)
	recs   map[int]*mobileRecorder
	playCb map[int]func(shell.Playback, error)
	plays  map[int]*mobilePlayback
}

func newMediaBridge(b *Bridge) *mediaBridge {
	return &mediaBridge{
		b:      b,
		perm:   map[int]func(shell.Permission){},
		photo:  map[int]func(image.Image, error){},
		recCb:  map[int]func(shell.Recorder, error){},
		recs:   map[int]*mobileRecorder{},
		playCb: map[int]func(shell.Playback, error){},
		plays:  map[int]*mobilePlayback{},
	}
}

func (m *mediaBridge) newReq() int { m.next++; return m.next }

// DeliverPermission answers an Authorize request.
func (b *Bridge) DeliverPermission(reqID int, granted bool) {
	cb := b.media.perm[reqID]
	if cb == nil {
		return
	}
	delete(b.media.perm, reqID)
	if granted {
		cb(shell.PermissionGranted)
	} else {
		cb(shell.PermissionDenied)
	}
}
