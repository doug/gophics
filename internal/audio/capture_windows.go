// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build windows

package audio

import "github.com/doug/gophics/internal/audio/internal/wasapi"

// wasapiCapture adapts internal/wasapi.Capture to the audio.Capture interface,
// the same way wasapiWrapper adapts the render driver: the inner package cannot
// import this one without a cycle, so the thin shim lives here.
type wasapiCapture struct{ inner *wasapi.Capture }

func (c *wasapiCapture) Open(sampleRate int) (int, error) { return c.inner.Open(sampleRate) }
func (c *wasapiCapture) Start(sink func([]float32)) error { return c.inner.Start(sink) }
func (c *wasapiCapture) Close() error                     { return c.inner.Close() }

func defaultCapture() Capture { return &wasapiCapture{inner: wasapi.NewCapture()} }
