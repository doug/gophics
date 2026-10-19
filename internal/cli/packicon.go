package cli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// Icon conversion, in process.
//
// Every platform wants the same picture in its own container: macOS an .icns,
// Windows an .ico, Linux a PNG per size under the hicolor theme. The
// conventional answers are iconutil (macOS only, and only from a .iconset
// directory), ImageMagick's convert (not installed by default anywhere) and a
// hand-run loop of resizes. All three would make desktop packaging depend on
// tooling the developer has to install first, and two of them only exist on
// one of the three platforms — which would mean a Mac could not produce the
// Linux icons, which is the case packaging has to handle.
//
// So the conversion is done here: decode one square PNG, resample it with
// x/image/draw (already a dependency, and the same resampler app/thumb.go
// uses), and write the containers. Both container formats accept PNG payloads,
// which is what makes this a few dozen lines rather than a BMP and ARGB
// encoder.

// iconSet is one source image, ready to be emitted at any size up to its own.
type iconSet struct {
	src    image.Image
	source int // the source's edge length in pixels
	path   string
}

// loadIconSet decodes a square PNG.
//
// Square is required rather than fixed up. A non-square source has to be
// cropped or letterboxed, both of which silently change the art — and the
// developer who exported a 1920x1080 screenshot by mistake wants to be told,
// not to receive a bundle whose icon is the middle third of their window.
func loadIconSet(p string) (*iconSet, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("icon: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("icon %s: %w (a PNG is expected)", p, err)
	}
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		return nil, fmt.Errorf("icon %s is %dx%d; app icons are square on every "+
			"platform, so crop or pad it first", p, b.Dx(), b.Dy())
	}
	if b.Dx() < 16 {
		return nil, fmt.Errorf("icon %s is %dx%d, too small to be an app icon", p, b.Dx(), b.Dy())
	}
	return &iconSet{src: img, source: b.Dx(), path: p}, nil
}

// at returns the icon resampled to px square, or ok=false when px is larger
// than the source. Upscaling is refused rather than done badly: a 256px source
// blown up to 1024 looks worse in the Dock than the 256px entry alone, which
// macOS will scale itself with the same information and better filtering.
func (s *iconSet) at(px int) (image.Image, bool) {
	if px > s.source {
		return nil, false
	}
	if px == s.source {
		return s.src, true
	}
	dst := image.NewRGBA(image.Rect(0, 0, px, px))
	// CatmullRom for the same reason app/thumb.go uses it: downscaling an icon
	// with a box filter loses the thin strokes that make a small icon legible.
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), s.src, s.src.Bounds(), xdraw.Src, nil)
	return dst, true
}

// pngAt encodes the icon at px square.
func (s *iconSet) pngAt(px int) ([]byte, bool, error) {
	img, ok := s.at(px)
	if !ok {
		return nil, false, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

// icnsEntry is one member of an .icns file: the four-character OSType Apple
// uses for a given nominal size and scale, and the pixel count it holds.
//
// The ic* types all accept a PNG payload (the older is32/il32 pairs of raw
// ARGB and a separate mask do not, and are not emitted). The duplicated pixel
// counts are not a mistake: 32px appears as both icp5 (32x32 @1x) and ic11
// (16x16 @2x), and a Retina Mac picks the @2x entry for a 16pt slot. iconutil
// emits the same pairs from an .iconset.
type icnsEntry struct {
	osType string
	px     int
}

var icnsEntries = []icnsEntry{
	{"icp4", 16},
	{"icp5", 32},
	{"ic11", 32},   // 16x16@2x
	{"icp6", 64},   // 64x64
	{"ic12", 64},   // 32x32@2x
	{"ic07", 128},  // 128x128
	{"ic08", 256},  // 256x256
	{"ic13", 256},  // 128x128@2x
	{"ic09", 512},  // 512x512
	{"ic14", 512},  // 256x256@2x
	{"ic10", 1024}, // 512x512@2x — the size the Dock and Finder's gallery use
}

// writeICNS writes an Apple icon file.
//
// The format is a flat chunk list: the magic, the total length, then one
// 4-byte OSType + big-endian length-including-header + payload per entry. No
// index, no alignment, no compression beyond the PNGs themselves.
func (s *iconSet) writeICNS(dst string) error {
	var body bytes.Buffer
	n := 0
	for _, e := range icnsEntries {
		data, ok, err := s.pngAt(e.px)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		body.WriteString(e.osType)
		_ = binary.Write(&body, binary.BigEndian, uint32(len(data)+8))
		body.Write(data)
		n++
	}
	if n == 0 {
		return fmt.Errorf("icon %s produced no icns entries", s.path)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	_ = binary.Write(&out, binary.BigEndian, uint32(body.Len()+8))
	out.Write(body.Bytes())
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, out.Bytes(), 0o644)
}

// icoSizes are the entries a Windows .ico carries. 256 is the largest the
// format can describe — its width field is a single byte, with 0 meaning 256 —
// so there is no point resampling above it.
var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

// writeICO writes a Windows icon file with PNG-compressed entries.
//
// PNG payloads in an .ico are read by Windows Vista and later, which is every
// Windows that can run a gophics build. The alternative is a 32-bit DIB per
// entry with a padded AND mask, three times the code for an icon nothing will
// read differently.
func (s *iconSet) writeICO(dst string) error {
	type entry struct {
		px   int
		data []byte
	}
	var entries []entry
	for _, px := range icoSizes {
		data, ok, err := s.pngAt(px)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		entries = append(entries, entry{px, data})
	}
	if len(entries) == 0 {
		return fmt.Errorf("icon %s produced no ico entries", s.path)
	}
	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), count.
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		dim := byte(e.px)
		if e.px >= 256 {
			dim = 0 // the field is one byte; 0 means 256
		}
		out.WriteByte(dim)                                      // width
		out.WriteByte(dim)                                      // height
		out.WriteByte(0)                                        // palette entries: none, it is truecolour
		out.WriteByte(0)                                        // reserved
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		_ = binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, out.Bytes(), 0o644)
}

// hicolorSizes are the sizes the freedesktop icon theme specification names as
// the standard ones for application icons. A theme directory per size, which is
// how a panel asks for 24px and a file manager for 128px and both get art
// drawn for that size rather than one scaled badly.
var hicolorSizes = []int{16, 24, 32, 48, 64, 128, 256, 512}

// writeHicolor installs the icon into an icon theme under share, as
// share/icons/hicolor/<size>x<size>/apps/<name>.png, and returns the relative
// paths it wrote.
//
// The spec's lookup is by theme, then by size directory, then by the icon's
// name without an extension — which is why the .desktop file's Icon key is a
// bare name here and not a path. A path works in some desktops and not others;
// the name works everywhere, and lets a user's icon theme override it.
func (s *iconSet) writeHicolor(share, name string) ([]string, error) {
	var written []string
	for _, px := range hicolorSizes {
		data, ok, err := s.pngAt(px)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		rel := filepath.Join("icons", "hicolor", fmt.Sprintf("%dx%d", px, px), "apps", name+".png")
		if _, err := writeFileIn(filepath.Join(share, filepath.Dir(rel)), name+".png", data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	if len(written) == 0 {
		return nil, fmt.Errorf("icon %s produced no theme icons", s.path)
	}
	sort.Strings(written)
	return written, nil
}
