//go:build js

package app

import (
	"context"

	"github.com/doug/gophics/text"
)

// loadWebFonts starts the on-demand web font chain for Config.WebFonts.
//
// It exists because a browser cannot hand a WebAssembly page the bytes of an
// installed font: the CSS Font Loading API exposes no source for a FontFace,
// and FontFaceSet.check() answers true for families that do not exist, so it
// cannot even be used to ask. Fetching the face is the only mechanism that
// works in every browser, which is why this takes URLs rather than family
// names.
//
// Nothing is fetched here. The source is lazy and per script: a run that the
// explicitly registered faces already cover never consults it, and a script
// that does appear costs one request, once. So listing a dozen scripts in a
// Config is not a dozen downloads — it is a dozen offers.
//
// notify is RequestFrameThreadSafe rather than an invalidate, because the fetch
// completes on its own goroutine and that is the one thing in the frame loop
// documented to be safe from there. Cache invalidation is deliberately *not*
// done here: Painter.UseWebFonts latches a flag and clears its shaping caches
// on the UI goroutine at the next shaping call, because those maps belong to
// the UI goroutine and clearing them from the fetch goroutine would be a race.
func loadWebFonts(c *core, urls map[text.Script]string) error {
	return c.Painter.UseWebFonts(
		context.Background(),
		text.FetchFontSource(urls),
		c.Owner.RequestFrameThreadSafe,
	)
}
