// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !windows && !darwin && !linux && !js

package audio

func defaultDriver() Driver {
	return &NullDriver{}
}
