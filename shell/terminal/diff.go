package terminal

import (
	"bytes"
	"image"
)

// tileSize is the dirty-region granularity. Frames are diffed per tile so that
// scattered changes (a moving cursor plus a distant update) transmit only the
// touched tiles instead of one bounding box spanning them — the difference
// between a few KB and most of the screen over a remote link.
const tileSize = 64

// diffTiles returns the tiles that differ between prev and cur — two RGBA
// buffers of the same w×h and row stride — clamped to the frame, plus the total
// changed area. An empty result means the frame is unchanged.
func diffTiles(prev, cur []byte, w, h, stride int) (rects []image.Rectangle, area int) {
	for ty := 0; ty < h; ty += tileSize {
		th := min(tileSize, h-ty)
		for tx := 0; tx < w; tx += tileSize {
			tw := min(tileSize, w-tx)
			if tileDiffers(prev, cur, stride, tx, ty, tw, th) {
				rects = append(rects, image.Rect(tx, ty, tx+tw, ty+th))
				area += tw * th
			}
		}
	}
	return rects, area
}

// tileDiffers reports whether the tw×th tile at (x,y) differs between the two
// buffers.
func tileDiffers(prev, cur []byte, stride, x, y, tw, th int) bool {
	row0 := x * 4
	for row := y; row < y+th; row++ {
		off := row*stride + row0
		if !bytes.Equal(prev[off:off+tw*4], cur[off:off+tw*4]) {
			return true
		}
	}
	return false
}
