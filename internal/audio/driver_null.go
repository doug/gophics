// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

// NullDriver discards all audio output.
// It is used for testing and headless environments where no audio
// hardware is available.
type NullDriver struct{}

// Open is a no-op for the null driver.
func (d *NullDriver) Open(_, _, _ int) error { return nil }

// SetSource is a no-op for the null driver.
func (d *NullDriver) SetSource(_ ReadFloat32er) {}

// Start is a no-op for the null driver.
func (d *NullDriver) Start() error { return nil }

// Stop is a no-op for the null driver.
func (d *NullDriver) Stop() error { return nil }

// Close is a no-op for the null driver.
func (d *NullDriver) Close() error { return nil }
