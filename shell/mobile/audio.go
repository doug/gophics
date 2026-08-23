package mobile

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/doug/gophics/shell"
)

// Recording and playback: one-shot audio transactions, each ending with a
// result. Live input monitoring belongs to the microphone, in microphone.go.

// Audio returns the record/playback capability, or nil until a MediaHost is set.
func (b *Bridge) Audio() shell.Audio {
	if b.media.host == nil {
		return nil
	}
	return &mobileAudio{b.media}
}

// --- Audio -------------------------------------------------------------------

type mobileAudio struct{ m *mediaBridge }

func (a *mobileAudio) Authorize(cb func(shell.Permission)) {
	id := a.m.newReq()
	a.m.perm[id] = cb
	a.m.host.AuthorizeMic(id)
}

func (a *mobileAudio) Record(_ shell.RecordOptions, done func(shell.Recorder, error)) {
	id := a.m.newReq()
	a.m.recCb[id] = done
	a.m.recs[id] = &mobileRecorder{m: a.m, id: id, start: time.Now()}
	a.m.host.StartRecording(id)
}

func (a *mobileAudio) Play(clip shell.Clip, done func(shell.Playback, error)) {
	id := a.m.newReq()
	a.m.playCb[id] = done
	a.m.plays[id] = &mobilePlayback{m: a.m, id: id, duration: clip.Duration}
	a.m.host.PlayClip(id, clip.Data)
}

type mobileRecorder struct {
	m      *mediaBridge
	id     int
	start  time.Time
	level  float32
	stopCb func(shell.Clip, error)
}

func (r *mobileRecorder) Level() float32         { return r.level }
func (r *mobileRecorder) Elapsed() time.Duration { return time.Since(r.start) }

func (r *mobileRecorder) Stop(done func(shell.Clip, error)) {
	r.stopCb = done
	r.m.host.StopRecording(r.id)
}

func (r *mobileRecorder) Cancel() {
	delete(r.m.recs, r.id)
	r.m.host.StopRecording(r.id)
}

// DeliverRecorderReady signals the mic is live; the Record callback fires.
func (b *Bridge) DeliverRecorderReady(reqID int) {
	cb := b.media.recCb[reqID]
	r := b.media.recs[reqID]
	if cb == nil || r == nil {
		return
	}
	delete(b.media.recCb, reqID)
	r.start = time.Now()
	cb(r, nil)
}

// FailRecording reports that recording could not start (e.g. permission denied).
func (b *Bridge) FailRecording(reqID int, msg string) {
	cb := b.media.recCb[reqID]
	delete(b.media.recCb, reqID)
	delete(b.media.recs, reqID)
	if cb != nil {
		cb(nil, errors.New(msg))
	}
}

// SetAudioLevel updates a live recording's input level (0..1) for the meter.
func (b *Bridge) SetAudioLevel(reqID int, level float32) {
	if r := b.media.recs[reqID]; r != nil {
		r.level = level
		b.dirty.Store(true)
	}
}

// DeliverPCM finalizes a recording: raw 16-bit LE mono PCM + sample rate. Go
// encodes the WAV Clip and computes its waveform envelope.
func (b *Bridge) DeliverPCM(reqID int, pcm []byte, sampleRate int, durationMs int) {
	r := b.media.recs[reqID]
	if r == nil {
		return
	}
	delete(b.media.recs, reqID)
	samples := pcmToInt16(pcm)
	dur := time.Duration(durationMs) * time.Millisecond
	if dur == 0 && sampleRate > 0 {
		dur = time.Duration(len(samples)) * time.Second / time.Duration(sampleRate)
	}
	if r.stopCb != nil {
		r.stopCb(shell.Clip{
			Data:     shell.EncodeWAV(samples, sampleRate),
			Mime:     "audio/wav",
			Duration: dur,
			Envelope: envelope(samples, 120),
		}, nil)
	}
}

type mobilePlayback struct {
	m        *mediaBridge
	id       int
	duration time.Duration
	position time.Duration
	playing  bool
}

func (p *mobilePlayback) Position() time.Duration { return p.position }
func (p *mobilePlayback) Duration() time.Duration { return p.duration }
func (p *mobilePlayback) Playing() bool           { return p.playing }
func (p *mobilePlayback) Seek(t time.Duration) {
	p.position = t
	p.m.host.SeekPlayback(p.id, int(t/time.Millisecond))
}
func (p *mobilePlayback) Stop() {
	p.playing = false
	p.m.host.StopPlayback(p.id)
}

// DeliverPlaybackReady signals playback started; the Play callback fires.
func (b *Bridge) DeliverPlaybackReady(reqID int) {
	cb := b.media.playCb[reqID]
	p := b.media.plays[reqID]
	if cb == nil || p == nil {
		return
	}
	delete(b.media.playCb, reqID)
	p.playing = true
	cb(p, nil)
}

// SetPlaybackPosition updates a playing clip's position (milliseconds).
func (b *Bridge) SetPlaybackPosition(reqID int, ms int) {
	if p := b.media.plays[reqID]; p != nil {
		p.position = time.Duration(ms) * time.Millisecond
		b.dirty.Store(true)
	}
}

// PlaybackEnded signals playback finished (natural end or Stop).
func (b *Bridge) PlaybackEnded(reqID int) {
	if p := b.media.plays[reqID]; p != nil {
		p.playing = false
		p.position = p.duration
	}
	delete(b.media.plays, reqID)
	b.dirty.Store(true)
}

// --- helpers -----------------------------------------------------------------

func pcmToInt16(b []byte) []int16 {
	n := len(b) / 2
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

// envelope downsamples |samples| into at most buckets peak values in 0..1.
func envelope(samples []int16, buckets int) []float32 {
	if len(samples) == 0 || buckets <= 0 {
		return nil
	}
	if buckets > len(samples) {
		buckets = len(samples)
	}
	out := make([]float32, buckets)
	per := len(samples) / buckets
	if per < 1 {
		per = 1
	}
	for i := 0; i < buckets; i++ {
		var peak int
		start := i * per
		for j := start; j < start+per && j < len(samples); j++ {
			v := int(samples[j])
			if v < 0 {
				v = -v
			}
			if v > peak {
				peak = v
			}
		}
		out[i] = float32(peak) / 32768
	}
	return out
}
