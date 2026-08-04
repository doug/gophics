// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"errors"
	"io"
	"sync"
)

const defaultBufferSizeMs = 20

const (
	defaultSampleRate = 44100
	defaultChannels   = 2
)

// Context manages audio output for an application. It owns the platform
// audio device and coordinates player lifecycle and mixing. Typically
// one Context exists per application.
//
// The driver uses a pull model: a background goroutine inside the driver
// reads PCM data from the Mixer via ReadFloat32s and feeds it to the
// hardware. Context.Start starts the driver; Context.Close stops it.
type Context struct {
	mu         sync.Mutex
	sampleRate int
	channels   int
	driver     Driver
	mixer      *Mixer
	players    []*Player
	running    bool
}

// Option configures the audio context during construction.
type Option func(*Context)

// WithSampleRate sets the audio sample rate in Hz. Default is 44100.
func WithSampleRate(rate int) Option {
	return func(c *Context) {
		if rate > 0 {
			c.sampleRate = rate
		}
	}
}

// WithChannels sets the number of output channels. Default is 2 (stereo).
func WithChannels(ch int) Option {
	return func(c *Context) {
		if ch > 0 {
			c.channels = ch
		}
	}
}

// WithDriver sets a specific audio driver. If not set, NullDriver is used
// until platform drivers are implemented.
func WithDriver(d Driver) Option {
	return func(c *Context) {
		c.driver = d
	}
}

// NewContext creates an audio context, opens the audio device, and starts
// playback. The driver pulls audio data from the internal Mixer, which
// sums all active players.
func NewContext(opts ...Option) (*Context, error) {
	c := &Context{
		sampleRate: defaultSampleRate,
		channels:   defaultChannels,
		mixer:      NewMixer(),
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.driver == nil {
		c.driver = defaultDriver()
	}

	if err := c.driver.Open(c.sampleRate, c.channels, defaultBufferSizeMs); err != nil {
		return nil, errors.New("audio: failed to open driver: " + err.Error())
	}

	// Wire the mixer as the driver's audio source.
	c.driver.SetSource(c.mixer)

	if err := c.driver.Start(); err != nil {
		// Clean up on start failure.
		_ = c.driver.Close() // best-effort
		return nil, errors.New("audio: failed to start driver: " + err.Error())
	}

	c.running = true
	return c, nil
}

// Close stops the driver and releases the audio device.
func (c *Context) Close() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	return c.driver.Close()
}

// NewPlayer creates a player from a PCM audio stream. The reader should
// provide interleaved float32 PCM samples encoded as little-endian bytes
// (4 bytes per sample, same layout as WAVDecoder.Read output).
func (c *Context) NewPlayer(src io.Reader) *Player {
	p := &Player{
		ctx:    c,
		src:    src,
		volume: 1.0,
	}

	c.mu.Lock()
	c.players = append(c.players, p)
	c.mixer.AddSource(p)
	c.mu.Unlock()

	return p
}

// PlayWAV decodes WAV data and starts playback immediately.
// Returns the player for volume/pause/stop control.
func (c *Context) PlayWAV(data []byte) (*Player, error) {
	dec, err := DecodeWAV(data)
	if err != nil {
		return nil, err
	}
	p := c.NewPlayer(dec)
	p.Play()
	return p, nil
}

// SampleRate returns the context's configured sample rate.
func (c *Context) SampleRate() int {
	return c.sampleRate
}

// Channels returns the context's configured channel count.
func (c *Context) Channels() int {
	return c.channels
}
