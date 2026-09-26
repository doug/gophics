package text

import "testing"

func ed(s string, caret int) *Editor {
	e := &Editor{}
	e.SetText(s)
	e.SetSelection(caret, caret)
	e.undo, e.redo, e.lastOp = nil, nil, opNone // SetText is not a user edit here
	return e
}

func TestMoveWord(t *testing.T) {
	e := ed("one two, three", 14)
	e.MoveWord(-1, false)
	if e.Caret() != 9 {
		t.Errorf("word left from end: caret %d, want 9 (start of 'three')", e.Caret())
	}
	e.MoveWord(-1, false)
	if e.Caret() != 4 {
		t.Errorf("word left again: caret %d, want 4 (start of 'two', past the comma)", e.Caret())
	}
	e.MoveWord(1, false)
	if e.Caret() != 7 {
		t.Errorf("word right: caret %d, want 7 (end of 'two')", e.Caret())
	}
	// Extending builds a selection; collapsing without extend lands on the edge.
	e.MoveWord(1, true)
	if s, en := e.Selection(); s != 7 || en != 14 {
		t.Errorf("shift+word right selected [%d,%d), want [7,14)", s, en)
	}
	e.MoveWord(-1, false)
	if e.HasSelection() || e.Caret() != 7 {
		t.Errorf("word left with a selection should collapse to its start: caret %d sel=%v", e.Caret(), e.HasSelection())
	}
}

func TestDeleteWord(t *testing.T) {
	e := ed("alpha beta gamma", 16)
	e.DeleteWordBackward()
	if e.Text() != "alpha beta " {
		t.Errorf("after deleting a word backward: %q", e.Text())
	}
	e = ed("alpha beta gamma", 0)
	e.DeleteWordForward()
	if e.Text() != " beta gamma" {
		t.Errorf("after deleting a word forward: %q", e.Text())
	}
}

func TestLineMovementAndDeletion(t *testing.T) {
	e := ed("first line\nsecond line\nthird", 15) // inside "second"
	e.LineHome(false)
	if e.Caret() != 11 {
		t.Errorf("LineHome: caret %d, want 11", e.Caret())
	}
	e.LineEnd(false)
	if e.Caret() != 22 {
		t.Errorf("LineEnd: caret %d, want 22", e.Caret())
	}
	e.SetSelection(15, 15)
	e.DeleteToLineStart()
	if e.Text() != "first line\nnd line\nthird" {
		t.Errorf("DeleteToLineStart: %q", e.Text())
	}
	e = ed("first\nsecond", 5) // at the end of "first"
	e.DeleteToLineEnd()        // nothing left on the line: eats the newline
	if e.Text() != "firstsecond" {
		t.Errorf("Ctrl+K at end of line should join lines: %q", e.Text())
	}
	e = ed("first\nsecond", 2)
	e.DeleteToLineEnd()
	if e.Text() != "fi\nsecond" {
		t.Errorf("Ctrl+K mid-line: %q", e.Text())
	}
}

func TestSelectLineAt(t *testing.T) {
	e := ed("a\nbcd\ne", 3)
	e.SelectLineAt(3)
	if e.SelectedText() != "bcd" {
		t.Errorf("triple-click selected %q, want the line", e.SelectedText())
	}
}

