//go:build windows && !js

package desktop

import "testing"

// Button order has to match the macOS and web shells, or a widget reading
// Buttons[0] has to know which platform it is on.
func TestXInputButtonOrder(t *testing.T) {
	for i, tc := range []struct {
		name string
		mask uint16
	}{
		{"A", padA}, {"B", padB}, {"X", padX}, {"Y", padY},
		{"LB", padLeftShoulder}, {"RB", padRightShoulder},
	} {
		g := xinputToGamepad(0, xinputGamepad{Buttons: tc.mask})
		if g.Buttons[i] != 1 {
			t.Errorf("%s should be Buttons[%d]; got %v", tc.name, i, g.Buttons[i])
		}
	}
	// Back/Start and the thumbstick clicks sit after the triggers.
	for idx, tc := range map[int]uint16{
		8: padBack, 9: padStart, 10: padLeftThumb, 11: padRightThumb,
		12: padDPadUp, 13: padDPadDown, 14: padDPadLeft, 15: padDPadRight,
	} {
		g := xinputToGamepad(0, xinputGamepad{Buttons: tc})
		if g.Buttons[idx] != 1 {
			t.Errorf("button mask %#x should be Buttons[%d]", tc, idx)
		}
	}
}

// Triggers are analog on XInput and must be reported as such — the browser
// Gamepad API puts a value on buttons 6 and 7, not just a press.
func TestXInputTriggersAreAnalog(t *testing.T) {
	g := xinputToGamepad(0, xinputGamepad{LeftTrigger: 255, RightTrigger: 128})
	if g.Buttons[6] != 1 {
		t.Errorf("full left trigger = %v, want 1", g.Buttons[6])
	}
	if g.Buttons[7] < 0.49 || g.Buttons[7] > 0.51 {
		t.Errorf("half right trigger = %v, want ~0.5", g.Buttons[7])
	}
}

// XInput's range is asymmetric (-32768..32767), so a full-left push would read
// past -1 without clamping.
func TestXInputAxisRange(t *testing.T) {
	g := xinputToGamepad(0, xinputGamepad{ThumbLX: -32768, ThumbRX: 32767})
	if g.Axes[0] != -1 {
		t.Errorf("full left = %v, want exactly -1", g.Axes[0])
	}
	if g.Axes[2] != 1 {
		t.Errorf("full right = %v, want 1", g.Axes[2])
	}
	if c := xinputToGamepad(0, xinputGamepad{}); c.Axes[0] != 0 {
		t.Errorf("centre = %v, want 0", c.Axes[0])
	}
}

// XInput points Y up; the Gamepad API the other shells report points it down.
func TestXInputAxisYIsNegated(t *testing.T) {
	g := xinputToGamepad(0, xinputGamepad{ThumbLY: 32767, ThumbRY: 32767})
	if g.Axes[1] >= 0 {
		t.Errorf("stick pushed up gave Axes[1] = %v, want negative", g.Axes[1])
	}
	if g.Axes[3] >= 0 {
		t.Errorf("right stick up gave Axes[3] = %v, want negative", g.Axes[3])
	}
}

func TestXInputShape(t *testing.T) {
	g := xinputToGamepad(2, xinputGamepad{})
	if len(g.Buttons) != 16 {
		t.Errorf("got %d buttons, want 16", len(g.Buttons))
	}
	if len(g.Axes) != 4 {
		t.Errorf("got %d axes, want 4", len(g.Axes))
	}
	if !g.Connected {
		t.Error("a polled controller should report Connected")
	}
	if g.ID != "XInput controller 3" {
		t.Errorf("ID = %q, want the 1-based slot", g.ID)
	}
}

// Poll runs every frame, so it must be safe with nothing attached and on a
// machine whose XInput DLL cannot be found at all.
func TestXInputPollSafeWithNothingAttached(t *testing.T) {
	g := windowsGamepads{}
	for range 3 {
		for _, pad := range g.Poll() {
			if len(pad.Buttons) != 16 || len(pad.Axes) != 4 {
				t.Errorf("connected controller has wrong shape: %+v", pad)
			}
		}
	}
}

// Windows always ships one of the XInput DLLs, so failing to resolve
// XInputGetState means the version fallback is wrong — and every other test
// here would pass trivially against a nil proc.
func TestXInputResolves(t *testing.T) {
	if loadXInput() == nil {
		t.Error("XInputGetState not found in xinput1_4, xinput1_3 or xinput9_1_0")
	}
}
