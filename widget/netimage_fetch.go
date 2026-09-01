package widget

import (
	"bytes"
	"context"
	"fmt"
	"image"

	"github.com/doug/gophics/fetch"
)

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
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	return loadResult{img: img}
}
