//go:build linux && !android && !js

// Linux implementation of the shell gamepad capability (shell/gamepad.go) over
// evdev — /dev/input/event*, read directly.
//
// evdev rather than the older joydev (/dev/input/js*): joydev is a compatibility
// layer that modern kernels build optionally, and it reports axes without their
// ranges, so normalising to -1..1 means guessing. evdev gives the range per axis
// through EVIOCGABS, which is the difference between a stick that reads 1.0 at
// full deflection and one that reads 0.53.
//
// The capability is poll-style but the kernel interface is a stream, so each
// device keeps the running state its events have built up and Poll() snapshots
// it. Reads are non-blocking: Poll must never stall a frame because nobody
// touched the controller.

package desktop

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/doug/gophics/shell"
)

// inputEventSize is sizeof(struct input_event) on 64-bit Linux: a 16-byte
// timeval, two u16 and an s32.
const inputEventSize = 24

// evdev event types and the code ranges we care about (linux/input-event-codes.h).
const (
	evKey = 0x01
	evAbs = 0x03

	btnSouth  = 0x130 // BTN_GAMEPAD — the marker that a device is a gamepad
	btnLast   = 0x13e // BTN_THUMBR
	absLast   = 0x11  // ABS_HAT0Y
	keyMaxBit = 0x2ff
)

// devInputDir is a var so tests can point discovery at a fake tree.
var devInputDir = "/dev/input"

// Gamepads satisfies shell.GamepadWindow for the Linux desktop shell.
func (w *window) Gamepads() shell.Gamepads { return &linuxGamepads{} }

type linuxGamepads struct {
	mu   sync.Mutex
	open map[string]*evdevDevice
}

