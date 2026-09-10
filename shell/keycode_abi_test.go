package shell

import "testing"

// KeyCode values are an ABI. The mobile Bridge passes them as integers across
// the FFI boundary and hosts hardcode them, so the enum is append-only and
// never reordered. This pins the values that existed before the alphabet was
// completed, and the first appended one, so an insertion in the middle fails
// here instead of in a host that suddenly reads Shift as Ctrl.
func TestKeyCodeValuesAreStable(t *testing.T) {
	pins := map[string]struct {
		got  KeyCode
		want KeyCode
	}{
		"KeyUnknown": {KeyUnknown, 0},
		"KeyEscape":  {KeyEscape, 4},
		"KeyA":       {KeyA, 12},
		"KeyX":       {KeyX, 15},
		"KeySpace":   {KeySpace, 16},
		"KeyF":       {KeyF, 23},
		"KeyShift":   {KeyShift, 24},
		"KeyCtrl":    {KeyCtrl, 25},
		"Key0":       {Key0, 26},
		"Key9":       {Key9, 35},
		"KeyB":       {KeyB, 36}, // first of the appended letters
		"KeyZ":       {KeyZ, 50},
	}
	for name, p := range pins {
		if p.got != p.want {
			t.Errorf("%s = %d, want %d — KeyCode is append-only; something was inserted or reordered", name, p.got, p.want)
		}
	}
}
