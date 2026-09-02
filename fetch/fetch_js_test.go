//go:build js && wasm

package fetch

// These run under node via go_js_wasm_exec — the js test lane in
// scripts/ci-local.sh. Node has the same whatwg fetch(), AbortController and
// data:-URL support the browser does, so the entire js implementation runs for
// real; what node cannot supply is a DOM, which this package never touches.

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func dataURL(mime string, body []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(body)
}

func TestGetReadsABody(t *testing.T) {
	want := `{"ok":true}`
	got, err := Get(context.Background(), dataURL("application/json", []byte(want)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The body is read as an ArrayBuffer, not text. Reading it as a string would
// round-trip through UTF-16 and corrupt exactly these bytes.
func TestBinaryBodySurvives(t *testing.T) {
	want := []byte{0x00, 0xff, 0xfe, 0x80, 0x01}
	got, err := Get(context.Background(), dataURL("application/octet-stream", want))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("got % x, want % x", got, want)
	}
}

// The size cap holds on the js path too — checked against the ArrayBuffer's
// length before the copy into Go memory, which on wasm is the allocation that
// kills the tab.
func TestMaxBytesHoldsOnJS(t *testing.T) {
	big := make([]byte, 2048)
	_, err := Do(context.Background(), Request{URL: dataURL("application/octet-stream", big), MaxBytes: 1024})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized response gave %v, want a size error", err)
	}
	resp, err := Do(context.Background(), Request{URL: dataURL("application/octet-stream", big), MaxBytes: 2048})
	if err != nil || len(resp.Body) != 2048 {
		t.Fatalf("at-cap response gave %d bytes, %v", len(resp.Body), err)
	}
}

// A context cancelled before the call starts must not fire a request at all.
func TestPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Get(ctx, dataURL("text/plain", []byte("x"))); err == nil {
		t.Fatal("a cancelled context still fetched")
	}
}
