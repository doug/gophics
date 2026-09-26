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
//
// The descriptors are raw ints read with unix.Read, not *os.File. An os.File
// opened with O_NONBLOCK is registered with the runtime poller, and
// os.File.Read answers EAGAIN by parking in the poller until data arrives —
// which is a blocking read by another name, and it stalled the frame loop
// until the controller was touched. Calling Fd() for the ioctls made it
// worse, since that puts the descriptor back into blocking mode. The raw
// syscall returns EAGAIN and nothing else can intervene.

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

// evdev event types and the codes we care about (linux/input-event-codes.h).
//
// The button names are the kernel's, and two of them are traps: BTN_NORTH is
// an alias of BTN_X and BTN_WEST of BTN_Y, so on an Xbox-style pad the
// physically-left X button reports as BTN_NORTH. The standard layout below is
// keyed by label (A, B, X, Y), which is what the macOS and Windows backends
// report, so the aliases are named for the labels here.
const (
	evKey = 0x01
	evAbs = 0x03

	btnSouth   = 0x130 // BTN_GAMEPAD / BTN_A — the marker that a device is a gamepad
	btnEast    = 0x131 // BTN_B
	btnX       = 0x133 // BTN_NORTH
	btnY       = 0x134 // BTN_WEST
	btnTL      = 0x136
	btnTR      = 0x137
	btnTL2     = 0x138
	btnTR2     = 0x139
	btnSelect  = 0x13a
	btnStart   = 0x13b
	btnThumbL  = 0x13d
	btnThumbR  = 0x13e
	btnLast    = 0x13e // BTN_THUMBR
	btnDpadUp  = 0x220
	btnDpadDn  = 0x221
	btnDpadL   = 0x222
	btnDpadR   = 0x223
	absX       = 0x00
	absY       = 0x01
	absZ       = 0x02 // left trigger on xpad and hid-playstation
	absRX      = 0x03
	absRY      = 0x04
	absRZ      = 0x05 // right trigger, likewise
	absHat0X   = 0x10
	absHat0Y   = 0x11
	absHat2X   = 0x14 // right trigger in the kernel's documented gamepad layout
	absHat2Y   = 0x15 // left trigger, likewise
	absLast    = 0x15
	keyMaxBit  = 0x2ff
	stdButtons = 16
	stdAxes    = 4
)

// stdButtonCodes is the evdev code behind each slot of the standard layout
// (shell/gamepad.go) up to the thumb clicks; the d-pad slots 12..15 are
// composed from the hat and the BTN_DPAD_* keys in snapshot, and the trigger
// slots 6 and 7 also take an analog axis when the driver has one.
var stdButtonCodes = [12]uint16{
	btnSouth, btnEast, btnX, btnY,
	btnTL, btnTR, btnTL2, btnTR2,
	btnSelect, btnStart, btnThumbL, btnThumbR,
}

// stdAxisCodes follows the kernel's documented gamepad layout
// (Documentation/input/gamepad.rst): ABS_X/ABS_Y left stick, ABS_RX/ABS_RY
// right stick, with Y positive downwards as the standard layout wants. xpad,
// hid-playstation, hid-nintendo and hid-steam all report this way; hid-sony's
// older DualShock 4 support predates the document and swaps the right stick
// with the triggers, which is the driver's divergence and what a mapping
// database (SDL's) exists to paper over.
var stdAxisCodes = [stdAxes]uint16{absX, absY, absRX, absRY}

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

// openInput opens an event node the way every read here needs it: read-only,
// non-blocking, and closed on exec so a spawned file chooser does not inherit
// the controller.
func openInput(path string) (int, error) {
	for {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == unix.EINTR {
			continue
		}
		return fd, err
	}
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
		fd, err := openInput(path)
		if err != nil {
			continue // no permission, most likely: user not in the input group
		}
		ok := hasGamepadButton(uintptr(fd))
		unix.Close(fd)
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
	fd  int
	id  string
	buf []byte
	rng map[uint16]absInfo // axis code → range, for normalisation
	// btn and axis hold the running state for every code the device
	// advertised; apply ignores codes that are not in them. snapshot reads
	// them through the fixed standard layout rather than in code order, so
	// Buttons[3] is Y whether or not the pad has a BTN_C, and a pad without
	// some button reports a zero in its slot rather than shifting the rest.
	btn  map[uint16]float32
	axis map[uint16]float32
}

