package mobile_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell/mobile"
	"github.com/doug/gossamer/widget"
)

type tapApp struct{}

func (tapApp) CreateState() widget.State { return &tapState{} }

type tapState struct {
	widget.StateBase[tapApp]
	on bool
}

func (s *tapState) Build(ctx widget.Ctx) widget.Widget {
	col := paint.RGB(0.1, 0.1, 0.9)
	if s.on {
		col = paint.RGB(0.9, 0.1, 0.1)
	}
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.on = !s.on }) }},
		Child: widget.Decorated{Color: col, Child: widget.Center(
			widget.Interactive{
				Handler: widget.Handler{OnTap: func() { _ = ctx.OpenURL("https://example.com") }},
				Child:   widget.Sized{W: 40, H: 40},
			},
		)},
	}
}

// TestBridgeEndToEnd drives the full mobile contract headless: resize,
// frames, touch taps mutating state, invalidation, and URL requests.
func TestBridgeEndToEnd(t *testing.T) {
	h, err := app.NewHandler(tapApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.Resize(400, 600, 2) // logical 200x300

	if !b.NeedsFrame() {
		t.Fatal("fresh bridge must want a frame")
	}
	pix := b.RenderFrame(0.016)
	if len(pix) != 400*600*4 {
		t.Fatalf("frame bytes = %d, want %d", len(pix), 400*600*4)
	}
	// Corner pixel is the blue background.
	if r, bl := pix[0], pix[2]; r > 100 || bl < 100 {
		t.Fatalf("expected blue start, got r=%d b=%d", r, bl)
	}

	// Tap the corner (outer interactive): toggles to red. Physical coords.
	b.Touch(mobile.TouchDown, 20, 20)
	b.Touch(mobile.TouchUp, 20, 20)
	if !b.NeedsFrame() {
		t.Fatal("tap must invalidate")
	}
	pix = b.RenderFrame(0.016)
	if r := pix[0]; r < 100 {
		t.Fatalf("expected red after tap, got r=%d", r)
	}

	// Tap the center child: requests a URL for the host.
	b.Touch(mobile.TouchDown, 200, 300)
	b.Touch(mobile.TouchUp, 200, 300)
	if u := b.TakeOpenedURL(); u != "https://example.com" {
		t.Fatalf("opened url = %q", u)
	}
	if u := b.TakeOpenedURL(); u != "" {
		t.Fatalf("url queue should be empty, got %q", u)
	}

	// Resize re-renders at the new physical size.
	b.Resize(200, 200, 1)
	pix = b.RenderFrame(0.016)
	if len(pix) != 200*200*4 {
		t.Fatalf("resized frame bytes = %d", len(pix))
	}
}
