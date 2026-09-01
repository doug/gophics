//go:build !js

package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// client is shared so connections are pooled across calls. http.DefaultClient
// has no timeout, which is right here: the deadline belongs to the caller's
// context, where it is visible.
var client = &http.Client{}

// Do performs r using net/http.
func Do(ctx context.Context, r Request) (*Response, error) {
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if len(r.Body) > 0 {
		body = bytes.NewReader(r.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, body)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", r.URL, err)
	}
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", r.URL, err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: reading body: %w", r.URL, err)
	}
	h := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		h[k] = resp.Header.Get(k)
	}
	return &Response{Status: resp.StatusCode, Header: h, Body: b}, nil
}
