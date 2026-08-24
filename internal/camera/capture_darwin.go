//go:build darwin && !ios

// macOS capture, over AVFoundation driven from Go through the Objective-C
// bridge in internal/objc — zero CGo, like every other native path here.
//
// The shape is the one AVFoundation dictates: a capture session owns a device
// input and a video data output, and the output pushes each frame to a delegate
// on a queue of its choosing. The delegate is a class defined at runtime whose
// one method is a Go function; see internal/objc/class.go for why that had to
// exist first.
//
// Camera access is gated by TCC. A bundled .app needs NSCameraUsageDescription
// in its Info.plist — `gophics build` checks for it — and a bare binary run
// from a terminal inherits the terminal's own grant, so the first run may
// prompt for the terminal rather than for this program.

package camera

import (
	"errors"
	"fmt"
	"image"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"

	"github.com/doug/gophics/internal/objc"
)

// AVAuthorizationStatus, from AVCaptureDevice.h.
const (
	avAuthNotDetermined = 0
	avAuthRestricted    = 1
	avAuthDenied        = 2
	avAuthAuthorized    = 3
)

// kCVPixelFormatType_32BGRA. Requested explicitly so every frame arrives in one
// known layout; without it the camera picks its own, commonly a biplanar YUV
// that would need converting per frame.
const pixelFormat32BGRA = 0x42475241 // 'BGRA'

// Options configure a capture session.
// Authorization reports whether the process may use the camera.
//
// It does not prompt. On macOS the prompt is raised by opening the device, so
// StatusPrompt means "asking will happen when you Start", not "ask now".
func Authorization() Status {
	if err := ensure(); err != nil {
		return StatusDenied
	}
	dev := objc.Class("AVCaptureDevice")
	if !dev.Valid() {
		return StatusDenied
	}
	switch dev.SendInt("authorizationStatusForMediaType:", objc.Obj(objc.String("vide"))) {
	case avAuthAuthorized:
		return StatusGranted
	case avAuthDenied, avAuthRestricted:
		return StatusDenied
	default:
		return StatusPrompt
	}
}

// Capture is a running preview. Frame returns the most recent image.
type Capture struct {
	session  objc.ID
	delegate objc.ID
	queue    uintptr

	frames
}

var (
	loadOnce sync.Once
	loadErr  error

	symDispatchQueueCreate unsafe.Pointer
	cifDispatchQueueCreate types.CallInterface

	symSampleBufferGetImageBuffer unsafe.Pointer
	cifSampleBufferGetImageBuffer types.CallInterface

	cvLock, cvUnlock, cvBase, cvWidth, cvHeight, cvBytesPerRow unsafe.Pointer
	cifCVLock, cifCVPtr, cifCVSize                             types.CallInterface

	// delegateClass is defined once. Objective-C class names are global, so
	// defining it per capture would fail on the second camera opened.
	delegateOnce sync.Once
	delegateCls  *objc.ClassDef
	delegateErr  error

	// live maps a delegate instance to its Capture. The callback arrives with
	// only the Objective-C self pointer, so this is how it finds its way back
	// into Go state.
	live sync.Map // objc.ID → *Capture
)

func ensure() error {
	loadOnce.Do(func() { loadErr = load() })
	return loadErr
}

