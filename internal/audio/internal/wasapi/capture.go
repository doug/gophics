// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build windows

package wasapi

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// WASAPI shared-mode capture — the mirror of driver.go's render path, reusing
// its COM plumbing (enumerator, IMMDevice, IAudioClient) and adding the one
// interface it lacks, IAudioCaptureClient.
//
// Capture is event-driven: the client signals an event each time a packet is
// ready, the pump wakes, drains every queued packet, and sleeps again. Polling
// on a timer instead would either burn CPU or drop packets when the OS is busy.

var iidIAudioCaptureClient = guid{0xC8ADBD64, 0xE71E, 0x48A0, [8]byte{0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}}

const (
	eCapture = 1 // EDataFlow: audio capture

	// AUDCLNT_BUFFERFLAGS_SILENT: the packet holds no real data and the caller
	// should treat it as zeros rather than reading the buffer.
	audclntBufferflagsSilent = 0x2
)

type iAudioCaptureClientVtbl struct {
	// IUnknown
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr

	// IAudioCaptureClient
	GetBuffer         uintptr
	ReleaseBuffer     uintptr
	GetNextPacketSize uintptr
}

type iAudioCaptureClient struct {
	vtbl *iAudioCaptureClientVtbl
}

// GetBuffer retrieves the next captured packet. It reports the frame count and
// the flags; data is only valid until ReleaseBuffer.
func (c *iAudioCaptureClient) GetBuffer() (data unsafe.Pointer, frames uint32, flags uint32, err error) {
	var devPos, qpcPos uint64
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetBuffer,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&frames)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&devPos)),
		uintptr(unsafe.Pointer(&qpcPos)),
	)
	// AUDCLNT_S_BUFFER_EMPTY (0x08890001) is a success code meaning "nothing
	// queued", not a failure — treating it as an error would tear down the
	// stream every time the pump woke early.
	if hr == 0x08890001 {
		return nil, 0, 0, nil
	}
	if hr != 0 {
		return nil, 0, 0, fmt.Errorf("IAudioCaptureClient.GetBuffer: HRESULT 0x%08X", hr)
	}
	return data, frames, flags, nil
}

// ReleaseBuffer returns the packet to the system.
func (c *iAudioCaptureClient) ReleaseBuffer(frames uint32) error {
	hr, _, _ := syscall.SyscallN(
		c.vtbl.ReleaseBuffer,
		uintptr(unsafe.Pointer(c)),
		uintptr(frames),
	)
	if hr != 0 {
		return fmt.Errorf("IAudioCaptureClient.ReleaseBuffer: HRESULT 0x%08X", hr)
	}
	return nil
}

// GetNextPacketSize reports how many frames the next packet holds (0 = none).
func (c *iAudioCaptureClient) GetNextPacketSize() (uint32, error) {
	var n uint32
	hr, _, _ := syscall.SyscallN(
		c.vtbl.GetNextPacketSize,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(&n)),
	)
	if hr != 0 {
		return 0, fmt.Errorf("IAudioCaptureClient.GetNextPacketSize: HRESULT 0x%08X", hr)
	}
	return n, nil
}

func (c *iAudioCaptureClient) Release() {
	syscall.SyscallN(c.vtbl.Release, uintptr(unsafe.Pointer(c)))
}

// Capture records mono float32 audio from the default input device.
type Capture struct {
	rate     int
	channels int // what the device actually gives us, for downmixing
	sink     func([]float32)

	enumerator *iMMDeviceEnumerator
	device     *iMMDevice
	client     *iAudioClient
	capture    *iAudioCaptureClient
	event      uintptr

	mono []float32 // reused downmix buffer

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
	initErr chan error
}

// NewCapture returns an unopened capture device.
func NewCapture() *Capture { return &Capture{} }

// Open records the requested rate; the device is not touched until Start,
// because COM must be initialized on the same thread that uses it.
func (c *Capture) Open(sampleRate int) (int, error) {
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	c.rate = sampleRate
	// AUTOCONVERTPCM makes WASAPI resample for us, so the requested rate is the
	// one delivered even when the hardware runs at another.
	return c.rate, nil
}

// Start begins capture on a dedicated, OS-thread-locked goroutine.
func (c *Capture) Start(sink func([]float32)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	c.sink = sink
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	c.initErr = make(chan error, 1)

	// COM apartment state is per-thread, so every call on these objects has to
	// happen on the thread that initialized COM — hence LockOSThread inside the
	// pump rather than setting up here and pumping there.
	go c.pump()

	if err := <-c.initErr; err != nil {
		close(c.stop)
		<-c.done
		return err
	}
	c.running = true
	return nil
}

