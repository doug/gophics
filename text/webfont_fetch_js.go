//go:build js

package text

import (
	"context"
	"fmt"

	"github.com/doug/gophics/fetch"
)

// FetchFontSource returns a WebFontSource that downloads a font file per
// script from the URLs given, using the browser's fetch().
//
// This is the route that works in every browser, and the only one that does.
// It asks the app for something only the app can know — where its fonts are —
// and in exchange it needs no permission, no API that two of the three engines
// have declined to implement, and no user gesture.
//
//	sh.UseWebFonts(ctx, text.FetchFontSource(map[text.Script]string{
//	    text.Script("Hani"): "/fonts/noto-sans-sc-subset.ttf",
//	    text.Script("Kana"): "/fonts/noto-sans-jp-subset.ttf",
//	    text.Script("Hira"): "/fonts/noto-sans-jp-subset.ttf",
//	}), post)
//
// Serve subsets, not whole faces. A full CJK face is 10–20MB and arrives after
// the text that needed it has already been read; a subset holding the few
// thousand runes an application actually shows is a few hundred kilobytes.
// Nothing here subsets for you — that is a build step (pyftsubset, fonttools,
// glyphhanger), because it needs the corpus and the browser does not have one.
//
// The URLs are the app's, which is the point: a map rather than a built-in CDN
// so that the bytes come from somewhere the app controls and has a
// Content-Security-Policy for. A default CDN would have made this a one-liner
// and also made every gophics web app phone a third party on first sight of a
// Cyrillic name.
//
// Cross-origin URLs need CORS (`Access-Control-Allow-Origin`) — this reads the
// response body rather than handing the URL to CSS, so an opaque response is
// no use. Files must be TTF, OTF, TTC, dfont or WOFF; see WebFontSource on why
// WOFF2 is not among them.
//
// The response is bounded by fetch.DefaultMaxBytes (64MB), which is above any
// sane face and below what crashes a tab.
func FetchFontSource(urls map[Script]string) WebFontSource {
	// Copied so a caller mutating its map later cannot change what a
	// half-finished fetch is about to ask for.
	m := make(map[Script]string, len(urls))
	for k, v := range urls {
		m[k] = v
	}
	return urlFontSource(m)
}

type urlFontSource map[Script]string

func (u urlFontSource) FontForScript(ctx context.Context, sc Script) ([]byte, error) {
	url, ok := u[sc]
	if !ok {
		// Named explicitly, because the alternative message — a generic
		// "no font" — leaves the reader unable to tell a missing map entry
		// from a 404, and the fixes are in different files.
		return nil, fmt.Errorf("no URL configured for script %s", sc)
	}
	return fetch.Get(ctx, url)
}
