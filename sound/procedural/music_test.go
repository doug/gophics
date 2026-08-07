package procedural

import (
	"testing"

	"github.com/doug/gophics/sound"
)

func TestDroneContinuous(t *testing.T) {
	d := Drone(110)
	out := make([]float32, 2048)
	if !d.Process(out) {
		t.Fatal("a drone should never finish")
	}
	if peak(out) < 0.05 {
		t.Fatalf("drone too quiet: peak %v", peak(out))
	}
}

func TestDungeonMusicBoundedAndAudible(t *testing.T) {
	src := DungeonMusic(1)
	block := make([]float32, 1024)
	var max float32
	// ~6 seconds — long enough for the note scheduler to fire.
	for i := 0; i < int(6*sound.SampleRate)/len(block); i++ {
		if !src.Process(block) {
			t.Fatal("music should be continuous")
		}
		for _, v := range block {
			if v > 2 || v < -2 {
				t.Fatalf("music sample out of expected range: %v", v)
			}
			if a := absf(v); a > max {
				max = a
			}
		}
	}
	if max < 0.1 {
		t.Fatalf("music too quiet: peak %v", max)
	}
}
