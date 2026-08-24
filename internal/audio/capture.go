// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

// Capture is the platform audio *input* interface — the mirror of Driver.
//
// It is deliberately push-shaped where Driver is pull-shaped. Playback can ask
// for samples whenever it has room, but capture has no such freedom: the audio
// arrives when the hardware says it does, and a consumer that is late has
// already lost it. So the platform calls sink from its own callback thread and
// the consumer's job is to be quick — in practice, to copy into a ring buffer
// (see internal/dsp) and return.
type Capture interface {
	// Open prepares mono capture near sampleRate and reports the rate actually
	// granted, which may differ: devices are entitled to refuse a rate, and a
	// caller that assumes it got what it asked for computes every pitch wrong
	// by the ratio.
	Open(sampleRate int) (int, error)

	// Start begins capture. sink receives mono float32 blocks in [-1,1] on the
	// platform's audio thread; it must not block, allocate heavily, or take a
	// lock the UI holds. The slice is only valid for the call.
	Start(sink func([]float32)) error

	// Close stops capture and releases the device. It must be safe to call
	// twice, since a monitor and its owner may both try to tidy up.
	Close() error
}

// NullCapture produces no audio. It backs platforms with no implementation yet,
// so a caller gets silence rather than a build failure or a nil panic.
type NullCapture struct{}

func (NullCapture) Open(sampleRate int) (int, error) {
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	return sampleRate, nil
}
func (NullCapture) Start(func([]float32)) error { return nil }
func (NullCapture) Close() error                { return nil }

// DefaultCapture returns the platform's capture implementation. Each platform
// file defines defaultCapture; unimplemented ones return NullCapture.
func DefaultCapture() Capture { return defaultCapture() }