func (c *Capture) pump() {
	defer close(c.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := c.initCOM(); err != nil {
		c.releaseCOM()
		c.initErr <- err
		return
	}
	c.initErr <- nil
	defer c.releaseCOM()

	if err := c.client.Start(); err != nil {
		return
	}
	defer c.client.Stop()

	for {
		select {
		case <-c.stop:
			return
		default:
		}
		// Wait for the "packet ready" event, with a timeout so a stalled or
		// unplugged device cannot wedge the goroutine past Close. A timeout is
		// not an error: the loop simply re-checks stop and drains whatever is
		// queued.
		waitForSingleObject(c.event, 200)
		if err := c.drain(); err != nil {
			return
		}
	}
}

// drain reads every queued packet, downmixes to mono, and delivers it.
func (c *Capture) drain() error {
	for {
		n, err := c.capture.GetNextPacketSize()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		data, frames, flags, err := c.capture.GetBuffer()
		if err != nil {
			return err
		}
		if frames == 0 {
			return nil
		}
		if cap(c.mono) < int(frames) {
			c.mono = make([]float32, frames)
		}
		c.mono = c.mono[:frames]

		if flags&audclntBufferflagsSilent != 0 || data == nil {
			for i := range c.mono {
				c.mono[i] = 0
			}
		} else {
			ch := c.channels
			if ch < 1 {
				ch = 1
			}
			src := unsafe.Slice((*float32)(data), int(frames)*ch)
			if ch == 1 {
				copy(c.mono, src)
			} else {
				// Downmix: taking channel 0 alone would halve the level on a
				// stereo input and drop a mic wired to the right channel.
				inv := float32(1) / float32(ch)
				for i := 0; i < int(frames); i++ {
					var s float32
					for j := 0; j < ch; j++ {
						s += src[i*ch+j]
					}
					c.mono[i] = s * inv
				}
			}
		}
		if s := c.sink; s != nil {
			s(c.mono)
		}
		if err := c.capture.ReleaseBuffer(frames); err != nil {
			return err
		}
	}
}

func (c *Capture) initCOM() error {
	if err := coInitializeEx(coinitMultithreaded); err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	obj, err := coCreateInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	c.enumerator = (*iMMDeviceEnumerator)(obj)

	c.device, err = c.enumerator.GetDefaultAudioEndpoint(eCapture, eConsole)
	if err != nil {
		return fmt.Errorf("wasapi: no capture device: %w", err)
	}
	c.client, err = c.device.Activate(&iidIAudioClient, clsctxAll)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Mono float32 at the requested rate, letting WASAPI convert from whatever
	// the hardware natively provides — the same trick the render path uses to
	// avoid AUDCLNT_E_UNSUPPORTED_FORMAT on picky devices.
	c.channels = 1
	format := waveFormatEx{
		FormatTag:      3, // WAVE_FORMAT_IEEE_FLOAT
		Channels:       1,
		SamplesPerSec:  uint32(c.rate),
		BitsPerSample:  32,
		BlockAlign:     4,
		AvgBytesPerSec: uint32(c.rate) * 4,
	}
	const bufferDuration = 20 * 10000 // 20 ms in 100-ns units
	err = c.client.Initialize(
		audclntSharemodeShared,
		audclntStreamflagsEventcallback|audclntStreamflagsAutoconvertpcm|audclntStreamflagsSrcDefaultQuality,
		bufferDuration, 0, &format,
	)
	if err != nil {
		return fmt.Errorf("wasapi: capture Initialize: %w", err)
	}

	c.event, err = createEvent()
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	if err := c.client.SetEventHandle(c.event); err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	svc, err := c.client.GetService(&iidIAudioCaptureClient)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	c.capture = (*iAudioCaptureClient)(svc)
	return nil
}

func (c *Capture) releaseCOM() {
	if c.capture != nil {
		c.capture.Release()
		c.capture = nil
	}
	if c.client != nil {
		c.client.Release()
		c.client = nil
	}
	if c.device != nil {
		c.device.Release()
		c.device = nil
	}
	if c.enumerator != nil {
		c.enumerator.Release()
		c.enumerator = nil
	}
	if c.event != 0 {
		closeHandle(c.event)
		c.event = 0
	}
	coUninitialize()
}

// Close stops capture and releases the device. Safe to call twice.
func (c *Capture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return nil
	}
	c.running = false
	close(c.stop)
	<-c.done
	c.sink = nil
	return nil
}
