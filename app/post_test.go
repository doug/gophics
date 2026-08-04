package app

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type asyncApp struct{ hook func(*asyncState) }

func (a asyncApp) CreateState() widget.State { return &asyncState{hook: a.hook} }

type asyncState struct {
	widget.StateBase[asyncApp]
	hook   func(*asyncState)
	post   func(func())
	loaded string
}

func (s *asyncState) Init(ctx widget.Ctx) {
	s.hook(s)
	s.post = ctx.Post()
	go func() { // simulated fetch off the UI goroutine
		time.Sleep(5 * time.Millisecond)
		s.post(func() { s.SetState(func() { s.loaded = "stories" }) })
	}()
}

func (s *asyncState) Build(widget.Ctx) widget.Widget {
	return widget.Text{S: s.loaded}
}

func TestPostDeliversToUIGoroutine(t *testing.T) {
	var st *asyncState
	h, err := NewHeadless(asyncApp{hook: func(s *asyncState) { st = s }}, Config{
		Size: geom.Size{W: 100, H: 40}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st.loaded != "" {
		t.Fatal("loaded too early")
	}
	deadline := time.Now().Add(2 * time.Second)
	for st.loaded == "" && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
		h.Render() // frames drain posted work
	}
	if st.loaded != "stories" {
		t.Fatalf("posted result not applied: %q", st.loaded)
	}
}
