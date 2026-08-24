//go:build windows

// Windows capture, over Media Foundation.
//
// MF is a COM API, and COM from Go without CGo means calling through the
// vtable by hand: an interface pointer points at a table of function
// pointers, and a method is an index into it. comCall below is that, and the
// index constants are this file's most dangerous content — a wrong one calls
// a real function with the wrong arguments rather than failing to link.
//
// The pipeline is the shortest one MF offers. A source reader wraps the
// capture device and hands out samples; asking it for RGB32 rather than the
// camera's native format lets MF insert its own converter, which is both
// faster and better tested than anything written here would be. The cost is
// that video processing has to be turned on explicitly, or SetCurrentMediaType
// simply fails on every camera that does not natively produce RGB32 — which is
// most of them.

package camera

import (
	"errors"
	"fmt"
	"image"
	"sync"
	"syscall"
	"unsafe"
)

var (
	mfplat   = syscall.NewLazyDLL("mfplat.dll")
	mf       = syscall.NewLazyDLL("mf.dll")
	mfreadwr = syscall.NewLazyDLL("mfreadwrite.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	oleaut32 = syscall.NewLazyDLL("oleaut32.dll")

	procMFStartup         = mfplat.NewProc("MFStartup")
	procMFCreateAttrs     = mfplat.NewProc("MFCreateAttributes")
	procMFEnumDevices     = mf.NewProc("MFEnumDeviceSources")
	procMFCreateMediaType = mfplat.NewProc("MFCreateMediaType")
	procMFCreateReader    = mfreadwr.NewProc("MFCreateSourceReaderFromMediaSource")
	procCoTaskMemFree     = ole32.NewProc("CoTaskMemFree")
	_                     = oleaut32
)

// COM vtable indices. IUnknown occupies 0..2 in every interface.
const (
	unkRelease = 2

	// IMFAttributes
	attrGetUINT32 = 7
	attrGetUINT64 = 8
	attrSetUINT32 = 21
	attrSetGUID   = 24

	// IMFActivate : IMFAttributes(33)
	activateObject = 33

	// IMFSourceReader : IUnknown(3)
	readerSetStreamSelection  = 4
	readerSetCurrentMediaType = 7
	readerGetCurrentMediaType = 6
	readerReadSample          = 9

	// IMFSample : IMFAttributes(33)
	sampleConvertToContiguous = 41

	// IMFMediaBuffer : IUnknown(3)
	bufferLock   = 3
	bufferUnlock = 4
)

const (
	mfVersion     = 0x00020070 // MF_SDK_VERSION<<16 | MF_API_VERSION
	mfStartupLite = 1

	firstVideoStream = 0xFFFFFFFC
)

type guid struct {
	A uint32
	B uint16
	C uint16
	D [8]byte
}

var (
	devsourceType    = guid{0xc60ac5fe, 0x252a, 0x478f, [8]byte{0xa0, 0xef, 0xbc, 0x8f, 0xa5, 0xf7, 0xca, 0xd3}}
	devsourceVidcap  = guid{0x8ac3587a, 0x4ae7, 0x42d8, [8]byte{0x99, 0xe0, 0x0a, 0x60, 0x13, 0xee, 0xf9, 0x0f}}
	iidMediaSource   = guid{0x279a808d, 0xaec7, 0x40c8, [8]byte{0x9c, 0x6b, 0xa6, 0xb4, 0x92, 0xc7, 0x8a, 0x66}}
	mtMajorType      = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mtSubtype        = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mtFrameSize      = guid{0x1652c33d, 0xd6b2, 0x4012, [8]byte{0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d}}
	mtDefaultStride  = guid{0x644b4e48, 0x1e02, 0x4516, [8]byte{0xb0, 0xeb, 0xc0, 0x1c, 0xa9, 0xd4, 0x9a, 0xc6}}
	mediaTypeVideo   = guid{0x73646976, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	videoFormatRGB32 = guid{0x00000016, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	// Without this the reader refuses any format the camera cannot produce
	// natively, which on most cameras means RGB32 is unavailable.
	enableVideoProcessing = guid{0xfb394f3d, 0xccf1, 0x42ee, [8]byte{0xbb, 0xb3, 0xf9, 0xb8, 0x45, 0xd5, 0x68, 0x1d}}
)

// iface is a COM interface pointer.
//
// unsafe.Pointer rather than uintptr throughout, because COM objects live in
// memory Go's collector does not own: it never moves them and never scans
// them, so holding the address in a pointer is both safe and honest, while
// round-tripping it through uintptr is the unsound conversion vet rejects.
type iface = unsafe.Pointer

// comCall invokes method idx on a COM interface pointer.
func comCall(this iface, idx int, a ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(this)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	args := make([]uintptr, 0, len(a)+1)
	args = append(args, uintptr(this))
	args = append(args, a...)
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

func release(p iface) {
	if p != nil {
		comCall(p, unkRelease)
	}
}

func hr(name string, r uintptr) error {
	if int32(r) < 0 {
		return fmt.Errorf("camera: %s failed (hresult 0x%08x)", name, uint32(r))
	}
	return nil
}

var startOnce struct {
	sync.Once
	err error
}

func startMF() error {
	startOnce.Do(func() {
		r, _, _ := procMFStartup.Call(mfVersion, mfStartupLite)
		startOnce.err = hr("MFStartup", r)
	})
	return startOnce.err
}

// Authorization reports granted without asking.
//
// Windows has a camera privacy setting, but no API to query it that is
// meaningful for a desktop app: a blocked camera is reported by the open
// failing, not by a status. Open is what finds out, and its error is what an
// app should show.
func Authorization() Status { return StatusGranted }

// Capture is a running Media Foundation stream.
type Capture struct {
	frames

	reader iface
	source iface
	done   chan struct{}
}

// Open starts capture on a camera.
//
// Facing is a hint Windows cannot honour: MF reports no orientation, so front
// takes the first enumerated device and back the second if one exists.
func Open(o Options) (*Capture, error) {
	if err := startMF(); err != nil {
		return nil, err
	}
	source, err := activateDevice(o.Facing)
	if err != nil {
		return nil, err
	}
	reader, err := makeReader(source)
	if err != nil {
		release(source)
		return nil, err
	}
	w, h, stride, err := configure(reader)
	if err != nil {
		release(reader)
		release(source)
		return nil, err
	}
	c := &Capture{reader: reader, source: source, done: make(chan struct{})}
	go c.stream(w, h, stride)
	return c, nil
}

func activateDevice(facing Facing) (iface, error) {
	var attrs iface
	r, _, _ := procMFCreateAttrs.Call(uintptr(unsafe.Pointer(&attrs)), 1)
	if err := hr("MFCreateAttributes", r); err != nil {
		return nil, err
	}
	defer release(attrs)

	if r := comCall(attrs, attrSetGUID,
		uintptr(unsafe.Pointer(&devsourceType)),
		uintptr(unsafe.Pointer(&devsourceVidcap))); int32(r) < 0 {
		return nil, hr("SetGUID(source type)", r)
	}

	var list iface
	var count uint32
	r, _, _ = procMFEnumDevices.Call(uintptr(attrs),
		uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&count)))
	if err := hr("MFEnumDeviceSources", r); err != nil {
		return nil, err
	}
	// The list is a CoTaskMem array of IMFActivate*, and every entry has to be
	// released whichever one is chosen.
	defer procCoTaskMemFree.Call(uintptr(list))
	if count == 0 {
		return nil, errors.New("camera: no capture device found")
	}
	devs := unsafe.Slice((*iface)(list), count)
	pick := 0
	if facing == FacingBack && count > 1 {
		pick = 1
	}
	var source iface
	var actErr error
	for i := range devs {
		p := devs[i]
		if i == pick {
			r := comCall(p, activateObject,
				uintptr(unsafe.Pointer(&iidMediaSource)),
				uintptr(unsafe.Pointer(&source)))
			actErr = hr("ActivateObject", r)
		}
		release(p)
	}
	if actErr != nil {
		return nil, actErr
	}
	return source, nil
}

func makeReader(source iface) (iface, error) {
	var attrs iface
	r, _, _ := procMFCreateAttrs.Call(uintptr(unsafe.Pointer(&attrs)), 1)
	if err := hr("MFCreateAttributes", r); err != nil {
		return nil, err
	}
	defer release(attrs)
	// See the note on enableVideoProcessing: without it, asking for RGB32
	// below fails on any camera that does not produce it natively.
	if r := comCall(attrs, attrSetUINT32,
		uintptr(unsafe.Pointer(&enableVideoProcessing)), 1); int32(r) < 0 {
		return nil, hr("SetUINT32(video processing)", r)
	}

	var reader iface
	r, _, _ = procMFCreateReader.Call(uintptr(source), uintptr(attrs), uintptr(unsafe.Pointer(&reader)))
	if err := hr("MFCreateSourceReaderFromMediaSource", r); err != nil {
		return nil, err
	}
	return reader, nil
}

// configure asks for RGB32 and reports the geometry actually granted.
func configure(reader iface) (w, h, stride int, err error) {
	if r := comCall(reader, readerSetStreamSelection, firstVideoStream, 1); int32(r) < 0 {
		return 0, 0, 0, hr("SetStreamSelection", r)
	}

	var mt iface
	r, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mt)))
	if err := hr("MFCreateMediaType", r); err != nil {
		return 0, 0, 0, err
	}
	defer release(mt)
	if r := comCall(mt, attrSetGUID,
		uintptr(unsafe.Pointer(&mtMajorType)),
		uintptr(unsafe.Pointer(&mediaTypeVideo))); int32(r) < 0 {
		return 0, 0, 0, hr("SetGUID(major type)", r)
	}
	if r := comCall(mt, attrSetGUID,
		uintptr(unsafe.Pointer(&mtSubtype)),
		uintptr(unsafe.Pointer(&videoFormatRGB32))); int32(r) < 0 {
		return 0, 0, 0, hr("SetGUID(subtype)", r)
	}
	if r := comCall(reader, readerSetCurrentMediaType, firstVideoStream, 0, uintptr(mt)); int32(r) < 0 {
		return 0, 0, 0, hr("SetCurrentMediaType(RGB32)", r)
	}

	// Read the geometry back rather than assuming it: the reader is entitled
	// to a different size than the camera's default.
	var cur iface
	if r := comCall(reader, readerGetCurrentMediaType, firstVideoStream,
		uintptr(unsafe.Pointer(&cur))); int32(r) < 0 {
		return 0, 0, 0, hr("GetCurrentMediaType", r)
	}
	defer release(cur)

	var size uint64
	if r := comCall(cur, attrGetUINT64,
		uintptr(unsafe.Pointer(&mtFrameSize)),
		uintptr(unsafe.Pointer(&size))); int32(r) < 0 {
		return 0, 0, 0, hr("GetUINT64(frame size)", r)
	}
	w, h = int(size>>32), int(size&0xffffffff)
	if w <= 0 || h <= 0 {
		return 0, 0, 0, fmt.Errorf("camera: reader reported a %dx%d frame", w, h)
	}

	// A negative default stride means the rows arrive bottom-up, which is RGB32's
	// historical default and the reason an otherwise correct capture comes out
	// upside down.
	stride = w * 4
	var ds uint32
	if r := comCall(cur, attrGetUINT32,
		uintptr(unsafe.Pointer(&mtDefaultStride)),
		uintptr(unsafe.Pointer(&ds))); int32(r) >= 0 {
		stride = int(int32(ds))
	}
	return w, h, stride, nil
}

