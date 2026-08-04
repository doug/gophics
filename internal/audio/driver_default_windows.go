// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build windows

package audio

import "github.com/doug/gossamer/internal/audio/internal/wasapi"

// wasapiWrapper adapts internal/wasapi.Driver to the audio.Driver interface.
// This is necessary because wasapi.Driver.SetSource takes wasapi.Source
// (to avoid circular imports), while audio.Driver.SetSource takes
// audio.ReadFloat32er. Both interfaces have identical method signatures,
// so audio.ReadFloat32er satisfies wasapi.Source via Go structural typing.
type wasapiWrapper struct {
	inner *wasapi.Driver
}

func (w *wasapiWrapper) Open(sampleRate, channels, bufferSizeMs int) error {
	return w.inner.Open(sampleRate, channels, bufferSizeMs)
}

func (w *wasapiWrapper) SetSource(src ReadFloat32er) {
	w.inner.SetSource(src)
}

func (w *wasapiWrapper) Start() error {
	return w.inner.Start()
}

func (w *wasapiWrapper) Stop() error {
	return w.inner.Stop()
}

func (w *wasapiWrapper) Close() error {
	return w.inner.Close()
}

func defaultDriver() Driver {
	return &wasapiWrapper{inner: wasapi.New()}
}
