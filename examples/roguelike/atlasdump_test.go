package main

import (
	"image/png"
	"os"
	"testing"
)

// TestDumpAtlas writes the generated atlas when ATLAS_SHOT is set. Scratch.
func TestDumpAtlas(t *testing.T) {
	out := os.Getenv("ATLAS_SHOT")
	if out == "" {
		t.Skip("set ATLAS_SHOT")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, buildAtlas()); err != nil {
		t.Fatal(err)
	}
}
