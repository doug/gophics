// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build linux && !android && !js

package audio

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
)

// PulseAudio blocking playback via libpulse-simple (pa_simple), through goffi
// (zero-cgo). pa_simple has no callback, so the driver runs its own pump
// goroutine that pulls from the source and blocks in pa_simple_write. This is
// the playback mirror of the pa_simple capture path.

const (
	paStreamPlayback  = 1 // PA_STREAM_PLAYBACK
	paSampleFloat32LE = 5 // PA_SAMPLE_FLOAT32LE
)

// paSampleSpec mirrors pa_sample_spec (format u32, rate u32, channels u8+pad).
type paSampleSpec struct {
	Format   uint32
	Rate     uint32
	Channels uint8
	_        [3]byte
}

var (
	paOnce sync.Once
	paErr  error

	paSimpleNew, paSimpleWrite, paSimpleDrain, paSimpleFree *proc
)

func loadPulse() error {
	paOnce.Do(func() {
		var h unsafe.Pointer
		var err error
		for _, n := range []string{"libpulse-simple.so.0", "libpulse-simple.so"} {
			if h, err = ffi.LoadLibrary(n); err == nil {
				break
			}
		}
		if h == nil {
			paErr = fmt.Errorf("audio: load libpulse-simple (install libpulse): %w", err)
			return
		}
		mk := func(name string, nargs int) *proc {
			fn, gerr := ffi.GetSymbol(h, name)
			if gerr != nil {
				paErr = fmt.Errorf("audio: resolve %s: %w", name, gerr)
				return nil
			}
			return newProc(fn, nargs)
		}
		paSimpleNew = mk("pa_simple_new", 9)
		paSimpleWrite = mk("pa_simple_write", 4)
		paSimpleDrain = mk("pa_simple_drain", 2)
		paSimpleFree = mk("pa_simple_free", 1)
	})
	return paErr
}

// pulseDriver implements Driver by writing mixed samples to PulseAudio.
type pulseDriver struct {
	sampleRate int
	channels   int
	frames     int // frames per write

	src ReadFloat32er
	s   uintptr // pa_simple*

	// Heap-stable FFI storage (see the CoreAudio driver's note on stack-address
	// staleness): the sample spec and error out-parameter.
	spec paSampleSpec
	perr int32

	mu   sync.Mutex
	stop chan struct{}
	done chan struct{}
}

func defaultDriver() Driver { return &pulseDriver{} }

func (d *pulseDriver) Open(sampleRate, channels, bufferSizeMs int) error {
	if err := loadPulse(); err != nil {
		return err
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	if channels < 1 {
		channels = 2
	}
	if bufferSizeMs <= 0 {
		bufferSizeMs = 50
	}
	d.sampleRate, d.channels = sampleRate, channels
	d.frames = sampleRate * bufferSizeMs / 1000
	return nil
}

func (d *pulseDriver) SetSource(src ReadFloat32er) { d.src = src }

func (d *pulseDriver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.s != 0 {
		return nil
	}
	d.spec = paSampleSpec{Format: paSampleFloat32LE, Rate: uint32(d.sampleRate), Channels: uint8(d.channels)}

	name := append([]byte("gophics"), 0)
	stream := append([]byte("playback"), 0)
	d.s = paSimpleNew.call(
		0, // server (default)
		uintptr(unsafe.Pointer(&name[0])),
		paStreamPlayback,
		0, // device (default)
		uintptr(unsafe.Pointer(&stream[0])),
		uintptr(unsafe.Pointer(&d.spec)),
		0, // channel map (default)
		0, // buffer attr (default)
		uintptr(unsafe.Pointer(&d.perr)),
	)
	if d.s == 0 {
		return fmt.Errorf("audio: pa_simple_new (playback) failed (error %d)", d.perr)
	}
	d.stop = make(chan struct{})
	d.done = make(chan struct{})
	go d.pump()
	return nil
}

// pump pulls from the source and blocks in pa_simple_write, forever, until Stop.
func (d *pulseDriver) pump() {
	defer close(d.done)
	buf := make([]float32, d.frames*d.channels)
	for {
		select {
		case <-d.stop:
			return
		default:
		}
		n := 0
		if d.src != nil {
			n, _ = d.src.ReadFloat32s(buf)
		}
		for i := n; i < len(buf); i++ {
			buf[i] = 0 // silence any shortfall
		}
		raw := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), len(buf)*4)
		var perr int32
		// pa_simple_write blocks until the server accepts the data.
		if int32(paSimpleWrite.call(d.s, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), uintptr(unsafe.Pointer(&perr)))) < 0 {
			return
		}
	}
}

func (d *pulseDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
		if d.done != nil {
			<-d.done
			d.done = nil
		}
	}
	return nil
}

func (d *pulseDriver) Close() error {
	_ = d.Stop()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.s != 0 {
		var perr int32
		paSimpleDrain.call(d.s, uintptr(unsafe.Pointer(&perr)))
		paSimpleFree.call(d.s)
		d.s = 0
	}
	return nil
}