func (c *Capture) stream(w, h, stride int) {
	for {
		select {
		case <-c.done:
			return
		default:
		}
		var streamIdx, flags uint32
		var timestamp int64
		var sample iface
		r := comCall(c.reader, readerReadSample,
			firstVideoStream, 0,
			uintptr(unsafe.Pointer(&streamIdx)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&timestamp)),
			uintptr(unsafe.Pointer(&sample)))
		if int32(r) < 0 {
			return
		}
		// A nil sample is normal: the reader returns one on a stream tick that
		// carried no frame, and treating it as an error ends the stream.
		if sample == nil {
			continue
		}
		c.convert(sample, w, h, stride)
		release(sample)
	}
}

func (c *Capture) convert(sample iface, w, h, stride int) {
	var buf iface
	if r := comCall(sample, sampleConvertToContiguous,
		uintptr(unsafe.Pointer(&buf))); int32(r) < 0 || buf == nil {
		return
	}
	defer release(buf)

	var data unsafe.Pointer
	var maxLen, curLen uint32
	if r := comCall(buf, bufferLock,
		uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&maxLen)),
		uintptr(unsafe.Pointer(&curLen))); int32(r) < 0 || data == nil {
		return
	}
	defer comCall(buf, bufferUnlock)

	abs := stride
	if abs < 0 {
		abs = -abs
	}
	if curLen < uint32(abs*h) {
		return
	}
	src := unsafe.Slice((*byte)(data), abs*h)
	c.frames.deliver(w, h, func(img *image.RGBA) {
		rgb32ToRGBA(src, img, w, h, stride)
	})
}

