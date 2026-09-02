package widget

import (
	"bytes"
	"context"
	"fmt"
	"image"

	"github.com/doug/gophics/fetch"
)

// maxImagePixels bounds a decoded image's area. 64MP is comfortably past any
// photo a UI would show inline and still caps the decode at ~256MB of RGBA —
// survivable where a bomb's 1.6GB is not.
const maxImagePixels = 64 << 20

// do loads one image.
//
// One file for both platforms now. It used to be two, because net/http drags
// crypto/tls and x509 into a browser build for a socket path that cannot run
// there — 2MB of gzipped wasm — so the js half hand-rolled a fetch() call.
// That trade still holds; it just lives in gophics/fetch, where every app gets
// it instead of only this one.
func (l *imgLoader) do(url string) loadResult {
	ctx, cancel := context.WithTimeout(context.Background(), imgFetchTimeout)
	defer cancel()

	b, err := fetch.Get(ctx, url)
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: %w", err)}
	}
	// Check the declared dimensions before decoding pixels. The byte count is
	// already bounded by fetch, but a decompression bomb is small on the wire
	// and enormous decoded — a 50KB PNG can declare 20,000×20,000 and ask for
	// 1.6GB of RGBA. DecodeConfig reads only the header.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	if px := int64(cfg.Width) * int64(cfg.Height); px > maxImagePixels {
		return loadResult{err: fmt.Errorf("netimage: %s is %dx%d (%dMP), over the %dMP limit",
			url, cfg.Width, cfg.Height, px>>20, maxImagePixels>>20)}
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	return loadResult{img: img}
}
