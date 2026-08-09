package layout

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
)

func textPainter(t *testing.T) *paint.Painter {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTextWrapsWithinConstraint(t *testing.T) {
	p := textPainter(t)
	tb := &TextBox{Painter: p, Text: "the quick brown fox jumps over the lazy dog", TextSize: 14, Wrap: true}

	wide := tb.Layout(Loose(geom.Size{W: 1000, H: 1000}))
	if wide.H != p.Metrics(14).Ascent+p.Metrics(14).Descent {
		t.Fatalf("wide layout should be single-line, H=%v", wide.H)
	}

	narrow := tb.Layout(Loose(geom.Size{W: 120, H: 1000}))
	if narrow.H <= wide.H {
		t.Fatal("narrow layout should wrap to multiple lines")
	}
	if narrow.W > 120 {
		t.Fatalf("wrapped width %v exceeds constraint 120", narrow.W)
	}

	// Unwrapped ignores the width constraint's effect on line count.
	// Direct config mutation requires MarkDirty (the widget layer does this
	// automatically on every updateBox).
	tb.Wrap = false
	tb.MarkDirty()
	single := tb.Layout(Loose(geom.Size{W: 120, H: 1000}))
	if single.H != wide.H {
		t.Fatal("non-wrap must stay single-line")
	}
}

func TestWrapRespectsNewlines(t *testing.T) {
	p := textPainter(t)
	lines := p.WrapText("a\nb\nc", 14, 1000)
	if len(lines) != 3 {
		t.Fatalf("newline wrap = %v", lines)
	}
}

func TestDecorationsPaintLines(t *testing.T) {
	p := textPainter(t)
	tb := &TextBox{Painter: p, Text: "one two three four five six", TextSize: 14,
		Wrap: true, Strike: true, Underline: true}
	tb.Layout(Loose(geom.Size{W: 80, H: 1000}))

	var list scene.List
	tb.Paint(list.Recorder(), geom.Pt{})
	// Each painted line contributes one text op + two line ops.
	if list.Len()%3 != 0 || list.Len() < 6 {
		t.Fatalf("ops = %d, want 3 per wrapped line (≥2 lines)", list.Len())
	}
}

// TestTextMemoAcrossConstraintChanges pins the measure memo (M12): a
// re-layout with the same text at the same width but a different height
// constraint (the Base skip-cache misses) must produce the identical result,
// and a text change after a memo hit must recompute.
func TestTextMemoAcrossConstraintChanges(t *testing.T) {
	p := textPainter(t)
	tb := &TextBox{Painter: p, Text: "the quick brown fox jumps over the lazy dog", TextSize: 14, Wrap: true}

	s1 := tb.Layout(Loose(geom.Size{W: 120, H: 1000}))
	tb.MarkDirty()                                    // force a real re-layout (not the Base skip-cache)
	s2 := tb.Layout(Loose(geom.Size{W: 120, H: 500})) // memo hit: same width
	if s1 != s2 {
		t.Fatalf("height-only constraint change altered size: %v vs %v", s1, s2)
	}

	tb.Text = "tiny"
	tb.MarkDirty()
	s3 := tb.Layout(Loose(geom.Size{W: 120, H: 500}))
	if s3.W >= s1.W || s3.H >= s1.H {
		t.Fatalf("text change after memo hit not recomputed: %v (was %v)", s3, s1)
	}
}

// TestRichMemoAcrossConstraintChanges is the RichBox analog.
func TestRichMemoAcrossConstraintChanges(t *testing.T) {
	p := textPainter(t)
	rb := &RichBox{Painter: p, TextSize: 14, Spans: []RichSpan{
		{Text: "the quick brown "}, {Text: "fox jumps over the lazy dog"},
	}}

	s1 := rb.Layout(Loose(geom.Size{W: 120, H: 1000}))
	segs := len(rb.segs)
	rb.MarkDirty()
	s2 := rb.Layout(Loose(geom.Size{W: 120, H: 500})) // memo hit: same width
	if s1 != s2 || len(rb.segs) != segs {
		t.Fatalf("height-only constraint change altered rich layout: %v/%d vs %v/%d", s1, segs, s2, len(rb.segs))
	}

	rb.Spans = []RichSpan{{Text: "tiny"}}
	rb.MarkDirty()
	s3 := rb.Layout(Loose(geom.Size{W: 120, H: 500}))
	if s3.W >= s1.W || s3.H >= s1.H {
		t.Fatalf("span change after memo hit not recomputed: %v (was %v)", s3, s1)
	}
}
