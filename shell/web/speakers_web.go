//go:build js && wasm

package web

import (
	"syscall/js"
	"time"

	"github.com/doug/gophics/shell"
)

// Audio output in the browser, over Web Audio.

// The window opts into playback by implementing shell.SpeakersWindow; this is
// the compile-time check that it still does.
var _ shell.SpeakersWindow = (*window)(nil)

// Speakers returns audio output, or nil when the browser lacks Web Audio
// (e.g. an insecure context).
func (w *window) Speakers() shell.Speakers {
	if audioContextCtor().IsUndefined() {
		return nil
	}
	if w.spk == nil {
		w.spk = &webSpeakers{}
	}
	return w.spk
}

// webSpeakers owns one AudioContext for every clip it plays. A context is a
// live hardware output stream, and browsers cap how many a page may hold;
// constructing one per Play and never closing it — nothing in a Playback's
// life ever did — accumulated one per UI sound until they stopped working.
type webSpeakers struct{ ctx js.Value }

// context returns the shared AudioContext, creating it on first use.
func (a *webSpeakers) context() js.Value {
	if !a.ctx.Truthy() {
		a.ctx = audioContextCtor().New()
	}
	return a.ctx
}

func (a *webSpeakers) Play(clip shell.Clip, done func(shell.Playback, error)) {
	ctx := a.context()
	u8 := bytesToJS(clip.Data)
	go func() {
		buf, err := await(ctx.Call("decodeAudioData", u8.Get("buffer")))
		if err != nil {
			if done != nil {
				done(nil, err)
			}
			return
		}
		p := &webPlayback{ctx: ctx, buffer: buf,
			duration: time.Duration(buf.Get("duration").Float() * float64(time.Second))}
		p.startFrom(0)
		if done != nil {
			done(p, nil)
		}
	}()
}
