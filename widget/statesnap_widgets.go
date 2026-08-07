package widget

import "encoding/json"

// Snapshottable implementations for the framework's stateful widgets — the
// "where was I" state that state-preserving hot-restart (and persistence,
// deep-linking, etc.) restores. Kept together so the set is easy to audit.
//
// Only durable state is persisted. Transient/animation state is intentionally
// left out, so it resets on restore (you don't want a half-finished swipe or an
// in-flight page transition to come back): Dismissible (mid-swipe offset),
// NetworkImage (image cache — re-fetches), LayoutBuilder (constraint cache),
// Rich (last press point), SelectionArea (runtime registry), OverlayHost
// (ephemeral dialogs/snackbars), and the animation controllers throughout.

// Scroll: the scroll offset. Overscroll/refresh/fade are transient.
type scrollSnap struct {
	Offset float32 `json:"offset"`
}

func (s *scrollState) SaveState() any { return scrollSnap{Offset: s.offset} }
func (s *scrollState) LoadState(d json.RawMessage) {
	var v scrollSnap
	if json.Unmarshal(d, &v) == nil {
		s.offset = v.Offset
	}
}

// LazyList: the scroll offset. Measured heights re-measure on layout.
type lazySnap struct {
	Offset float32 `json:"offset"`
}

func (s *lazyState) SaveState() any { return lazySnap{Offset: s.offset} }
func (s *lazyState) LoadState(d json.RawMessage) {
	var v lazySnap
	if json.Unmarshal(d, &v) == nil {
		s.offset = v.Offset
	}
}

// TextField: content and caret/selection. Set text first — SetText clamps the
// caret/anchor — then restore them exactly.
type textFieldSnap struct {
	Text   string `json:"text"`
	Caret  int    `json:"caret"`
	Anchor int    `json:"anchor"`
}

func (s *textFieldState) SaveState() any {
	return textFieldSnap{Text: s.ed.Text(), Caret: s.ed.Caret(), Anchor: s.ed.Anchor()}
}
func (s *textFieldState) LoadState(d json.RawMessage) {
	var v textFieldSnap
	if json.Unmarshal(d, &v) != nil {
		return
	}
	s.ed.SetText(v.Text)
	s.ed.SetSelection(v.Anchor, v.Caret) // clamps into the new text
}

// SelectableText: the selection anchor/focus (linear rune offsets).
type selectableSnap struct {
	Anchor int `json:"anchor"`
	Focus  int `json:"focus"`
}

func (s *selectableState) SaveState() any {
	if s.anchor == s.focus {
		return nil // no selection — nothing to persist
	}
	return selectableSnap{Anchor: s.anchor, Focus: s.focus}
}
func (s *selectableState) LoadState(d json.RawMessage) {
	var v selectableSnap
	if json.Unmarshal(d, &v) == nil {
		s.anchor, s.focus = v.Anchor, v.Focus
	}
}

// Navigator: the pushed page stack — the primary "where am I" state. Pages are
// live Widget values, so they round-trip through the widget registry
// (RegisterSnapshotType). An unregistered page truncates the restored stack at
// that point (contiguity preserved), so nav depth degrades gracefully rather
// than restoring a broken page.
type navSnap struct {
	Stack []SerializedWidget `json:"stack"`
}

func (s *navState) SaveState() any {
	if len(s.stack) == 0 {
		return nil
	}
	out := make([]SerializedWidget, 0, len(s.stack))
	for _, w := range s.stack {
		b, ok := MarshalWidget(w)
		if !ok {
			break // stop at the first page we can't serialize
		}
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil
	}
	return navSnap{Stack: out}
}
func (s *navState) LoadState(d json.RawMessage) {
	var v navSnap
	if json.Unmarshal(d, &v) != nil {
		return
	}
	s.stack = s.stack[:0]
	for _, b := range v.Stack {
		w, ok := UnmarshalWidget(b)
		if !ok {
			break
		}
		s.stack = append(s.stack, w)
	}
}

func clampInt(i, max int) int {
	if i < 0 {
		return 0
	}
	if i > max {
		return max
	}
	return i
}