// Typing coalesces into one undo step; a deletion or a paste is its own.
func TestUndoGroupsTypingLikeANativeField(t *testing.T) {
	e := ed("", 0)
	for _, r := range "hello" {
		e.Insert(string(r))
	}
	e.Insert(" ")
	e.Insert("w")
	if !e.Undo() || e.Text() != "" {
		t.Fatalf("one undo should take back the whole typed run, got %q", e.Text())
	}
	if !e.Redo() || e.Text() != "hello w" {
		t.Fatalf("redo should restore it, got %q", e.Text())
	}

	e = ed("", 0)
	e.Insert("ab")
	e.DeleteBackward()
	e.Insert("c")
	e.Undo()
	if e.Text() != "a" {
		t.Errorf("undo after type/delete/type should revert the last type: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "ab" {
		t.Errorf("second undo should revert the delete: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "" {
		t.Errorf("third undo should revert the first typing: %q", e.Text())
	}
	if e.Undo() {
		t.Error("nothing left to undo")
	}
}

// Moving the caret breaks the typing group: "ab", move, "c" is two steps.
func TestUndoBoundaryOnCaretMove(t *testing.T) {
	e := ed("", 0)
	e.Insert("ab")
	e.MoveTo(0, false)
	e.Insert("c")
	e.Undo()
	if e.Text() != "ab" {
		t.Errorf("undo after a caret move should revert only the second run: %q", e.Text())
	}
}

// A paste is one step and never merges with typing around it.
func TestReplaceIsItsOwnUndoStep(t *testing.T) {
	e := ed("", 0)
	e.Insert("x")
	e.Replace("PASTE")
	e.Insert("y")
	e.Undo()
	if e.Text() != "xPASTE" {
		t.Errorf("undo should revert the typing after the paste: %q", e.Text())
	}
	e.Undo()
	if e.Text() != "x" {
		t.Errorf("undo should revert the paste on its own: %q", e.Text())
	}
}

// A new edit after undo discards the redo stack, as everywhere.
func TestEditAfterUndoClearsRedo(t *testing.T) {
	e := ed("", 0)
	e.Insert("a")
	e.Undo()
	e.Insert("b")
	if e.Redo() {
		t.Error("redo survived a new edit")
	}
}

// Windows word-right lands at the start of the next word; the Mac lands at the
// end of the current one. Same text, two conventions, both by request.
func TestMoveWordStartIsTheWindowsConvention(t *testing.T) {
	e := ed("one two, three", 0)
	e.MoveWordStart(false)
	if e.Caret() != 4 {
		t.Errorf("Ctrl+Right from 0: caret %d, want 4 (start of 'two')", e.Caret())
	}
	e.MoveWordStart(false)
	if e.Caret() != 9 {
		t.Errorf("Ctrl+Right again: caret %d, want 9 (start of 'three', past the comma)", e.Caret())
	}
	e.MoveWordStart(false)
	if e.Caret() != 14 {
		t.Errorf("Ctrl+Right at the last word: caret %d, want the end", e.Caret())
	}
}

// Typing over a selection must not coalesce with the typing before it:
// SelectAll leaves the caret where typing ended, so the caret check alone
// merged the replacement into the previous group and Undo skipped the state
// that still had the text.
func TestTypingOverASelectionIsItsOwnUndoStep(t *testing.T) {
	var e Editor
	e.Insert("hello")
	e.SelectAll()
	e.Insert("x")
	if e.Text() != "x" {
		t.Fatalf("typing over the selection gave %q", e.Text())
	}
	if !e.Undo() || e.Text() != "hello" {
		t.Errorf("Undo gave %q, want %q", e.Text(), "hello")
	}
}

// Inserting nothing with nothing selected is not an edit and not an undo
// step, the same as a no-op delete.
func TestEmptyInsertIsNotAnUndoStep(t *testing.T) {
	var e Editor
	e.Insert("a")
	e.MoveTo(0, false)
	e.Insert("")
	e.Replace("")
	if !e.Undo() || e.Text() != "" {
		t.Fatalf("after Undo: %q, want empty (the no-op inserts must not push history)", e.Text())
	}
	if e.Undo() {
		t.Error("a second Undo found history that no edit made")
	}
}

// Combining marks and joiners are inside the grapheme of the letter before
// them, so word movement, word deletion and double-click must not stop
// between a letter and its accent, a consonant and its vowel sign, or the
// halves of a ZWJ-joined conjunct.
func TestWordOpsKeepCombiningMarksWithTheirLetter(t *testing.T) {
	e := ed("café bar", 0) // e + combining acute
	e.MoveWord(1, false)
	if e.Caret() != 5 {
		t.Errorf("word right over café: caret %d, want 5 (after the accent)", e.Caret())
	}
	e = ed("नमस्ते x", 0)
	e.SelectWordAt(0)
	if got := e.SelectedText(); got != "नमस्ते" {
		t.Errorf("double-click on नमस्ते selected %q", got)
	}
	e = ed("नमस्ते x", 0)
	e.DeleteWordForward()
	if got := e.Text(); got != " x" {
		t.Errorf("delete word forward over नमस्ते left %q, want %q", got, " x")
	}
	e = ed("क्‍ष a", 0) // ZWJ inside a conjunct
	e.MoveWord(1, false)
	if e.Caret() != 4 {
		t.Errorf("word right over a ZWJ conjunct: caret %d, want 4", e.Caret())
	}
}

// Double-clicking a non-word character selects the grapheme there, not one
// rune of it: an emoji with a skin-tone modifier is two runes.
func TestSelectWordAtOnAGraphemeSelectsAllOfIt(t *testing.T) {
	e := ed("👍🏽 a", 0)
	e.SelectWordAt(0)
	if s, en := e.Selection(); s != 0 || en != 2 {
		t.Errorf("SelectWordAt(0) on 👍🏽 selected [%d,%d), want [0,2)", s, en)
	}
	e.SelectWordAt(1) // inside the grapheme
	if s, en := e.Selection(); s != 0 || en != 2 {
		t.Errorf("SelectWordAt(1) inside 👍🏽 selected [%d,%d), want [0,2)", s, en)
	}
	e = ed("a 👍🏽", 4)
	e.SelectWordAt(4) // at the end
	if s, en := e.Selection(); s != 2 || en != 4 {
		t.Errorf("SelectWordAt at the end selected [%d,%d), want [2,4)", s, en)
	}
}