func load() error {
	if err := objc.Init(); err != nil {
		return err
	}
	for _, fw := range []string{"AVFoundation", "CoreMedia", "CoreVideo"} {
		if err := objc.LoadFramework(fw); err != nil {
			return fmt.Errorf("camera: load %s: %w", fw, err)
		}
	}

	sys, err := ffi.LoadLibrary("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return fmt.Errorf("camera: load libSystem: %w", err)
	}
	if symDispatchQueueCreate, err = ffi.GetSymbol(sys, "dispatch_queue_create"); err != nil {
		return fmt.Errorf("camera: dispatch_queue_create: %w", err)
	}

	cm, err := ffi.LoadLibrary("/System/Library/Frameworks/CoreMedia.framework/CoreMedia")
	if err != nil {
		return fmt.Errorf("camera: load CoreMedia: %w", err)
	}
	if symSampleBufferGetImageBuffer, err = ffi.GetSymbol(cm, "CMSampleBufferGetImageBuffer"); err != nil {
		return fmt.Errorf("camera: CMSampleBufferGetImageBuffer: %w", err)
	}

	cv, err := ffi.LoadLibrary("/System/Library/Frameworks/CoreVideo.framework/CoreVideo")
	if err != nil {
		return fmt.Errorf("camera: load CoreVideo: %w", err)
	}
	for _, s := range []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"CVPixelBufferLockBaseAddress", &cvLock},
		{"CVPixelBufferUnlockBaseAddress", &cvUnlock},
		{"CVPixelBufferGetBaseAddress", &cvBase},
		{"CVPixelBufferGetWidth", &cvWidth},
		{"CVPixelBufferGetHeight", &cvHeight},
		{"CVPixelBufferGetBytesPerRow", &cvBytesPerRow},
	} {
		p, err := ffi.GetSymbol(cv, s.name)
		if err != nil {
			return fmt.Errorf("camera: %s: %w", s.name, err)
		}
		*s.dst = p
	}

	ptr := types.PointerTypeDescriptor
	u64 := types.UInt64TypeDescriptor
	if err := ffi.PrepareCallInterface(&cifDispatchQueueCreate, types.DefaultCall, ptr,
		[]*types.TypeDescriptor{ptr, ptr}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifSampleBufferGetImageBuffer, types.DefaultCall, ptr,
		[]*types.TypeDescriptor{ptr}); err != nil {
		return err
	}
	// CVReturn CVPixelBufferLockBaseAddress(CVPixelBufferRef, CVOptionFlags)
	if err := ffi.PrepareCallInterface(&cifCVLock, types.DefaultCall, types.SInt32TypeDescriptor,
		[]*types.TypeDescriptor{ptr, u64}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifCVPtr, types.DefaultCall, ptr,
		[]*types.TypeDescriptor{ptr}); err != nil {
		return err
	}
	if err := ffi.PrepareCallInterface(&cifCVSize, types.DefaultCall, u64,
		[]*types.TypeDescriptor{ptr}); err != nil {
		return err
	}
	return nil
}

