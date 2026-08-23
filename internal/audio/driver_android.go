// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build android

package audio

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
)

// AAudio output driver for Android (zero-cgo, via goffi) — the counterpart to
// the CoreAudio, PulseAudio and WASAPI drivers.
//
// This file exists because Android is linux and is not: driver_linux.go
// excludes it (PulseAudio is not there) and driver_default_other.go excludes all
// of linux, so the package had no defaultDriver on Android at all and failed to
// compile. Nothing built for Android until live capture gave a reason to.
//
// It runs AAudio in *blocking write* mode rather than its callback mode, which
// is the shape the PulseAudio driver uses and not the shape CoreAudio does. The
// reason is a hard constraint, not a preference: goffi cannot create C-callable
// callbacks on Android ("ffi: callbacks are unsupported on Android"), so
// AAudioStreamBuilder_setDataCallback is unusable here. Without a data callback
// AAudio expects the app to push, so the driver runs its own pump goroutine
// that pulls from the source and blocks in AAudioStream_write.
//
// libaaudio.so arrived in API 26; on anything older the load fails and the null
// driver takes over, so an app loses its sound rather than its launch.
//
// Input deliberately does not come through here. Android capture needs
// RECORD_AUDIO, which only an Activity can request, so it goes through
// AudioRecord on the Java side and into Go over the mobile bridge
// (shell/mobile.MonitorHost) instead.

// AAudio constants, from <aaudio/AAudio.h>.
const (
	aaudioOK = 0

	aaudioDirectionOutput = 0

	aaudioFormatPCMFloat = 2

	aaudioSharingModeShared = 1

	aaudioPerformanceModeLowLatency = 12

	// writeTimeoutNanos bounds a blocking write. It is generous on purpose:
	// the pump has nothing else to do, and a timeout here means the device
	// stalled, not that the app is late.
	writeTimeoutNanos = 1_000_000_000
)

var (
	aaOnce sync.Once
	aaErr  error

	aaCreateBuilder, aaSetDirection, aaSetSampleRate, aaSetChannelCount *proc
	aaSetFormat, aaSetPerfMode, aaSetSharingMode                        *proc
	aaOpenStream, aaDeleteBuilder, aaWrite                              *proc
	aaRequestStart, aaRequestStop, aaClose                              *proc
	aaGetSampleRate, aaGetChannelCount, aaGetFramesPerBurst             *proc
)

func loadAAudio() error {
	aaOnce.Do(func() {
		h, err := ffi.LoadLibrary("libaaudio.so")
		if err != nil {
			aaErr = fmt.Errorf("audio: load libaaudio (needs Android 8.0+): %w", err)
			return
		}
		mk := func(name string, nargs int) *proc {
			fn, gerr := ffi.GetSymbol(h, name)
			if gerr != nil {
				aaErr = fmt.Errorf("audio: resolve %s: %w", name, gerr)
				return nil
			}
			return newProc(fn, nargs)
		}
		aaCreateBuilder = mk("AAudio_createStreamBuilder", 1)
		aaSetDirection = mk("AAudioStreamBuilder_setDirection", 2)
		aaSetSampleRate = mk("AAudioStreamBuilder_setSampleRate", 2)
		aaSetChannelCount = mk("AAudioStreamBuilder_setChannelCount", 2)
		aaSetFormat = mk("AAudioStreamBuilder_setFormat", 2)
		aaSetPerfMode = mk("AAudioStreamBuilder_setPerformanceMode", 2)
		aaSetSharingMode = mk("AAudioStreamBuilder_setSharingMode", 2)
		aaOpenStream = mk("AAudioStreamBuilder_openStream", 2)
		aaWrite = mk("AAudioStream_write", 4)
		aaDeleteBuilder = mk("AAudioStreamBuilder_delete", 1)
		aaRequestStart = mk("AAudioStream_requestStart", 1)
		aaRequestStop = mk("AAudioStream_requestStop", 1)
		aaClose = mk("AAudioStream_close", 1)
		aaGetSampleRate = mk("AAudioStream_getSampleRate", 1)
		aaGetChannelCount = mk("AAudioStream_getChannelCount", 1)
		aaGetFramesPerBurst = mk("AAudioStream_getFramesPerBurst", 1)
	})
	return aaErr
}

type aaudioDriver struct {
	sampleRate int
	channels   int
	frames     int // frames per write

	src ReadFloat32er

	// FFI out-parameters live on the heap-allocated struct so their addresses
	// stay stable; a &stackLocal passed as a uintptr can go stale if the
	// goroutine's stack moves before C writes to it. Same reasoning as the
	// CoreAudio driver.
	builder uintptr
	stream  uintptr

	mu      sync.Mutex
	started bool
	stop    chan struct{}
	done    chan struct{}
}

