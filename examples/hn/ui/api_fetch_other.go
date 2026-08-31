//go:build !js

package ui

import (
	"context"
	"encoding/json"
	"net/http"
)

type liveAPI struct{ client http.Client }

func newLiveAPI() *liveAPI {
	return &liveAPI{client: http.Client{Timeout: apiTimeout}}
}

func (a *liveAPI) get(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}
