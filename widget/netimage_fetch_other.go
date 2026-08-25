//go:build !js

package widget

import (
	"fmt"
	"image"
	"net/http"
)

var httpClient = http.Client{Timeout: imgFetchTimeout}

func (l *imgLoader) do(url string) loadResult {
	resp, err := httpClient.Get(url)
	if err != nil {
		return loadResult{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return loadResult{err: fmt.Errorf("netimage: %s: %s", url, resp.Status)}
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	return loadResult{img: img}
}