// Open starts a preview and returns a handle to its frames.
func Open(o Options) (*Capture, error) {
	if err := ensure(); err != nil {
		return nil, err
	}
	if s := Authorization(); s == StatusDenied {
		return nil, errors.New("camera: access denied — grant it in System Settings ▸ Privacy & Security ▸ Camera")
	}

	dev, err := device(o.Facing)
	if err != nil {
		return nil, err
	}

	// A camera another application holds still opens, still starts a session,
	// and still delivers nothing — the same silence as a broken pipeline, and
	// for a long time indistinguishable from one. AVCaptureDevice knows the
	// difference, so ask it rather than leaving the caller to guess from an
	// empty frame.
	var errOut objc.ID
	input := objc.Class("AVCaptureDeviceInput").Send("deviceInputWithDevice:error:",
		objc.Obj(dev), objc.Obj(objc.ID(uintptr(unsafe.Pointer(&errOut)))))
	if !input.Valid() {
		return nil, errors.New("camera: could not open the device for capture")
	}

	session := objc.Class("AVCaptureSession").Send("alloc").Send("init")
	if !session.Valid() {
		return nil, errors.New("camera: could not create a capture session")
	}
	if !session.SendBool("canAddInput:", objc.Obj(input)) {
		return nil, errors.New("camera: the session refused the camera input")
	}
	session.SendVoid("addInput:", objc.Obj(input))

	out := objc.Class("AVCaptureVideoDataOutput").Send("alloc").Send("init")
	if !out.Valid() {
		return nil, errors.New("camera: could not create the video output")
	}
	// One known pixel layout, and drop rather than queue: a preview wants the
	// newest frame, not every frame.
	settings := objc.Class("NSMutableDictionary").Send("dictionary")
	settings.SendVoid("setObject:forKey:",
		objc.Obj(objc.Class("NSNumber").Send("numberWithUnsignedInt:", objc.UInt(pixelFormat32BGRA))),
		objc.Obj(objc.String("PixelFormatType")))
	out.SendVoid("setVideoSettings:", objc.Obj(settings))
	out.SendVoid("setAlwaysDiscardsLateVideoFrames:", objc.Bool(true))

	if !session.SendBool("canAddOutput:", objc.Obj(out)) {
		return nil, errors.New("camera: the session refused the video output")
	}
	session.SendVoid("addOutput:", objc.Obj(out))

	cls, err := ensureDelegateClass()
	if err != nil {
		return nil, err
	}
	c := &Capture{session: session}
	c.delegate = cls.New()
	if !c.delegate.Valid() {
		return nil, errors.New("camera: could not create the frame delegate")
	}
	live.Store(c.delegate, c)

	qname := append([]byte("com.gophics.camera"), 0)
	np := uintptr(unsafe.Pointer(&qname[0]))
	var nilAttr uintptr
	var q uintptr
	if _, err := ffi.CallFunction(&cifDispatchQueueCreate, symDispatchQueueCreate,
		unsafe.Pointer(&q), []unsafe.Pointer{unsafe.Pointer(&np), unsafe.Pointer(&nilAttr)}); err != nil {
		return nil, fmt.Errorf("camera: dispatch_queue_create: %w", err)
	}
	c.queue = q
	out.SendVoid("setSampleBufferDelegate:queue:", objc.Obj(c.delegate), objc.Obj(objc.ID(q)))

	session.SendVoid("startRunning")
	return c, nil
}

// device picks a camera. Facing is a hint on a Mac, where the built-in camera
// is the only one most machines have.
func device(f Facing) (objc.ID, error) {
	pos := int64(2) // AVCaptureDevicePositionFront
	if f == FacingBack {
		pos = 1
	}
	types := objc.NewArray(objc.String("AVCaptureDeviceTypeBuiltInWideAngleCamera"))
	disc := objc.Class("AVCaptureDeviceDiscoverySession").Send(
		"discoverySessionWithDeviceTypes:mediaType:position:",
		objc.Obj(types), objc.Obj(objc.String("vide")), objc.Int(pos))
	if disc.Valid() {
		if devs := objc.Array(disc.Send("devices")); len(devs) > 0 {
			return devs[0], nil
		}
	}
	// Any camera beats none: a Mac with an external webcam and no built-in one
	// finds nothing above.
	if d := objc.Class("AVCaptureDevice").Send("defaultDeviceWithMediaType:",
		objc.Obj(objc.String("vide"))); d.Valid() {
		return d, nil
	}
	return 0, errors.New("camera: no camera found")
}

// ensureDelegateClass defines the one delegate class this process uses.
func ensureDelegateClass() (*objc.ClassDef, error) {
	delegateOnce.Do(func() {
		c, err := objc.DefineClass("GophicsCameraDelegate", "NSObject")
		if err != nil {
			delegateErr = err
			return
		}
		// -captureOutput:didOutputSampleBuffer:fromConnection:
		// v@:@@@ — void, self, _cmd, and three object pointers.
		if err := c.AddMethod(
			"captureOutput:didOutputSampleBuffer:fromConnection:", "v@:@@@", onFrame,
		); err != nil {
			delegateErr = err
			return
		}
		if _, err := c.Register(); err != nil {
			delegateErr = err
			return
		}
		delegateCls = c
	})
	return delegateCls, delegateErr
}

