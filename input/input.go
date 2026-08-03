// Package input is per-frame, poll-style input state for games: which keys are
// held right now, which were pressed/released this frame, and the pointer. It is
// fed from shell events by the app runner (HandleKey/HandlePointer/NewFrame) and
// read by widgets via Ctx.Input(). It depends only on shell and geom.
//
// Held state is global — it works whether or not any widget has focus, so a
// game canvas can poll WASD without a focused text field. The idiom
// `if in.TextCapturing() { return }` lets a game ignore movement keys while the
// user is typing into a focused field.
package input

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// State is the current input snapshot. Not safe for concurrent use; touched only
// on the UI goroutine.
type State struct {
	down     map[shell.KeyCode]bool // currently held
	pressed  map[shell.KeyCode]bool // went down this frame (sticky)
	released map[shell.KeyCode]bool // went up this frame (sticky)
	mods     shell.Mods
	pointer  geom.Pt
	buttons  uint8 // bit b set → button b held
	pressBtn uint8 // pressed this frame
	text     bool  // a focused field owns the keyboard
}

// New returns an empty State.
func New() *State {
	return &State{
		down:     map[shell.KeyCode]bool{},
		pressed:  map[shell.KeyCode]bool{},
		released: map[shell.KeyCode]bool{},
	}
}

// Down reports whether k is currently held.
func (s *State) Down(k shell.KeyCode) bool { return s.down[k] }

// JustPressed reports whether k went down during this frame.
func (s *State) JustPressed(k shell.KeyCode) bool { return s.pressed[k] }

// JustReleased reports whether k went up during this frame.
func (s *State) JustReleased(k shell.KeyCode) bool { return s.released[k] }

// Axis returns +1 if pos is held, -1 if neg is held, 0 if neither or both — the
// WASD/arrow movement helper.
func (s *State) Axis(neg, pos shell.KeyCode) float32 {
	var v float32
	if s.down[pos] {
		v++
	}
	if s.down[neg] {
		v--
	}
	return v
}

// Mods returns the active keyboard modifiers.
func (s *State) Mods() shell.Mods { return s.mods }

// Pointer returns the last pointer position (logical pixels).
func (s *State) Pointer() geom.Pt { return s.pointer }

// PointerDown reports whether the given pointer button (0=primary) is held.
func (s *State) PointerDown(button uint8) bool { return s.buttons&(1<<button) != 0 }

// PointerJustPressed reports whether the button went down this frame.
func (s *State) PointerJustPressed(button uint8) bool { return s.pressBtn&(1<<button) != 0 }

// TextCapturing reports whether a focused field currently owns the keyboard, so
// a game can ignore movement keys while typing.
func (s *State) TextCapturing() bool { return s.text }

// --- runner side (not for widgets) ---

// HandleKey folds a key event into the held/edge state.
func (s *State) HandleKey(k shell.Key) {
	s.mods = k.Mods
	switch k.Kind {
	case shell.KeyPress:
		if !s.down[k.Code] { // sticky edge; survives a same-frame down+up
			s.pressed[k.Code] = true
		}
		s.down[k.Code] = true
	case shell.KeyRelease:
		if s.down[k.Code] {
			s.released[k.Code] = true
		}
		s.down[k.Code] = false
	}
}

// HandlePointer folds a pointer event into the pointer/button state.
func (s *State) HandlePointer(p shell.Pointer) {
	s.pointer = p.Pos
	switch p.Kind {
	case shell.PointerDown:
		s.buttons |= 1 << p.Button
		s.pressBtn |= 1 << p.Button
	case shell.PointerUp:
		s.buttons &^= 1 << p.Button
	}
}

// SetTextCapturing records whether a focused field owns the keyboard.
func (s *State) SetTextCapturing(b bool) { s.text = b }

// NewFrame clears the per-frame edge sets. The runner calls it once per frame,
// after the frame has read the state.
func (s *State) NewFrame() {
	clear(s.pressed)
	clear(s.released)
	s.pressBtn = 0
}

// Clear drops all held state — the runner calls it on focus loss so keys held
// when the window blurred don't stay stuck.
func (s *State) Clear() {
	clear(s.down)
	clear(s.pressed)
	clear(s.released)
	s.buttons = 0
	s.pressBtn = 0
}
