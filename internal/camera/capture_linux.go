//go:build linux && !android

// Linux capture, over V4L2 — ioctls and mmap through golang.org/x/sys/unix,
// so this stays zero-CGo like every other native path here.
//
// V4L2's streaming model is a ring the driver and the program pass buffers
// around: the program hands over empty buffers with QBUF, the driver fills
// them and gives them back through DQBUF, and the program returns each one
// after copying it out. Buffers are mmapped once, so a frame costs one
// conversion and no allocation.
//
// The struct layouts below are the kernel's, not a translation of them. Their
// sizes are load-bearing: the ioctl request numbers encode the size of the
// argument, so a struct that is a byte off is rejected by the kernel rather
// than silently misread — which is the one good property of this interface,
// and the reason the sizes are asserted in a test.

package camera

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/unix"
)

// V4L2 ioctl requests, for the struct sizes below. The size is baked into the
// number, so these constants and those structs must change together.
const (
	vidiocQuerycap  = 0x80685600 // _IOR ('V',  0, v4l2_capability)  104 bytes
	vidiocSFmt      = 0xc0d05605 // _IOWR('V',  5, v4l2_format)      208
	vidiocReqbufs   = 0xc0145608 // _IOWR('V',  8, v4l2_requestbuffers) 20
	vidiocQuerybuf  = 0xc0585609 // _IOWR('V',  9, v4l2_buffer)       88
	vidiocQbuf      = 0xc058560f // _IOWR('V', 15, v4l2_buffer)       88
	vidiocDqbuf     = 0xc0585611 // _IOWR('V', 17, v4l2_buffer)       88
	vidiocStreamon  = 0x40045612 // _IOW ('V', 18, int)
	vidiocStreamoff = 0x40045613 // _IOW ('V', 19, int)
)

const (
	bufTypeVideoCapture = 1
	memoryMMAP          = 1

	capVideoCapture = 0x00000001
	capStreaming    = 0x04000000
	capDeviceCaps   = 0x80000000

	// Four-character codes, little-endian as the kernel packs them.
	pixFmtYUYV  = 0x56595559 // 'YUYV'
	pixFmtMJPEG = 0x47504a4d // 'MJPG'
	pixFmtRGB24 = 0x33424752 // 'RGB3'
	pixFmtBGR24 = 0x33524742 // 'BGR3'
)

type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

type v4l2PixFormat struct {
	Width        uint32
	Height       uint32
	PixelFormat  uint32
	Field        uint32
	BytesPerLine uint32
	SizeImage    uint32
	Colorspace   uint32
	Priv         uint32
	Flags        uint32
	Enc          uint32
	Quantization uint32
	XferFunc     uint32
}

// v4l2Format's union is 200 bytes and 8-aligned, which is why Pix is padded
// out rather than declared alone.
type v4l2Format struct {
	Type uint32
	_    uint32
	Pix  v4l2PixFormat
	_    [200 - 48]byte
}

type v4l2RequestBuffers struct {
	Count        uint32
	Type         uint32
	Memory       uint32
	Capabilities uint32
	Flags        uint8
	Reserved     [3]uint8
}

type v4l2Buffer struct {
	Index     uint32
	Type      uint32
	BytesUsed uint32
	Flags     uint32
	Field     uint32
	_         uint32 // timestamp is 8-aligned
	Timestamp [2]int64
	Timecode  [16]byte
	Sequence  uint32
	Memory    uint32
	Offset    uint64 // union m: offset for MMAP
	Length    uint32
	Reserved2 uint32
	RequestFD int32
	_         uint32
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		if errno == 0 {
			return nil
		}
		if errno == unix.EINTR {
			continue
		}
		return errno
	}
}

// devices lists the capture-capable video nodes, in device order.
//
// /dev/video* includes nodes that are not cameras at all — metadata and output
// devices share the numbering — so each candidate is opened and asked. A
// webcam commonly claims two adjacent numbers, only the first of which
// captures.
func devices() []string {
	all, _ := filepath.Glob("/dev/video*")
	sort.Slice(all, func(i, j int) bool { return devNum(all[i]) < devNum(all[j]) })
	var out []string
	for _, p := range all {
		fd, err := unix.Open(p, unix.O_RDWR|unix.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		var cap v4l2Capability
		err = ioctl(fd, vidiocQuerycap, unsafe.Pointer(&cap))
		unix.Close(fd)
		if err != nil {
			continue
		}
		caps := cap.Capabilities
		if caps&capDeviceCaps != 0 {
			caps = cap.DeviceCaps
		}
		if caps&capVideoCapture != 0 && caps&capStreaming != 0 {
			out = append(out, p)
		}
	}
	return out
}

func devNum(p string) int {
	var n int
	if _, err := fmt.Sscanf(filepath.Base(p), "video%d", &n); err != nil {
		return 1 << 30
	}
	return n
}

// Authorization reports whether the process may use a camera.
//
// Linux has no prompt and no per-app camera permission: access is the file
// permission on the device node, which is why StatusPrompt is never returned
// here. A refusal is almost always the same one — the user is not in the
// "video" group — and it is worth distinguishing from having no camera at
// all, because only one of the two is fixable by the person reading the error.
func Authorization() Status {
	all, _ := filepath.Glob("/dev/video*")
	denied := false
	for _, p := range all {
		fd, err := unix.Open(p, unix.O_RDWR|unix.O_NONBLOCK, 0)
		if err == nil {
			unix.Close(fd)
			return StatusGranted
		}
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			denied = true
		}
	}
	if denied {
		return StatusDenied
	}
	// No device, or none openable for another reason. Granted is the honest
	// answer to "may you": Open is what discovers there is nothing to open.
	return StatusGranted
}

