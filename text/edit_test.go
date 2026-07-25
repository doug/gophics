package text

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestEditorInsertDelete(t *testing.T) {
	var e Editor
	e.Insert("hello")
	if e.Text() != "hello" || e.Caret != 5 {
		t.Fatalf("text=%q caret=%d", e.Text(), e.Caret)
	}
	e.MoveTo(2, false)
	e.Insert("XY")
	if e.Text() != "heXYllo" || e.Caret != 4 {
		t.Fatalf("text=%q caret=%d", e.Text(), e.Caret)
	}
	e.DeleteBackward()
	if e.Text() != "heXllo" || e.Caret != 3 {
		t.Fatalf("after backspace: %q caret=%d", e.Text(), e.Caret)
	}
	e.DeleteForward()
	if e.Text() != "heXlo" {
		t.Fatalf("after delete: %q", e.Text())
	}
}

func TestEditorSelection(t *testing.T) {
	var e Editor
	e.Insert("hello world")
	e.MoveTo(0, false)
	for range 5 {
		e.Move(1, true) // shift-right ×5
	}
	if e.SelectedText() != "hello" {
		t.Fatalf("selected %q", e.SelectedText())
	}
	e.Insert("goodbye")
	if e.Text() != "goodbye world" {
		t.Fatalf("replace selection: %q", e.Text())
	}

	e.SelectAll()
	e.DeleteBackward()
	if e.Text() != "" || e.Caret != 0 {
		t.Fatalf("select-all delete: %q", e.Text())
	}
}

func TestEditorCollapseDirection(t *testing.T) {
	var e Editor
	e.Insert("abcdef")
	e.MoveTo(1, false)
	e.MoveTo(4, true) // select [1,4)
	e.Move(-1, false) // collapse left
	if e.Caret != 1 || e.HasSelection() {
		t.Fatalf("collapse left: caret=%d", e.Caret)
	}
	e.MoveTo(4, true)
	e.Move(1, false) // collapse right
	if e.Caret != 4 || e.HasSelection() {
		t.Fatalf("collapse right: caret=%d", e.Caret)
	}
}

func TestEditorGraphemes(t *testing.T) {
	var e Editor
	// family emoji (ZWJ sequence, 7 runes) + "a" + combining acute (2 runes)
	e.Insert("👨‍👩‍👧áx")
	e.End(false)
	e.Move(-1, false) // over 'x'
	e.Move(-1, false) // over 'a'+combining as one grapheme
	if e.Caret != 5 {
		t.Fatalf("caret=%d, want 5 (before a+combining)", e.Caret)
	}
	e.DeleteBackward() // deletes whole ZWJ emoji
	if e.Text() != "áx" {
		t.Fatalf("after emoji delete: %q (len %d)", e.Text(), len([]rune(e.Text())))
	}
}

func TestCaretGeometryRoundTrip(t *testing.T) {
	f, err := Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	s := NewShaper(f)
	const str = "Hello, world"
	l := s.Line(str, 16)

	n := len([]rune(str))
	if l.CaretX(0) != 0 {
		t.Fatal("caret 0 must be at x=0")
	}
	if l.CaretX(n) != l.Width {
		t.Fatalf("caret at end = %v, want width %v", l.CaretX(n), l.Width)
	}
	// Monotonic and round-trippable via IndexAt.
	prev := float32(-1)
	for i := 0; i <= n; i++ {
		x := l.CaretX(i)
		if x < prev {
			t.Fatalf("CaretX not monotonic at %d", i)
		}
		prev = x
		if got := l.IndexAt(x + 0.1); got != i && i < n {
			t.Fatalf("IndexAt(CaretX(%d)) = %d", i, got)
		}
	}
	if got := l.IndexAt(-5); got != 0 {
		t.Fatalf("IndexAt before start = %d", got)
	}
	if got := l.IndexAt(l.Width + 100); got != n {
		t.Fatalf("IndexAt past end = %d, want %d", got, n)
	}
}
