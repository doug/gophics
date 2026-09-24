//go:build linux && !android && !js

package desktop

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// event builds one struct input_event as the kernel would write it.
func event(typ, code uint16, value int32) []byte {
	b := make([]byte, inputEventSize)
	// The leading 16 bytes are the timeval, which the decoder ignores; leave
	// them zero rather than pretending they carry information.
	binary.LittleEndian.PutUint16(b[16:18], typ)
	binary.LittleEndian.PutUint16(b[18:20], code)
	binary.LittleEndian.PutUint32(b[20:24], uint32(value))
	return b
}

// newTestDevice builds a device with one button and one stick axis, ranged the
// way a real controller reports.
func newTestDevice() *evdevDevice {
	d := &evdevDevice{
		fd:   -1, // no descriptor; the zero value would name stdin
		rng:  map[uint16]absInfo{},
		btn:  map[uint16]float32{},
		axis: map[uint16]float32{},
	}
	d.btnCodes = []uint16{btnSouth}
	d.btn[btnSouth] = 0
	d.axisCodes = []uint16{0} // ABS_X
	d.axis[0] = 0
	d.rng[0] = absInfo{Minimum: -32768, Maximum: 32767}
	return d
}

func TestApplyButtonPressAndRelease(t *testing.T) {
	d := newTestDevice()

	d.apply(event(evKey, btnSouth, 1))
	if got := d.snapshot().Buttons[0]; got != 1 {
		t.Errorf("after press, button = %v, want 1", got)
	}
	d.apply(event(evKey, btnSouth, 0))
	if got := d.snapshot().Buttons[0]; got != 0 {
		t.Errorf("after release, button = %v, want 0", got)
	}
}

// evdev sends value 2 for auto-repeat while a key is held. Treating it as
// anything but "still pressed" makes a held button flicker.
func TestApplyAutorepeatCountsAsHeld(t *testing.T) {
	d := newTestDevice()
	d.apply(event(evKey, btnSouth, 2))
	if got := d.snapshot().Buttons[0]; got != 1 {
		t.Errorf("autorepeat gave %v, want 1", got)
	}
}

// The whole reason for using evdev over joydev: the driver's range is what
// makes full deflection read as 1.0.
func TestAxisNormalisation(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  int32
		want float32
	}{
		{"centre", 0, 0},
		{"full right", 32767, 1},
		{"full left", -32768, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDevice()
			d.apply(event(evAbs, 0, tc.raw))
			got := d.snapshot().Axes[0]
			if diff := got - tc.want; diff > 0.001 || diff < -0.001 {
				t.Errorf("axis = %v, want %v", got, tc.want)
			}
		})
	}
}

// A driver that reports no range (a hat switch) already speaks -1/0/1, so the
// value passes through rather than being divided by a zero span.
func TestAxisWithoutRangePassesThrough(t *testing.T) {
	d := newTestDevice()
	d.axisCodes = append(d.axisCodes, 0x10) // ABS_HAT0X
	d.axis[0x10] = 0
	d.apply(event(evAbs, 0x10, -1))
	if got := d.snapshot().Axes[1]; got != -1 {
		t.Errorf("hat axis = %v, want -1", got)
	}
}

// Out-of-range values must clamp: a stick that overshoots its declared range
// should not report 1.4 and push a UI past its extent.
func TestAxisClamped(t *testing.T) {
	d := newTestDevice()
	d.apply(event(evAbs, 0, 999999))
	if got := d.snapshot().Axes[0]; got != 1 {
		t.Errorf("axis = %v, want it clamped to 1", got)
	}
}

// Several events arrive per read; all of them must land, not just the first.
func TestApplyMultipleEventsInOneBuffer(t *testing.T) {
	d := newTestDevice()
	var buf []byte
	buf = append(buf, event(evKey, btnSouth, 1)...)
	buf = append(buf, event(evAbs, 0, 32767)...)
	d.apply(buf)

	s := d.snapshot()
	if s.Buttons[0] != 1 {
		t.Errorf("button = %v, want 1", s.Buttons[0])
	}
	if diff := s.Axes[0] - 1; diff > 0.001 || diff < -0.001 {
		t.Errorf("axis = %v, want 1", s.Axes[0])
	}
}