// onFrame runs on the capture queue, not the UI goroutine.
//
// It copies the pixels out and returns: the buffer belongs to CoreVideo and is
// recycled the moment this returns, so anything retained would be overwritten
// under the reader.
func onFrame(self, cmd, output, sampleBuf, conn uintptr) uintptr {
	v, ok := live.Load(objc.ID(self))
	if !ok {
		return 0
	}
	c := v.(*Capture)

	sb := sampleBuf
	var pixBuf uintptr
	if _, err := ffi.CallFunction(&cifSampleBufferGetImageBuffer, symSampleBufferGetImageBuffer,
		unsafe.Pointer(&pixBuf), []unsafe.Pointer{unsafe.Pointer(&sb)}); err != nil || pixBuf == 0 {
		return 0
	}

	const readOnly = 1 // kCVPixelBufferLock_ReadOnly
	flags := uint64(readOnly)
	var rc int32
	if _, err := ffi.CallFunction(&cifCVLock, cvLock, unsafe.Pointer(&rc),
		[]unsafe.Pointer{unsafe.Pointer(&pixBuf), unsafe.Pointer(&flags)}); err != nil || rc != 0 {
		return 0
	}
	defer func() {
		var out int32
		_, _ = ffi.CallFunction(&cifCVLock, cvUnlock, unsafe.Pointer(&out),
			[]unsafe.Pointer{unsafe.Pointer(&pixBuf), unsafe.Pointer(&flags)})
	}()

	w := int(cvSize(cvWidth, pixBuf))
	h := int(cvSize(cvHeight, pixBuf))
	stride := int(cvSize(cvBytesPerRow, pixBuf))
	if w <= 0 || h <= 0 || stride < w*4 {
		return 0
	}
	// base is unsafe.Pointer rather than uintptr on purpose. The pixels live in
	// CoreVideo's memory, not Go's, so the address is stable and the collector
	// simply ignores it — but round-tripping it through uintptr and back is the
	// unsound pattern vet rejects, because in general nothing keeps a uintptr's
	// referent alive across the conversion.
	var base unsafe.Pointer
	if _, err := ffi.CallFunction(&cifCVPtr, cvBase, unsafe.Pointer(&base),
		[]unsafe.Pointer{unsafe.Pointer(&pixBuf)}); err != nil || base == nil {
		return 0
	}

	src := unsafe.Slice((*byte)(base), stride*h)
	c.deliver(src, w, h, stride)
	return 0
}

func cvSize(sym unsafe.Pointer, buf uintptr) uint64 {
	var out uint64
	b := buf
	_, _ = ffi.CallFunction(&cifCVSize, sym, unsafe.Pointer(&out), []unsafe.Pointer{unsafe.Pointer(&b)})
	return out
}

// deliver converts BGRA to RGBA into the next pooled image and publishes it.
func (c *Capture) deliver(src []byte, w, h, stride int) {
	c.frames.deliver(w, h, func(img *image.RGBA) {
		bgraToRGBA(src, img, w, h, stride)
	})
}

// bgraToRGBA converts CoreVideo's 32BGRA rows into RGBA.
//
// Split out from deliver, and bounded per row, for the reason the V4L2 and
// Media Foundation converters are: all three index a buffer sized from a
// stride the platform reported, and slicing w*4 out of each row is only safe
// while that stride is at least a row wide. The other two were reading off the
// end when it was not; this one is the same code and was fixed with them
// rather than waiting to be caught separately.
func bgraToRGBA(src []byte, img *image.RGBA, w, h, stride int) {
	for y := 0; y < h; y++ {
		off := y * stride
		if off < 0 || off+w*4 > len(src) {
			continue
		}
		s := src[off : off+w*4]
		d := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w*4; x += 4 {
			// BGRA → RGBA; the alpha the camera reports is not meaningful.
			d[x+0], d[x+1], d[x+2], d[x+3] = s[x+2], s[x+1], s[x+0], 0xff
		}
	}
}

// Stop ends the session and releases the camera, turning its light off.
func (c *Capture) Stop() {
	if !c.stop() {
		return
	}

	if c.session.Valid() {
		c.session.SendVoid("stopRunning")
	}
	live.Delete(c.delegate)
}
