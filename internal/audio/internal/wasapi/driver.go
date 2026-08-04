// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build windows

package wasapi

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// Source provides interleaved float32 PCM samples.
// This mirrors audio.ReadFloat32er to avoid circular imports.
type Source interface {
	ReadFloat32s(buf []float32) (int, error)
}

// Driver implements audio output using the Windows Audio Session API
// (WASAPI) in shared mode with event-driven buffering.
//
// The driver uses COM interfaces via syscall.SyscallN (zero CGO).
// A dedicated goroutine (locked to its OS thread) owns all COM state:
// initialization, the audio pull loop, and teardown.
type Driver struct {
	mu     sync.Mutex
	source Source

	sampleRate int
	channels   int

	// COM objects -- owned by the audio goroutine.
	enumerator   *iMMDeviceEnumerator
	device       *iMMDevice
	client       *iAudioClient
	render       *iAudioRenderClient
	event        uintptr // Windows event handle for buffer notification
	bufferFrames uint32

	// mixBuf is a reusable scratch buffer sized to bufferFrames*channels.
	mixBuf []float32

	done    chan struct{} // closed by Close to signal the audio goroutine
	stopped chan struct{} // closed by the audio goroutine when it exits
	running bool
}

// New creates a new WASAPI audio driver.
func New() *Driver {
	return &Driver{}
}

// Open initializes COM on a dedicated OS thread, obtains the default
// audio playback device, and prepares a shared-mode WASAPI stream.
// The stream does not begin playing until Start is called.
func (d *Driver) Open(sampleRate, channels, bufferSizeMs int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return fmt.Errorf("wasapi: driver already open")
	}

	d.sampleRate = sampleRate
	d.channels = channels
	d.done = make(chan struct{})
	d.stopped = make(chan struct{})

	// COM initialization and audio loop run on a single locked OS thread.
	// We synchronize the init result via errCh.
	errCh := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(d.stopped)

		if err := d.initCOM(bufferSizeMs); err != nil {
			d.cleanupCOM()
			errCh <- err
			return
		}
		errCh <- nil

		// Run the audio pull loop until Close signals d.done.
		d.audioLoop()

		// Teardown.
		if d.client != nil {
			_ = d.client.Stop() // best-effort
			_ = d.client.Reset()
		}
		d.cleanupCOM()
	}()

	if err := <-errCh; err != nil {
		return err
	}

	d.running = true
	return nil
}

// initCOM initializes all COM objects. Runs on the locked audio thread.
func (d *Driver) initCOM(bufferSizeMs int) error {
	if err := coInitializeEx(coinitMultithreaded); err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Device enumerator.
	obj, err := coCreateInstance(&clsidMMDeviceEnumerator, &iidIMMDeviceEnumerator)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	d.enumerator = (*iMMDeviceEnumerator)(obj)

	// Default playback device.
	d.device, err = d.enumerator.GetDefaultAudioEndpoint(eRender, eConsole)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// IAudioClient.
	d.client, err = d.device.Activate(&iidIAudioClient, clsctxAll)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Request float32 PCM format with AUTOCONVERTPCM. This tells WASAPI
	// to accept our format and do sample rate / format conversion internally
	// if the hardware uses a different native format. This fixes
	// AUDCLNT_E_UNSUPPORTED_FORMAT (0x88890008) on devices that do not
	// natively support IEEE float at the requested sample rate.
	format := waveFormatEx{
		FormatTag:      3, // WAVE_FORMAT_IEEE_FLOAT
		Channels:       uint16(d.channels),
		SamplesPerSec:  uint32(d.sampleRate),
		BitsPerSample:  32,
		BlockAlign:     uint16(d.channels) * 4,
		AvgBytesPerSec: uint32(d.sampleRate) * uint32(d.channels) * 4,
		CbSize:         0,
	}

	bufferDuration := referenceTimeFromMs(bufferSizeMs)
	streamFlags := uint32(audclntStreamflagsEventcallback |
		audclntStreamflagsAutoconvertpcm |
		audclntStreamflagsSrcDefaultQuality)

	err = d.client.Initialize(
		audclntSharemodeShared,
		streamFlags,
		bufferDuration,
		0, // periodicity (must be 0 for shared mode)
		&format,
	)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Actual buffer size (may differ from requested).
	d.bufferFrames, err = d.client.GetBufferSize()
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Event handle for buffer-ready notifications.
	d.event, err = createEvent()
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	if err := d.client.SetEventHandle(d.event); err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}

	// Render client for writing PCM data.
	svc, err := d.client.GetService(&iidIAudioRenderClient)
	if err != nil {
		return fmt.Errorf("wasapi: %w", err)
	}
	d.render = (*iAudioRenderClient)(svc)

	// Pre-allocate mix buffer.
	d.mixBuf = make([]float32, d.bufferFrames*uint32(d.channels))

	return nil
}

// SetSource sets the audio data source. The driver pulls from this source
// in its audio loop.
func (d *Driver) SetSource(src Source) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.source = src
}

// Start begins WASAPI audio streaming. The audio loop goroutine (already
// running since Open) starts pulling samples from the source and writing
// them to the hardware buffer.
func (d *Driver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return fmt.Errorf("wasapi: driver not open")
	}

	return d.client.Start()
}

// Stop pauses audio playback without closing the device.
func (d *Driver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	return d.client.Stop()
}

// Close stops playback, signals the audio goroutine to exit, and waits
// for COM cleanup to finish.
func (d *Driver) Close() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = false
	d.mu.Unlock()

	close(d.done)
	<-d.stopped // wait for COM cleanup on the audio thread
	return nil
}

// cleanupCOM releases all COM resources. Must run on the COM thread.
func (d *Driver) cleanupCOM() {
	if d.render != nil {
		d.render.Release()
		d.render = nil
	}
	if d.client != nil {
		d.client.Release()
		d.client = nil
	}
	if d.device != nil {
		d.device.Release()
		d.device = nil
	}
	if d.enumerator != nil {
		d.enumerator.Release()
		d.enumerator = nil
	}
	if d.event != 0 {
		closeHandle(d.event)
		d.event = 0
	}
	coUninitialize()
}

// audioLoop runs on the COM goroutine. It waits for WASAPI buffer-ready
// events, pulls samples from the source, and writes them to the render
// buffer. Exits when d.done is closed.
func (d *Driver) audioLoop() {
	for {
		select {
		case <-d.done:
			return
		default:
		}

		// Wait for WASAPI to signal buffer availability (timeout 50ms
		// ensures we check d.done periodically even if no audio plays).
		waitForSingleObject(d.event, 50)

		// How many frames are already queued.
		padding, err := d.client.GetCurrentPadding()
		if err != nil {
			continue
		}

		available := d.bufferFrames - padding
		if available == 0 {
			continue
		}

		// Get writable region from WASAPI.
		data, err := d.render.GetBuffer(available)
		if err != nil {
			continue
		}

		samples := d.mixBuf[:available*uint32(d.channels)]

		// Zero the scratch buffer.
		for i := range samples {
			samples[i] = 0
		}

		// Pull audio from the source.
		d.mu.Lock()
		src := d.source
		d.mu.Unlock()

		if src != nil {
			_, _ = src.ReadFloat32s(samples) // Mixer never errors
		}

		// Copy to the WASAPI buffer.
		dst := unsafe.Slice((*float32)(data), available*uint32(d.channels))
		copy(dst, samples)

		_ = d.render.ReleaseBuffer(available, 0) // 0 = data written
	}
}
