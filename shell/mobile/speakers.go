package mobile

import (
	"time"

	"github.com/doug/gophics/shell"
)

// Audio output, over MediaHost. Recording is the microphone's, in
// microphone.go, even though it shares this host.

// Speakers returns audio output, or nil until a MediaHost is set.
func (b *Bridge) Speakers() shell.Speakers {
	if b.media.host == nil {
		return nil
	}
	return &mobileSpeakers{b.media}
}

// --- Speakers ----------------------------------------------------------------

type mobileSpeakers struct{ m *mediaBridge }

func (a *mobileSpeakers) Play(clip shell.Clip, done func(shell.Playback, error)) {
	id := a.m.newReq()
	a.m.playCb[id] = done
	a.m.plays[id] = &mobilePlayback{m: a.m, id: id, duration: clip.Duration}
	a.m.host.PlayClip(id, clip.Data)
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
