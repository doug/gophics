package ogg

import (
	"strings"
	"testing"
)

func TestDecodeInvalid(t *testing.T) {
	if _, err := Decode(strings.NewReader("not an ogg file")); err == nil {
		t.Fatal("expected an error decoding non-ogg data")
	}
}
