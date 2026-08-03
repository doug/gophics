package mobile_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/mobile"
)

// fakeHost stands in for the native (iOS/Android) media backend: it services
// each request synchronously by delivering canned results back through the
// Bridge — exactly the contract a real host implements, so the Go path is
// exercised end to end without any native code.
type fakeHost struct {
	b       *mobile.Bridge
	photo   []byte // canned PNG returned by CapturePhoto
	pcm     []byte // canned PCM returned by StopRecording
	rate    int
	lastWAV []byte // WAV handed to PlayClip (the round-tripped recording)
}

func (h *fakeHost) AuthorizeCamera(reqID int)      { h.b.DeliverPermission(reqID, true) }
func (h *fakeHost) CapturePhoto(reqID, facing int) { h.b.DeliverPhoto(reqID, h.photo) }
func (h *fakeHost) AuthorizeMic(reqID int)         { h.b.DeliverPermission(reqID, true) }
func (h *fakeHost) StartRecording(reqID int) {
	h.b.DeliverRecorderReady(reqID)
	h.b.SetAudioLevel(reqID, 0.5)
}
func (h *fakeHost) StopRecording(reqID int) { h.b.DeliverPCM(reqID, h.pcm, h.rate, 100) }
func (h *fakeHost) PlayClip(reqID int, wav []byte) {
	h.lastWAV = wav
	h.b.DeliverPlaybackReady(reqID)
	h.b.SetPlaybackPosition(reqID, 50)
}
func (h *fakeHost) SeekPlayback(reqID, ms int) {}
func (h *fakeHost) StopPlayback(reqID int)     { h.b.PlaybackEnded(reqID) }

func TestBridgeMediaCapabilities(t *testing.T) {
	h, err := app.NewHandler(tapApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)

	// No host yet → capabilities are nil (app degrades to text-only).
	if b.Camera() != nil || b.Audio() != nil {
		t.Fatal("capabilities must be nil before a MediaHost is registered")
	}

	// Canned inputs.
	src := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var pbuf bytes.Buffer
	if err := png.Encode(&pbuf, src); err != nil {
		t.Fatal(err)
	}
	pcm := []int16{0, 8000, -8000, 16000, -16000, 100}
	raw := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(s))
	}
	host := &fakeHost{b: b, photo: pbuf.Bytes(), pcm: raw, rate: 48000}
	b.SetMediaHost(host)

	cam, aud := b.Camera(), b.Audio()
	if cam == nil || aud == nil {
		t.Fatal("capabilities must be present after SetMediaHost")
	}

	// Camera: Capture decodes the delivered photo.
	var gotImg image.Image
	var capErr error
	cam.Capture(shell.CaptureOptions{Facing: shell.FacingFront}, func(img image.Image, err error) {
		gotImg, capErr = img, err
	})
	if capErr != nil || gotImg == nil {
		t.Fatalf("capture failed: img=%v err=%v", gotImg, capErr)
	}
	if b := gotImg.Bounds(); b.Dx() != 4 || b.Dy() != 3 {
		t.Fatalf("decoded photo bounds = %v, want 4x3", b)
	}

	// Audio: record → live level → stop yields a portable WAV clip.
	var rec shell.Recorder
	aud.Record(shell.RecordOptions{}, func(r shell.Recorder, err error) { rec = r })
	if rec == nil {
		t.Fatal("Record did not deliver a recorder")
	}
	if lvl := rec.Level(); lvl != 0.5 {
		t.Fatalf("recorder level = %v, want 0.5", lvl)
	}
	var clip shell.Clip
	rec.Stop(func(c shell.Clip, err error) { clip = c })
	if clip.Mime != "audio/wav" || len(clip.Data) <= 44 {
		t.Fatalf("clip not a WAV: mime=%q len=%d", clip.Mime, len(clip.Data))
	}
	if len(clip.Envelope) == 0 {
		t.Fatal("clip has no waveform envelope")
	}
	// The WAV round-trips back to the delivered PCM.
	got, gotRate, err := shell.DecodeWAV(clip.Data)
	if err != nil {
		t.Fatal(err)
	}
	if gotRate != 48000 || len(got) != len(pcm) {
		t.Fatalf("decoded WAV rate=%d n=%d, want 48000 n=%d", gotRate, len(got), len(pcm))
	}
	for i := range pcm {
		if got[i] != pcm[i] {
			t.Fatalf("sample %d = %d, want %d", i, got[i], pcm[i])
		}
	}

	// Playback: the recorded WAV is what gets played; position + end propagate.
	var pb shell.Playback
	aud.Play(clip, func(p shell.Playback, err error) { pb = p })
	if pb == nil || !pb.Playing() {
		t.Fatal("Play did not start")
	}
	if !bytes.Equal(host.lastWAV, clip.Data) {
		t.Fatal("PlayClip received different bytes than the clip")
	}
	if pb.Position() != 50*1e6 { // 50ms
		t.Fatalf("position = %v, want 50ms", pb.Position())
	}
	pb.Stop()
	if pb.Playing() {
		t.Fatal("playback still playing after Stop")
	}
}