// Poll rescans for hotplugged controllers, drains each device's pending events
// and returns the resulting state.
func (g *linuxGamepads) Poll() []shell.Gamepad {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open == nil {
		g.open = map[string]*evdevDevice{}
	}

	// Rescan every poll. Reading a directory of a dozen entries is cheap next
	// to a frame, and it is the only way to notice a controller plugged in
	// after start without a udev/netlink listener.
	paths := findGamepads()
	for _, p := range paths {
		if _, ok := g.open[p]; !ok {
			if d, err := openEvdev(p); err == nil {
				g.open[p] = d
			}
		}
	}

	out := make([]shell.Gamepad, 0, len(g.open))
	for p, d := range g.open {
		if err := d.drain(); err != nil {
			// Unplugged mid-session: drop it rather than reporting stale state.
			d.Close()
			delete(g.open, p)
			continue
		}
		out = append(out, d.snapshot())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// findGamepads lists the event devices that advertise BTN_SOUTH, which is how
// a gamepad distinguishes itself from the keyboards, mice, lid switches and
// power buttons that share this directory.
func findGamepads() []string {
	entries, err := os.ReadDir(devInputDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if len(name) < 6 || name[:5] != "event" {
			continue
		}
		path := filepath.Join(devInputDir, name)
		f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			continue // no permission, most likely: user not in the input group
		}
		ok := hasGamepadButton(f.Fd())
		f.Close()
		if ok {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// hasGamepadButton asks the driver for its key bitmap and tests BTN_SOUTH.
func hasGamepadButton(fd uintptr) bool {
	bits := make([]byte, keyMaxBit/8+1)
	// EVIOCGBIT(EV_KEY, len)
	req := ioR('E', 0x20+evKey, uintptr(len(bits)))
	if err := ioctlPtr(fd, req, unsafe.Pointer(&bits[0])); err != nil {
		return false
	}
	return bitSet(bits, btnSouth)
}

func bitSet(bits []byte, bit int) bool {
	i := bit / 8
	return i < len(bits) && bits[i]&(1<<(bit%8)) != 0
}

// absInfo mirrors struct input_absinfo.
type absInfo struct {
	Value, Minimum, Maximum, Fuzz, Flat, Resolution int32
}

type evdevDevice struct {
	f    *os.File
	id   string
	buf  []byte
	rng  map[uint16]absInfo // axis code → range, for normalisation
	btn  map[uint16]float32
	axis map[uint16]float32
	// codes fix the reported order, so Buttons[3] means the same thing on
	// every poll rather than moving as a map iterates.
	btnCodes, axisCodes []uint16
}

func openEvdev(path string) (*evdevDevice, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	d := &evdevDevice{
		f: f, buf: make([]byte, inputEventSize*64),
		rng: map[uint16]absInfo{}, btn: map[uint16]float32{}, axis: map[uint16]float32{},
	}
	d.id = deviceName(f.Fd())

	keyBits := make([]byte, keyMaxBit/8+1)
	if err := ioctlPtr(f.Fd(), ioR('E', 0x20+evKey, uintptr(len(keyBits))), unsafe.Pointer(&keyBits[0])); err == nil {
		for c := btnSouth; c <= btnLast; c++ {
			if bitSet(keyBits, c) {
				d.btnCodes = append(d.btnCodes, uint16(c))
				d.btn[uint16(c)] = 0
			}
		}
	}
	absBits := make([]byte, absLast/8+1)
	if err := ioctlPtr(f.Fd(), ioR('E', 0x20+evAbs, uintptr(len(absBits))), unsafe.Pointer(&absBits[0])); err == nil {
		for c := 0; c <= absLast; c++ {
			if !bitSet(absBits, c) {
				continue
			}
			d.axisCodes = append(d.axisCodes, uint16(c))
			d.axis[uint16(c)] = 0
			var info absInfo
			if err := ioctlPtr(f.Fd(), ioR('E', 0x40+c, unsafe.Sizeof(info)), unsafe.Pointer(&info)); err == nil {
				d.rng[uint16(c)] = info
				d.axis[uint16(c)] = normalizeAxis(info, info.Value)
			}
		}
	}
	return d, nil
}

func (d *evdevDevice) Close() { d.f.Close() }

// drain reads whatever the kernel has buffered and folds it into the state.
// EAGAIN means "nothing new", which is the common case and not an error.
func (d *evdevDevice) drain() error {
	for {
		n, err := d.f.Read(d.buf)
		if err != nil {
			if err == unix.EAGAIN || os.IsTimeout(err) {
				return nil
			}
			if pe, ok := err.(*os.PathError); ok && pe.Err == unix.EAGAIN {
				return nil
			}
			return err
		}
		if n == 0 {
			return nil
		}
		d.apply(d.buf[:n])
		if n < len(d.buf) {
			return nil
		}
	}
}

// apply folds a buffer of input_event records into the running state. Split out
// so it can be tested with bytes rather than hardware.
func (d *evdevDevice) apply(b []byte) {
	for len(b) >= inputEventSize {
		rec := b[:inputEventSize]
		b = b[inputEventSize:]
		// The timeval occupies the first 16 bytes; the payload follows.
		typ := binary.LittleEndian.Uint16(rec[16:18])
		code := binary.LittleEndian.Uint16(rec[18:20])
		val := int32(binary.LittleEndian.Uint32(rec[20:24]))
		switch typ {
		case evKey:
			if _, ok := d.btn[code]; ok {
				// Value 2 is auto-repeat, which for a button means still held.
				if val != 0 {
					d.btn[code] = 1
				} else {
					d.btn[code] = 0
				}
			}
		case evAbs:
			if _, ok := d.axis[code]; ok {
				d.axis[code] = normalizeAxis(d.rng[code], val)
			}
		}
	}
}

// normalizeAxis maps a raw axis value onto -1..1 using the driver's reported
// range. A range the driver did not give (both bounds zero) passes the value
// through clamped, which is right for hat switches: they report -1, 0 or 1
// already.
func normalizeAxis(info absInfo, v int32) float32 {
	if info.Maximum == info.Minimum {
		return clamp1(float32(v))
	}
	span := float32(info.Maximum - info.Minimum)
	// Map [min,max] to [-1,1].
	return clamp1(2*(float32(v)-float32(info.Minimum))/span - 1)
}

func clamp1(v float32) float32 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

func (d *evdevDevice) snapshot() shell.Gamepad {
	g := shell.Gamepad{ID: d.id, Connected: true}
	for _, c := range d.btnCodes {
		g.Buttons = append(g.Buttons, d.btn[c])
	}
	for _, c := range d.axisCodes {
		g.Axes = append(g.Axes, d.axis[c])
	}
	return g
}

// deviceName reads EVIOCGNAME, falling back to the device path's basename.
func deviceName(fd uintptr) string {
	buf := make([]byte, 256)
	if err := ioctlPtr(fd, ioR('E', 0x06, uintptr(len(buf))), unsafe.Pointer(&buf[0])); err != nil {
		return "gamepad"
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// ioR builds a _IOR ioctl request number. Encoding is from asm-generic/ioctl.h:
// direction in the top 2 bits, then size, type and number.
func ioR(typ, nr int, size uintptr) uint {
	const (
		read      = 2
		nrBits    = 8
		typeBits  = 8
		sizeBits  = 14
		nrShift   = 0
		typeShift = nrShift + nrBits
		sizeShift = typeShift + typeBits
		dirShift  = sizeShift + sizeBits
	)
	return uint(read<<dirShift) | uint(size)<<sizeShift | uint(typ)<<typeShift | uint(nr)<<nrShift
}

func ioctlPtr(fd uintptr, req uint, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, uintptr(req), uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
