//go:build js

package text

import (
	"context"
	"errors"
	"log"

	"github.com/go-text/typesetting/font"
)

// WebFontSource supplies the bytes of a font covering a script.
//
// FontForScript runs on a goroutine the Shaper starts for it, never on the UI
// goroutine, so it is expected to block: the normal implementation is a network
// fetch. It is called at most once per script per Shaper. A failure is
// remembered rather than retried, because a chain that could not get a Han face
// this frame will not get one next frame either, and retrying per rune would
// turn one missing font into a request per glyph.
//
// A source must impose its own deadline — context.WithTimeout around whatever
// it calls. ctx from UseWebFonts bounds the app's lifetime, not the request's,
// and a source that never returns leaves its script permanently in flight:
// shaping keeps working, but the missing-glyph report stays suppressed for that
// script (see pendingFor), so a hung fetch is the one way to get the silence
// this mechanism exists to end.
//
// The bytes must be in a format go-text/typesetting can parse: TTF, OTF, TTC,
// dfont, or WOFF 1. **WOFF2 cannot be used** — decoding it needs a brotli
// decompressor, which is exactly the weight this whole path exists to avoid, so
// there is no WOFF2 reader in the dependency and adding one would cost more
// than the feature saves. That matters in practice because WOFF2 is what font
// CDNs serve by default: the Google Fonts CSS2 endpoint and the Fontsource
// packages both hand back `.woff2` unless asked otherwise. A source pointed at
// one of those has to name `.ttf` files explicitly. Bytes that start with the
// WOFF2 signature are reported as such rather than as a parse failure, because
// "invalid font" sends the reader looking in the wrong place.
type WebFontSource interface {
	FontForScript(ctx context.Context, script Script) ([]byte, error)
}

