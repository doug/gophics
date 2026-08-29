package app

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type inspApp struct{}

func (inspApp) Build(widget.Ctx) widget.Widget {
	return widget.Padding{All: 10, Child: widget.Column(
		widget.Text{S: "Header"},
		widget.Interactive{Gestures: widget.Gestures{OnTap: func() {}},
			Child: widget.Decorated{Color: paint.RGB(0.2, 0.2, 0.2), Child: widget.Sized{W: 80, H: 30}}},
	)}
}

func TestInspectTree(t *testing.T) {
	h, err := NewHeadless(inspApp{}, Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	nodes := h.core.InspectTree()
	if len(nodes) < 5 {
		t.Fatalf("tree too shallow: %d nodes", len(nodes))
	}
	// Root box has the full surface rect; depth increases inward.
	if nodes[0].Rect.Dx() != 200 || nodes[0].Depth != 0 {
		t.Fatalf("root node = %+v", nodes[0])
	}
	// Contains a Flex and a button-role node with the header label somewhere.
	var haveFlex, haveButton, haveLabel bool
	for _, n := range nodes {
		if strings.Contains(n.Type, "Flex") {
			haveFlex = true
		}
		if n.Role == layout.RoleButton {
			haveButton = true
		}
		if n.Label == "Header" {
			haveLabel = true
		}
	}
	if !haveFlex || !haveButton || !haveLabel {
		t.Fatalf("inspect missing pieces: flex=%v button=%v label=%v", haveFlex, haveButton, haveLabel)
	}
	// String() renders indented.
	if !strings.Contains(nodes[1].String(), "  ") {
		t.Fatalf("depth-1 node not indented: %q", nodes[1].String())
	}
}

func TestDebugPaintChangesOutput(t *testing.T) {
	mk := func(debug bool) []byte {
		h, _ := NewHeadless(inspApp{}, Config{
			Size: geom.Size{W: 200, H: 200}, Background: paint.RGB(0, 0, 0),
			Font: goregular.TTF, Debug: debug,
		}, 1)
		img := h.Render()
		b := img.Bounds()
		out := make([]byte, 0, b.Dx()*b.Dy())
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, _, _, _ := img.At(x, y).RGBA()
				out = append(out, byte(r>>8))
			}
		}
		return out
	}
	plain := mk(false)
	debug := mk(true)
	diff := 0
	for i := range plain {
		if plain[i] != debug[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("debug paint drew no outlines")
	}
}

func TestInspectHTML(t *testing.T) {
	h, err := NewHeadless(inspApp{}, Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	page := h.core.InspectHTML()
	for _, want := range []string{"<!doctype html>", "render-tree inspector", "Flex", "button", "Header", `class="node"`, `class="row"`} {
		if !strings.Contains(page, want) {
			t.Errorf("inspector HTML missing %q", want)
		}
	}
	if out := os.Getenv("GOPHICS_INSPECT_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s", out)
	}
}