// rgb32ToRGBA converts Media Foundation's RGB32 rows into RGBA.
//
// A negative stride means the rows arrive bottom-up, which is RGB32's
// historical default and the reason an otherwise correct capture comes out
// upside down.
//
// Split out from convert so the arithmetic can be tested without a camera: it
// is the same shape as the V4L2 converters, indexing a driver-sized buffer
// with a driver-reported stride, and it had the same defect. The buffer length
// was checked once as stride*height while each row was then sliced at w*4, so
// a stride narrower than a row of pixels — which nothing forbids a driver from
// reporting — walked off the end on the last row.
//
// The fix is the per-row bound rather than a corrected stride. A stride too
// narrow to hold a row describes no layout this can recover, so the rows that
// do not fit are skipped instead of being invented.
func rgb32ToRGBA(src []byte, img *image.RGBA, w, h, stride int) {
	abs := stride
	if abs < 0 {
		abs = -abs
	}
	bottomUp := stride < 0
	for y := 0; y < h; y++ {
		sy := y
		if bottomUp {
			sy = h - 1 - y
		}
		off := sy * abs
		if off < 0 || off+w*4 > len(src) {
			continue
		}
		s := src[off : off+w*4]
		d := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w*4; x += 4 {
			// RGB32 is B,G,R,X in memory; the X byte is not alpha.
			d[x+0], d[x+1], d[x+2], d[x+3] = s[x+2], s[x+1], s[x+0], 0xff
		}
	}
}

// Stop ends the stream and releases the device, turning the camera light off.
func (c *Capture) Stop() {
	if !c.stop() {
		return
	}
	close(c.done)
	release(c.reader)
	release(c.source)
	c.reader, c.source = nil, nil
}
