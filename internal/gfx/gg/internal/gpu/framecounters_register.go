package gpu

import (
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// The encoder counters live in wgpu, at the choke points that cannot miss a
// call site; the frame boundary that zeroes them lives in gg. This package
// imports both, so it is where they are joined — see gg.RegisterEncoderReset
// for why gg does not simply import wgpu.
func init() { gg.RegisterEncoderReset(wgpu.ResetEncoderStats) }