// Capture is a running V4L2 stream.
type Capture struct {
	frames

	fd   int
	bufs [][]byte
	done chan struct{}
}

// Open starts capture on a camera.
//
// Facing is a hint Linux cannot honour properly: V4L2 reports no front/back
// orientation, so it selects by device order — front takes the first capture
// node, back the second if one exists. On a laptop that is right; on a machine
// with two arbitrary webcams it is a guess, and an honest one is not available.
func Open(o Options) (*Capture, error) {
	devs := devices()
	if len(devs) == 0 {
		if Authorization() == StatusDenied {
			return nil, errors.New("camera: /dev/video* exists but is not readable (is this user in the \"video\" group?)")
		}
		return nil, errors.New("camera: no capture device found at /dev/video*")
	}
	path := devs[0]
	if o.Facing == FacingBack && len(devs) > 1 {
		path = devs[1]
	}

	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("camera: opening %s: %w", path, err)
	}
	c := &Capture{fd: fd, done: make(chan struct{})}

	width := o.Width
	if width <= 0 {
		width = 640
	}
	format, w, h, stride, err := c.setFormat(width)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	if err := c.mapBuffers(); err != nil {
		unix.Close(fd)
		return nil, err
	}
	typ := int32(bufTypeVideoCapture)
	if err := ioctl(fd, vidiocStreamon, unsafe.Pointer(&typ)); err != nil {
		c.release()
		return nil, fmt.Errorf("camera: STREAMON: %w", err)
	}
	go c.stream(format, w, h, stride)
	return c, nil
}

// setFormat asks for YUYV and reports what the driver actually granted.
//
// The driver is entitled to answer with a different size or a different pixel
// format entirely, and many do — a camera that only offers MJPEG at the
// requested width will quietly switch. Believing the request rather than the
// reply is how a converter ends up reading JPEG bytes as raw luma.
func (c *Capture) setFormat(width int) (format uint32, w, h, stride int, err error) {
	f := v4l2Format{Type: bufTypeVideoCapture}
	f.Pix.Width = uint32(width)
	f.Pix.Height = uint32(width * 3 / 4)
	f.Pix.PixelFormat = pixFmtYUYV
	if err := ioctl(c.fd, vidiocSFmt, unsafe.Pointer(&f)); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("camera: S_FMT: %w", err)
	}
	switch f.Pix.PixelFormat {
	case pixFmtYUYV, pixFmtMJPEG, pixFmtRGB24, pixFmtBGR24:
	default:
		return 0, 0, 0, 0, fmt.Errorf("camera: driver chose pixel format %s, which this build cannot convert",
			fourCC(f.Pix.PixelFormat))
	}
	return f.Pix.PixelFormat, int(f.Pix.Width), int(f.Pix.Height), int(f.Pix.BytesPerLine), nil
}

func fourCC(v uint32) string {
	b := []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
	for i, c := range b {
		if c < 0x20 || c > 0x7e {
			b[i] = '?'
		}
	}
	return string(b)
}

// bufferCount is the depth of the ring. Four is the usual choice: enough that
// the driver always has somewhere to write while one buffer is being
// converted, few enough that a slow consumer falls behind by a frame or two
// rather than by half a second of stale video.
const bufferCount = 4