// Codes the device never advertised must be ignored rather than growing the
// state and shifting every index in the reported slice.
func TestApplyIgnoresUnknownCodes(t *testing.T) {
	d := newTestDevice()
	d.apply(event(evKey, 0x131, 1)) // BTN_EAST, not advertised
	d.apply(event(evAbs, 5, 100))   // ABS_RZ, not advertised
	s := d.snapshot()
	if len(s.Buttons) != 1 || len(s.Axes) != 1 {
		t.Errorf("state grew: %d buttons, %d axes, want 1 and 1", len(s.Buttons), len(s.Axes))
	}
}

// A truncated tail (a partial record at the end of a read) must not panic.
func TestApplyIgnoresPartialRecord(t *testing.T) {
	d := newTestDevice()
	buf := append(event(evKey, btnSouth, 1), 0x01, 0x02, 0x03)
	d.apply(buf)
	if got := d.snapshot().Buttons[0]; got != 1 {
		t.Errorf("button = %v, want the complete record still applied", got)
	}
}

// The ioctl request encoding is worth pinning: a wrong direction or size bit
// makes every ioctl fail, and the fallback is silence — no gamepads found.
func TestIoctlRequestEncoding(t *testing.T) {
	// EVIOCGNAME(256) is 0x81004506 on 64-bit Linux.
	if got := ioR('E', 0x06, 256); got != 0x81004506 {
		t.Errorf("EVIOCGNAME(256) = %#x, want 0x81004506", got)
	}
	// EVIOCGBIT(EV_KEY, 96) is 0x80604521.
	if got := ioR('E', 0x20+evKey, 96); got != 0x80604521 {
		t.Errorf("EVIOCGBIT(EV_KEY,96) = %#x, want 0x80604521", got)
	}
}

// With no /dev/input at all, discovery reports nothing rather than failing.
func TestFindGamepadsWithoutDevInput(t *testing.T) {
	old := devInputDir
	devInputDir = t.TempDir()
	t.Cleanup(func() { devInputDir = old })
	if got := findGamepads(); len(got) != 0 {
		t.Errorf("findGamepads() = %v on an empty tree", got)
	}
}

// Poll runs every frame, and a controller that nobody is touching is the
// common case. The device is opened non-blocking so that reads return EAGAIN
// then — but an *os.File opened with O_NONBLOCK is pollable, and its Read
// turns EAGAIN into a wait in the runtime poller, so drain blocked the frame
// loop until the controller emitted an event. A FIFO opened by the same code
// path as an event node shows the difference: with a writer attached and
// nothing written, a correct drain returns at once.
func TestDrainReturnsAtOnceWithNothingPending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event0")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	// The real open: ioctls fail with ENOTTY on a FIFO, so the device comes
	// back with no advertised buttons, which is fine — the read path is what
	// is under test.
	d, err := openEvdev(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	// A writer has to exist or every read reports EOF rather than "nothing
	// yet"; opening it blocks until the reader above is open, which it is.
	w, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	done := make(chan error, 1)
	go func() { done <- d.drain() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("drain reported %v with nothing pending; EAGAIN is not an error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("drain blocked for 2s on a device with nothing to read — " +
			"every frame would wait for the controller to be touched")
	}

	// And events that are pending are folded in on the next drain.
	d.btnCodes = []uint16{btnSouth}
	d.btn[btnSouth] = 0
	if _, err := w.Write(event(evKey, btnSouth, 1)); err != nil {
		t.Fatal(err)
	}
	if err := d.drain(); err != nil {
		t.Fatal(err)
	}
	if got := d.snapshot().Buttons[0]; got != 1 {
		t.Errorf("after a pending press, button = %v, want 1", got)
	}
}

// Poll must be safe with nothing attached — a game calls it every frame.
func TestPollWithNoDevices(t *testing.T) {
	old := devInputDir
	devInputDir = t.TempDir()
	t.Cleanup(func() { devInputDir = old })
	g := &linuxGamepads{}
	if got := g.Poll(); len(got) != 0 {
		t.Errorf("Poll() = %v, want empty", got)
	}
	g.Poll() // second call must not double-open or panic
}