func defaultDriver() Driver {
	if err := loadAAudio(); err != nil {
		// No AAudio (API < 26, or an unusual image): discard audio rather than
		// failing to start, the same contract every unimplemented capability
		// follows.
		return &NullDriver{}
	}
	return &aaudioDriver{}
}

func (d *aaudioDriver) Open(sampleRate, channels, bufferSizeMs int) error {
	if err := loadAAudio(); err != nil {
		return err
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	if channels < 1 {
		channels = 2
	}
	d.sampleRate, d.channels = sampleRate, channels
	return nil
}

func (d *aaudioDriver) SetSource(src ReadFloat32er) { d.src = src }

// pump pulls from the source and blocks in AAudioStream_write until Stop.
//
// This is the goroutine AAudio's callback would otherwise be, and it carries
// the same obligation: fall behind and the stream underruns audibly. It does no
// allocation and takes no lock in the loop for that reason.
func (d *aaudioDriver) pump() {
	defer close(d.done)
	buf := make([]float32, d.frames*d.channels)
	for {
		select {
		case <-d.stop:
			return
		default:
		}
		got := 0
		if d.src != nil {
			got, _ = d.src.ReadFloat32s(buf)
		}
		for i := got; i < len(buf); i++ {
			buf[i] = 0 // silence any shortfall rather than repeating the last block
		}
		n := int32(aaWrite.call(
			d.stream,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(d.frames),
			uintptr(writeTimeoutNanos),
		))
		if n < 0 {
			return // the stream went away; Close tidies up
		}
	}
}

func (d *aaudioDriver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil
	}
	if d.sampleRate == 0 {
		return fmt.Errorf("audio: started before Open")
	}

	d.builder = 0
	if st := aaCreateBuilder.call(uintptr(unsafe.Pointer(&d.builder))); int32(st) != aaudioOK || d.builder == 0 {
		return fmt.Errorf("audio: AAudio_createStreamBuilder failed (status %d)", int32(st))
	}
	b := d.builder

	aaSetDirection.call(b, aaudioDirectionOutput)
	aaSetSampleRate.call(b, uintptr(d.sampleRate))
	aaSetChannelCount.call(b, uintptr(d.channels))
	aaSetFormat.call(b, aaudioFormatPCMFloat)
	aaSetSharingMode.call(b, aaudioSharingModeShared)
	aaSetPerfMode.call(b, aaudioPerformanceModeLowLatency)
	// Deliberately no setDataCallback: see the file comment. Without one,
	// AAudio runs in blocking-write mode and expects the app to push.

	d.stream = 0
	st := aaOpenStream.call(b, uintptr(unsafe.Pointer(&d.stream)))
	// The builder has done its job either way; keeping it alive leaks.
	aaDeleteBuilder.call(b)
	d.builder = 0
	if int32(st) != aaudioOK || d.stream == 0 {
		return fmt.Errorf("audio: AAudioStreamBuilder_openStream failed (status %d)", int32(st))
	}

	// The device is entitled to refuse what was asked for, and the callback's
	// frame arithmetic depends on the channel count actually granted — so read
	// both back rather than assuming.
	if got := int32(aaGetChannelCount.call(d.stream)); got > 0 {
		d.channels = int(got)
	}
	if got := int32(aaGetSampleRate.call(d.stream)); got > 0 {
		d.sampleRate = int(got)
	}

	// Write a burst at a time: that is the granularity AAudio actually moves
	// data in, so anything else either wastes wakeups or invites underruns.
	d.frames = int(int32(aaGetFramesPerBurst.call(d.stream)))
	if d.frames <= 0 {
		d.frames = 256
	}

	if st := aaRequestStart.call(d.stream); int32(st) != aaudioOK {
		aaClose.call(d.stream)
		d.stream = 0
		return fmt.Errorf("audio: AAudioStream_requestStart failed (status %d)", int32(st))
	}
	d.stop = make(chan struct{})
	d.done = make(chan struct{})
	go d.pump()
	d.started = true
	return nil
}

// halt stops the pump. The caller must hold d.mu.
func (d *aaudioDriver) halt() {
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
	}
	if d.done != nil {
		<-d.done
		d.done = nil
	}
}

func (d *aaudioDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.halt()
	if d.stream != 0 {
		aaRequestStop.call(d.stream)
	}
	d.started = false
	return nil
}

func (d *aaudioDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.halt()
	if d.stream != 0 {
		aaRequestStop.call(d.stream)
		aaClose.call(d.stream)
		d.stream = 0
	}
	d.started = false
	return nil
}
