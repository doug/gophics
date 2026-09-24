// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import "testing"

// Players used to be added to the context and mixer and never removed: Stop
// set done, EOF set done, and both left the player — and its source, the whole
// decoded clip — in the mix list for the life of the context, walked by every
// Mix call. shell/internal/devmedia creates a new player on every Play and
// every Seek, so a long-running app grew a copy of the clip per seek.

func TestPlayer_StopDetachesFromContextAndMixer(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	p := ctx.NewPlayer(makePCMReader(make([]float32, 4096)))
	p.Play()
	if n := sourceCount(ctx); n != 1 {
		t.Fatalf("after NewPlayer: %d mixer sources, want 1", n)
	}
	p.Stop()
	if n := sourceCount(ctx); n != 0 {
		t.Errorf("after Stop: %d mixer sources, want 0", n)
	}
	if n := playerCount(ctx); n != 0 {
		t.Errorf("after Stop: context still tracks %d players, want 0", n)
	}
	p.Stop() // idempotent: a second detach finds nothing to remove
}

func TestMixer_DropsSourcesAtEOF(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	short := ctx.NewPlayer(makePCMReader([]float32{0.5, 0.5}))
	short.Play()
	long := ctx.NewPlayer(makePCMReader(make([]float32, 1<<16)))
	long.Play()

	out := make([]float32, 64)
	ctx.mixer.Mix(out) // the short player hits EOF here
	ctx.mixer.Mix(out) // and is gone from the list on the next pass
	if n := sourceCount(ctx); n != 1 {
		t.Errorf("after EOF: %d mixer sources, want 1 (the long player)", n)
	}
	if !short.isDone() {
		t.Error("the short player did not report done at EOF")
	}

	// The context forgets finished players the next time one is created, so
	// the list is bounded by players live at once, not players ever made.
	_ = ctx.NewPlayer(makePCMReader(make([]float32, 16)))
	if n := playerCount(ctx); n != 2 {
		t.Errorf("after a new player: context tracks %d players, want 2 (long + new)", n)
	}
}

func sourceCount(c *Context) int {
	c.mixer.mu.Lock()
	defer c.mixer.mu.Unlock()
	return len(c.mixer.sources)
}

func playerCount(c *Context) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.players)
}
