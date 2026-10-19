//go:build js

package paint

import (
	"context"
	"sync/atomic"

	"github.com/doug/gophics/text"
)

// UseWebFonts extends every family with fonts fetched on demand, the browser's
// answer to LoadSystemFonts — see text.Shaper.UseWebFonts for what a
// WebFontSource is, why the bytes cannot come from the machine's installed
// fonts, and why WOFF2 is not a format it can take.
//
// The shape mirrors LoadSystemFonts exactly: the default family's shaper owns
// the map and the rest share it, including families registered afterwards,
// because LoadFontFamily hands a new shaper the default's map through the same
// ShareSystemFonts call. One map means one fetch per script for the whole
// Painter rather than one per family, which matters when the fetch is a
// network round trip.
//
// notify is called once a font has landed. It arrives on the fetch goroutine,
// so it must do nothing but marshal back to the UI goroutine and ask for a
// frame — widget.Ctx.Post is its shape. It may be nil, in which case the font
// still installs but nothing redraws until something else causes a frame.
//
// The Painter's own caches are invalidated for you. That is the whole reason
// this method exists rather than leaving an app to call
// Shaper.UseWebFonts itself: the Painter memoizes shaped lines, ink bounds,
// metrics and — worst of all — *rasterized* text runs, and a run cached before
// the font arrived is the tofu it drew then, blitted for as long as the entry
// lives. A feature that fetches the right font and then keeps painting the
// wrong glyphs looks broken in precisely the way that makes someone conclude
// it does not work. The invalidation itself happens on the UI goroutine rather
// than inside notify; see the fontLanded field for why it cannot be done where
// it is triggered.
//
// The caches are also dropped here, at registration, which is not merely
// tidiness: a measurement already memoized is returned without the shaper
// being consulted, so a string this Painter has measured before the call would
// never reach the fallback chain and never trigger the fetch its missing rune
// needs. Shaping has to be allowed to miss once for a font to be asked for at
// all.
func (p *Painter) UseWebFonts(ctx context.Context, src text.WebFontSource, notify func()) error {
	// One flag per Painter, kept across repeated calls: a second
	// UseWebFonts replaces the Shaper's map, and a notify still in flight
	// from the first one must land somewhere that applyFontLanding reads.
	landed := p.fontLanded
	if landed == nil {
		landed = new(atomic.Bool)
		p.fontLanded = landed
	}
	def := p.shapers[""]
	if err := def.UseWebFonts(ctx, src, func() {
		landed.Store(true)
		if notify != nil {
			notify()
		}
	}); err != nil {
		return err
	}
	for name, sh := range p.shapers {
		if name != "" {
			sh.ShareSystemFonts(def)
		}
	}
	p.invalidateShapedText()
	return nil
}