func (c *Capture) mapBuffers() error {
	req := v4l2RequestBuffers{Count: bufferCount, Type: bufTypeVideoCapture, Memory: memoryMMAP}
	if err := ioctl(c.fd, vidiocReqbufs, unsafe.Pointer(&req)); err != nil {
		return fmt.Errorf("camera: REQBUFS: %w", err)
	}
	if req.Count < 2 {
		return fmt.Errorf("camera: driver granted only %d buffers", req.Count)
	}
	for i := uint32(0); i < req.Count; i++ {
		b := v4l2Buffer{Index: i, Type: bufTypeVideoCapture, Memory: memoryMMAP}
		if err := ioctl(c.fd, vidiocQuerybuf, unsafe.Pointer(&b)); err != nil {
			return fmt.Errorf("camera: QUERYBUF %d: %w", i, err)
		}
		mem, err := unix.Mmap(c.fd, int64(b.Offset), int(b.Length),
			unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
		if err != nil {
			return fmt.Errorf("camera: mmap buffer %d: %w", i, err)
		}
		c.bufs = append(c.bufs, mem)
		if err := ioctl(c.fd, vidiocQbuf, unsafe.Pointer(&b)); err != nil {
			return fmt.Errorf("camera: QBUF %d: %w", i, err)
		}
	}
	return nil
}

// stream owns the ring until Stop.
func (c *Capture) stream(format uint32, w, h, stride int) {
	fds := []unix.PollFd{{Fd: int32(c.fd), Events: unix.POLLIN}}
	for {
		select {
		case <-c.done:
			return
		default:
		}
		// A timeout rather than a blocking wait, so Stop is noticed by a
		// camera that has gone quiet instead of only by one still delivering.
		n, err := unix.Poll(fds, 200)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}
		b := v4l2Buffer{Type: bufTypeVideoCapture, Memory: memoryMMAP}
		if err := ioctl(c.fd, vidiocDqbuf, unsafe.Pointer(&b)); err != nil {
			if errors.Is(err, unix.EAGAIN) {
				continue
			}
			return
		}
		if int(b.Index) < len(c.bufs) && b.BytesUsed > 0 {
			c.convert(format, c.bufs[b.Index][:b.BytesUsed], w, h, stride)
		}
		// Returned to the driver whatever happened: a buffer kept back is one
		// the ring never gets again, and four mistakes stop the stream dead.
		if err := ioctl(c.fd, vidiocQbuf, unsafe.Pointer(&b)); err != nil {
			return
		}
	}
}

func (c *Capture) convert(format uint32, src []byte, w, h, stride int) {
	switch format {
	case pixFmtYUYV:
		c.frames.deliver(w, h, func(img *image.RGBA) { yuyvToRGBA(src, img, w, h, stride) })
	case pixFmtRGB24:
		c.frames.deliver(w, h, func(img *image.RGBA) { packedToRGBA(src, img, w, h, stride, 0, 1, 2) })
	case pixFmtBGR24:
		c.frames.deliver(w, h, func(img *image.RGBA) { packedToRGBA(src, img, w, h, stride, 2, 1, 0) })
	case pixFmtMJPEG:
		// Each buffer is a whole JPEG. Decoding allocates, unlike the other
		// paths, but a camera that only offers MJPEG leaves no alternative.
		m, err := jpeg.Decode(bytes.NewReader(src))
		if err != nil {
			return
		}
		bnd := m.Bounds()
		c.frames.deliver(bnd.Dx(), bnd.Dy(), func(img *image.RGBA) {
			for y := 0; y < bnd.Dy(); y++ {
				for x := 0; x < bnd.Dx(); x++ {
					r, g, b, _ := m.At(bnd.Min.X+x, bnd.Min.Y+y).RGBA()
					o := y*img.Stride + x*4
					img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] =
						uint8(r>>8), uint8(g>>8), uint8(b>>8), 0xff
				}
			}
		})
	}
}

// yuyvToRGBA converts 4:2:2 packed YUYV, two pixels per four bytes sharing one
// chroma pair. Integer BT.601, matching the Android converter so the same
// camera looks the same on both.
func yuyvToRGBA(src []byte, img *image.RGBA, w, h, stride int) {
	if stride <= 0 {
		stride = w * 2
	}
	for y := 0; y < h; y++ {
		row := y * stride
		if row+w*2 > len(src) {
			return
		}
		d := img.Pix[y*img.Stride:]
		for x := 0; x < w; x += 2 {
			i := row + x*2
			y0, u, y1, v := int(src[i]), int(src[i+1]), int(src[i+2]), int(src[i+3])
			cu, cv := u-128, v-128
			o := x * 4
			writeYUV(d[o:], y0, cu, cv)
			if x+1 < w {
				writeYUV(d[o+4:], y1, cu, cv)
			}
		}
	}
}

func writeYUV(d []byte, y, cu, cv int) {
	yy := 1192 * (y - 16)
	d[0] = clamp8((yy + 1634*cv) >> 10)
	d[1] = clamp8((yy - 833*cv - 400*cu) >> 10)
	d[2] = clamp8((yy + 2066*cu) >> 10)
	d[3] = 0xff
}

func packedToRGBA(src []byte, img *image.RGBA, w, h, stride, r, g, b int) {
	if stride <= 0 {
		stride = w * 3
	}
	for y := 0; y < h; y++ {
		row := y * stride
		if row+w*3 > len(src) {
			return
		}
		s := src[row:]
		d := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			d[x*4+0], d[x*4+1], d[x*4+2], d[x*4+3] = s[x*3+r], s[x*3+g], s[x*3+b], 0xff
		}
	}
}

func clamp8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Stop ends the stream and releases the device, turning the camera light off.
func (c *Capture) Stop() {
	if !c.stop() {
		return
	}
	close(c.done)
	typ := int32(bufTypeVideoCapture)
	_ = ioctl(c.fd, vidiocStreamoff, unsafe.Pointer(&typ))
	c.release()
}

func (c *Capture) release() {
	for _, b := range c.bufs {
		_ = unix.Munmap(b)
	}
	c.bufs = nil
	_ = unix.Close(c.fd)
}
