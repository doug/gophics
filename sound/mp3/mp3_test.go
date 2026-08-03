package mp3

import (
	"strings"
	"testing"
)

func TestDecodeInvalid(t *testing.T) {
	if _, err := Decode(strings.NewReader("not an mp3 file")); err == nil {
		t.Fatal("expected an error decoding non-mp3 data")
	}
}