func openEvdev(path string) (*evdevDevice, error) {
	fd, err := openInput(path)
	if err != nil {
		return nil, err
	}
	d := &evdevDevice{
		fd: fd, buf: make([]byte, inputEventSize*64),
		rng: map[uint16]absInfo{}, btn: map[uint16]float32{}, axis: map[uint16]float32{},
	}
	ufd := uintptr(fd)
	d.id = deviceName(ufd)

	keyBits := make([]byte, keyMaxBit/8+1)
	if err := ioctlPtr(ufd, ioR('E', 0x20+evKey, uintptr(len(keyBits))), unsafe.Pointer(&keyBits[0])); err == nil {
		for c := btnSouth; c <= btnLast; c++ {
			if bitSet(keyBits, c) {
				d.btn[uint16(c)] = 0
			}
		}
		for c := btnDpadUp; c <= btnDpadR; c++ {
			if bitSet(keyBits, c) {
				d.btn[uint16(c)] = 0
			}
		}
	}
	absBits := make([]byte, absLast/8+1)
	if err := ioctlPtr(ufd, ioR('E', 0x20+evAbs, uintptr(len(absBits))), unsafe.Pointer(&absBits[0])); err == nil {
		for c := 0; c <= absLast; c++ {
			if !bitSet(absBits, c) {
				continue
			}
			d.axis[uint16(c)] = 0
			var info absInfo
			if err := ioctlPtr(ufd, ioR('E', 0x40+c, unsafe.Sizeof(info)), unsafe.Pointer(&info)); err == nil {
				d.rng[uint16(c)] = info
				d.axis[uint16(c)] = normalizeAxis(info, info.Value)
			}
		}
	}
	return d, nil
}

func (d *evdevDevice) Close() {
	if d.fd >= 0 {
		unix.Close(d.fd)
		d.fd = -1
	}
}

// drain reads whatever the kernel has buffered and folds it into the state.
// EAGAIN means "nothing new", which is the common case and not an error.
func (d *evdevDevice) drain() error {
	for {
		n, err := unix.Read(d.fd, d.buf)
		if err != nil {
			switch err {
			case unix.EINTR:
				continue
			case unix.EAGAIN:
				return nil
			}
			return err
		}
		if n <= 0 {
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

// snapshot reads the running state out through the standard layout. A map
// miss is a code the device never advertised, and reads as zero — the slot
// is still there, so a widget reading Buttons[9] for Start finds Start.
func (d *evdevDevice) snapshot() shell.Gamepad {
	g := shell.Gamepad{
		ID: d.id, Connected: true,
		Buttons: make([]float32, stdButtons),
		Axes:    make([]float32, stdAxes),
	}
	for i, c := range stdButtonCodes {
		g.Buttons[i] = d.btn[c]
	}
	// Triggers: a digital BTN_TL2/TR2, an analog ABS_Z/RZ, or the documented
	// ABS_HAT2Y/HAT2X — whichever the driver has, and the largest if several,
	// so an XInput-style pad reports the pull as a value the way the Windows
	// backend does.
	g.Buttons[6] = max(g.Buttons[6], d.unipolar(absZ), d.unipolar(absHat2Y))
	g.Buttons[7] = max(g.Buttons[7], d.unipolar(absRZ), d.unipolar(absHat2X))
	// D-pad: a hat reports -1/0/1 per axis, and some pads have the four keys
	// instead (or as well).
	hx, hy := d.axis[absHat0X], d.axis[absHat0Y]
	g.Buttons[12] = max(d.btn[btnDpadUp], clamp1(-hy))
	g.Buttons[13] = max(d.btn[btnDpadDn], hy)
	g.Buttons[14] = max(d.btn[btnDpadL], clamp1(-hx))
	g.Buttons[15] = max(d.btn[btnDpadR], hx)
	for i, c := range stdAxisCodes {
		g.Axes[i] = d.axis[c]
	}
	return g
}

// unipolar reads a trigger axis as 0..1. The axis was normalised to -1..1
// from the driver's range, so rest (the range's minimum) is -1 and full pull
// is 1; an axis the device does not have reads as its rest value.
func (d *evdevDevice) unipolar(code uint16) float32 {
	v, ok := d.axis[code]
	if !ok {
		return 0
	}
	return (v + 1) / 2
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