// UseWebFonts extends the fallback chain with fonts fetched on demand: the
// first time shaping meets a rune no registered font covers, the Shaper asks
// src for a face for that rune's script, and uses it from then on.
//
// This is the browser's answer to UseSystemFonts, and it is a different answer
// on purpose, because the obvious one does not exist. A wasm page cannot read
// the bytes of an installed font: the CSS Font Loading API has no accessor for
// a FontFace's source (see the IDL in CSS Font Loading Module Level 3 — family,
// style, weight, unicodeRange, features, variations, status, load(), and
// nothing that returns data), document.fonts holds only the faces the page's
// own CSS declared or added, never the machine's, and FontFaceSet.check()
// answers true for a family that does not exist, so it cannot even be used to
// test coverage. The one API that does hand over bytes is the Local Font Access
// API, and it is desktop-Chromium-only and permission-gated — LocalFontSource
// wraps it, with the reasons it cannot be the default. What is left, and what
// works everywhere, is to fetch the face.
//
// ctx bounds every fetch the Shaper makes: cancelling it abandons the ones in
// flight, which is the hook an app tearing a window down wants. notify is
// called once a font has landed and is called from the fetch goroutine, so it
// must do nothing but marshal back to the UI goroutine and ask for another
// frame — widget.Ctx.Post is exactly its shape. It may be nil, in which case
// the font still installs, but not until something else causes a reshape.
//
// The first frame containing an uncovered rune still draws tofu: this cannot be
// otherwise, because shaping is synchronous and the bytes are not there yet.
// The fetch makes the second frame right.
//
// Note for the integrator: a caller that caches shaped output keyed on
// paint.Painter.ShapeGen must invalidate it from notify, or the cache will
// serve the tofu it measured before the font arrived.
func (s *Shaper) UseWebFonts(ctx context.Context, src WebFontSource, notify func()) error {
	if src == nil {
		return errors.New("text: UseWebFonts: nil WebFontSource")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.system = &webFontMap{
		ctx:     ctx,
		src:     src,
		notify:  notify,
		ready:   map[Script]*font.Face{},
		pending: map[Script]bool{},
		// Buffered so a landing font never blocks its goroutine before
		// notify runs; a real chain is missing a handful of scripts, not
		// thirty-two, and if it somehow were, blocking is correct
		// back-pressure rather than a dropped font.
		landed: make(chan scriptFace, 32),
	}
	return nil
}

// webFontMap is the systemFontMap a Shaper gets from UseWebFonts: a per-script
// cache in front of a WebFontSource.
//
// Only the UI goroutine touches the maps. The fetch goroutines own nothing but
// the freshly parsed face they put on the channel, and the channel's
// send/receive is what publishes it — which is the reason for a channel rather
// than a mutex around the map. A mutex would make the map safe and leave the
// face unsafe: harfbuzz writes into a font.Face's cmap cache while shaping, so
// a face must belong to one goroutine from the moment it is first shaped
// through, and the handoff has to be a transfer of ownership rather than shared
// access. ShareSystemFonts documents the same hazard for fontscan's faces.
type webFontMap struct {
	ctx    context.Context
	src    WebFontSource
	notify func()

	// ready holds the outcome per script. A present-but-nil entry is a
	// remembered failure, which is why this is not just a map of faces: a
	// missing key means "not asked yet" and must be distinguishable from
	// "asked, and there is no face".
	ready   map[Script]*font.Face
	pending map[Script]bool
	landed  chan scriptFace
}

type scriptFace struct {
	script Script
	face   *font.Face
	err    error
}

// ResolveFace answers for r, starting a fetch for its script if this is the
// first time r's script has come up.
//
// The answer for a script already fetched is still checked against r itself.
// One face per script is a bucket, not a promise: the Han face that covers 中
// may well not cover a rare ideograph, and claiming it does would hand the
// shaper a face with no glyph — a silently wrong glyph index instead of a
// reported miss.
func (m *webFontMap) ResolveFace(r rune) *font.Face {
	m.drain()
	sc := ScriptOf(r)
	if face, asked := m.ready[sc]; asked {
		if face != nil {
			if _, ok := face.NominalGlyph(r); ok {
				return face
			}
		}
		return nil
	}
	if m.pending[sc] {
		return nil
	}
	m.pending[sc] = true
	go m.load(sc)
	return nil
}

// pendingFor implements pendingFontMap: a fetch for r's script is in flight, so
// the rune is not yet known to be uncovered and must not be reported as such.
//
// Note the consequence, which is also why WebFontSource is told to impose its
// own deadline: a source that never returns leaves its script pending for ever
// and so suppresses the report for it indefinitely.
func (m *webFontMap) pendingFor(r rune) bool { return m.pending[ScriptOf(r)] }

// drain installs everything that has arrived since the last call. It runs at
// the top of ResolveFace rather than from a timer or a callback so that
// installation happens on the goroutine that shapes, with no lock and no
// window in which a half-installed face is reachable.
func (m *webFontMap) drain() {
	for {
		select {
		case got := <-m.landed:
			delete(m.pending, got.script)
			m.ready[got.script] = got.face
			if got.err != nil {
				log.Printf("gophics/text: no web font for script %s: %v", got.script, got.err)
			}
		default:
			return
		}
	}
}

// woff2Signature is the first four bytes of a WOFF2 file.
const woff2Signature = "wOF2"

func (m *webFontMap) load(sc Script) {
	data, err := m.src.FontForScript(m.ctx, sc)
	var face *font.Face
	switch {
	case err != nil:
	case len(data) >= 4 && string(data[:4]) == woff2Signature:
		err = errors.New("the bytes are WOFF2, which gophics cannot parse " +
			"(it would need a brotli decoder); serve TTF, OTF or WOFF instead")
	default:
		var f *Font
		if f, err = Parse(data); err == nil {
			face = f.face
		}
	}
	// Send before notify: notify may schedule a frame that calls drain, and
	// a frame that drains an empty channel would leave the font to wait for
	// the frame after it.
	m.landed <- scriptFace{script: sc, face: face, err: err}
	if m.notify != nil {
		m.notify()
	}
}
