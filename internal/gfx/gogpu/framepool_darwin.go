//go:build darwin

package gogpu

import "github.com/doug/gossamer/internal/gfx/gogpu/internal/platform/darwin"

func runInFramePool(fn func()) {
	darwin.RunInFramePool(fn)
}
