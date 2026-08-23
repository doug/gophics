// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build !darwin && !windows && (!linux || android)

package audio

// Platforms with no native capture path get silence.
//
// js/wasm is here on purpose and is not a gap: the web shell captures through
// getUserMedia and a Web Audio AnalyserNode directly (shell/web), which is both
// the only way a browser exposes input and better than anything this layer
// could do. Android likewise captures on the Java side and pushes PCM through
// the mobile bridge, because AudioRecord needs a permission-bearing Activity.
func defaultCapture() Capture { return NullCapture{} }
