//go:build js && wasm

package web

import (
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

type webSpeakers struct{}

func (a *webSpeakers) Play(clip shell.Clip, done func(shell.Playback, error)) {
	ctx := audioContextCtor().New()
	u8 := bytesToJS(clip.Data)
	go func() {
		buf, err := await(ctx.Call("decodeAudioData", u8.Get("buffer")))
		if err != nil {
			ctx.Call("close")
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
