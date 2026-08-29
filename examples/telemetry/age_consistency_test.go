package main

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

// Every visible row's Age must advance, together, without anything being
// touched.
//
// Age is computed in cell() from the clock, so it only changes when the row is
// rebuilt. Tick used to ask for a repaint, and a repaint redraws the strings
// that are already there — so ages stood still until something dirtied a row.
// Hovering dirties exactly one row, which is why the row under the pointer
// advanced its age while every row around it stayed put.
//
// The live tail is switched off so the row set is frozen: anything that moves
// here is the clock, not new data arriving.
func TestAllVisibleAgesAdvanceTogether(t *testing.T) {
	var st *dash
	stateHook = func(d *dash) { st = d }
	defer func() { stateHook = nil }()

	// The synthetic producer is a goroutine on a real-time ticker, so the store
	// needs a moment of wall clock before there are rows to look at.
	store := NewStore(7)
	stop := make(chan struct{})
	go store.Produce(stop)
	time.Sleep(250 * time.Millisecond)
	close(stop) // freeze the data; the clock keeps running

	// Wide enough that the responsive column set includes Age.
	a := apptest.New(t, App{Store: store}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 1500, H: 700}, Font: goregular.TTF,
	}))
	a.Render()
	if st == nil {
		t.Fatal("dash state hook never fired")
	}
	st.live = false
	a.Render()

	before := visibleAges(a)
	if len(before) < 5 {
		t.Fatalf("only %d age cells on screen; the table rendered no rows, so this "+
			"test would pass without checking anything", len(before))
	}

	// Let the clock move. Nothing is hovered, clicked, or scrolled.
	for range 20 {
		a.Step(1.0 / 60)
	}
	time.Sleep(700 * time.Millisecond)
	for range 20 {
		a.Step(1.0 / 60)
	}

	after := visibleAges(a)
	if len(after) != len(before) {
		t.Fatalf("the row set changed (%d -> %d) with the live tail off",
			len(before), len(after))
	}

	moved := 0
	for i := range before {
		if before[i] != after[i] {
			moved++
		}
	}
	switch {
	case moved == 0:
		t.Errorf("none of the %d visible ages advanced over ~0.7s — the table is "+
			"repainting without rebuilding, so the clock never reaches cell()",
			len(before))
	case moved != len(before):
		t.Errorf("%d of %d ages advanced and the rest did not; they must move "+
			"together or the table shows a mix of fresh and stale rows",
			moved, len(before))
	}
}

// visibleAges pulls the Age column out of each rendered row. A row's semantics
// label is its cells joined, and Age is the second field: "15:04:05.000 1.2s
// catalog …".
func visibleAges(a *apptest.App) []string {
	var out []string
	for _, l := range a.Labels() {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		// The first field is a wall clock stamp, which is what marks a data row.
		if strings.Count(f[0], ":") != 2 {
			continue
		}
		out = append(out, f[1])
	}
	return out
}
