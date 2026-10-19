package cli

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The icon containers are written in process rather than with iconutil or
// ImageMagick, so these tests read the bytes back and check the structure —
// which is the only thing that stands between a plausible-looking file and
// Finder showing a generic icon with no error anywhere.

// icnsChunk is one entry of a parsed .icns.
type icnsChunk struct {
	osType string
	data   []byte
}

// readICNS parses the flat chunk list of an Apple icon file.
func readICNS(t *testing.T, p string) []icnsChunk {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 8 || string(raw[:4]) != "icns" {
		t.Fatalf("%s does not start with the icns magic", p)
	}
	if total := binary.BigEndian.Uint32(raw[4:8]); int(total) != len(raw) {
		t.Errorf("the header claims %d bytes, the file is %d — a reader trusts the header", total, len(raw))
	}
	var out []icnsChunk
	for off := 8; off < len(raw); {
		if off+8 > len(raw) {
			t.Fatalf("a chunk header runs past the end of the file at %d", off)
		}
		typ := string(raw[off : off+4])
		n := int(binary.BigEndian.Uint32(raw[off+4 : off+8]))
		if n < 8 || off+n > len(raw) {
			t.Fatalf("chunk %q claims %d bytes at offset %d", typ, n, off)
		}
		out = append(out, icnsChunk{typ, raw[off+8 : off+n]})
		off += n
	}
	return out
}

func TestICNSStructure(t *testing.T) {
	set, err := loadIconSet(testIcon(t, 1024))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "App.icns")
	if err := set.writeICNS(dst); err != nil {
		t.Fatal(err)
	}
	chunks := readICNS(t, dst)
	if len(chunks) != len(icnsEntries) {
		t.Errorf("%d chunks from a 1024px source, want all %d", len(chunks), len(icnsEntries))
	}
	// Every chunk's payload must be a PNG at exactly the size its OSType
	// promises — macOS picks a slot by its type and does not check.
	byType := map[string]icnsChunk{}
	for _, c := range chunks {
		byType[c.osType] = c
		cfg, err := png.DecodeConfig(bytes.NewReader(c.data))
		if err != nil {
			t.Errorf("chunk %q is not a PNG: %v", c.osType, err)
			continue
		}
		var want int
		for _, e := range icnsEntries {
			if e.osType == c.osType {
				want = e.px
			}
		}
		if cfg.Width != want || cfg.Height != want {
			t.Errorf("chunk %q holds a %dx%d image, want %dx%d", c.osType, cfg.Width, cfg.Height, want, want)
		}
	}
	// The sizes the Dock and the Finder gallery actually use.
	for _, typ := range []string{"ic07", "ic08", "ic09", "ic10"} {
		if _, ok := byType[typ]; !ok {
			t.Errorf("no %q chunk; the large sizes are the ones a user sees", typ)
		}
	}
}

// Upscaling an icon looks worse than letting the platform scale the largest
// real size, so sizes above the source are left out rather than invented.
func TestIconSkipsSizesAboveTheSource(t *testing.T) {
	set, err := loadIconSet(testIcon(t, 64))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	icns := filepath.Join(dir, "App.icns")
	if err := set.writeICNS(icns); err != nil {
		t.Fatal(err)
	}
	for _, c := range readICNS(t, icns) {
		cfg, err := png.DecodeConfig(bytes.NewReader(c.data))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width > 64 {
			t.Errorf("chunk %q is %dpx, upscaled from a 64px source", c.osType, cfg.Width)
		}
	}
	ico := filepath.Join(dir, "app.ico")
	if err := set.writeICO(ico); err != nil {
		t.Fatal(err)
	}
	for _, e := range readICO(t, ico) {
		if e.px > 64 {
			t.Errorf("ico entry is %dpx, upscaled from a 64px source", e.px)
		}
	}
	// And the hicolor tree gets the sizes that exist, not empty directories
	// for the ones that do not.
	share := filepath.Join(dir, "share")
	written, err := set.writeHicolor(share, "com.example.app")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 5 { // 16, 24, 32, 48, 64 — and nothing above
		t.Errorf("wrote %v from a 64px source", written)
	}
}

type icoEntry struct {
	px   int
	data []byte
}

// readICO parses an ICONDIR and its entries.
func readICO(t *testing.T, p string) []icoEntry {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 6 {
		t.Fatal("too short to be an .ico")
	}
	if got := binary.LittleEndian.Uint16(raw[0:2]); got != 0 {
		t.Errorf("ICONDIR reserved = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint16(raw[2:4]); got != 1 {
		t.Errorf("ICONDIR type = %d, want 1 (icon)", got)
	}
	n := int(binary.LittleEndian.Uint16(raw[4:6]))
	var out []icoEntry
	for i := 0; i < n; i++ {
		off := 6 + 16*i
		if off+16 > len(raw) {
			t.Fatalf("directory entry %d runs past the end", i)
		}
		w, h := int(raw[off]), int(raw[off+1])
		// The width field is one byte, so 256 is encoded as 0.
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		if w != h {
			t.Errorf("entry %d is %dx%d, not square", i, w, h)
		}
		size := int(binary.LittleEndian.Uint32(raw[off+8 : off+12]))
		dataOff := int(binary.LittleEndian.Uint32(raw[off+12 : off+16]))
		if dataOff+size > len(raw) {
			t.Fatalf("entry %d points at %d..%d of a %d byte file", i, dataOff, dataOff+size, len(raw))
		}
		out = append(out, icoEntry{w, raw[dataOff : dataOff+size]})
	}
	return out
}

func TestICOStructure(t *testing.T) {
	set, err := loadIconSet(testIcon(t, 1024))
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "app.ico")
	if err := set.writeICO(dst); err != nil {
		t.Fatal(err)
	}
	entries := readICO(t, dst)
	if len(entries) != len(icoSizes) {
		t.Errorf("%d entries, want %d", len(entries), len(icoSizes))
	}
	seen := map[int]bool{}
	for _, e := range entries {
		cfg, err := png.DecodeConfig(bytes.NewReader(e.data))
		if err != nil {
			t.Errorf("the %dpx entry is not a PNG: %v", e.px, err)
			continue
		}
		if cfg.Width != e.px || cfg.Height != e.px {
			t.Errorf("the entry declared %dpx holds a %dx%d image", e.px, cfg.Width, cfg.Height)
		}
		seen[e.px] = true
	}
	// 16 and 32 are the sizes Explorer and the taskbar use most; 256 is the
	// one the installer and the properties dialog show.
	for _, px := range []int{16, 32, 256} {
		if !seen[px] {
			t.Errorf("no %dpx entry", px)
		}
	}
}

// A non-square source has to be cropped or padded, both of which silently
// change the art — so it is refused, naming the size that was found.
func TestLoadIconSetRejectsNonSquare(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wide.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = loadIconSet(p)
	if err == nil {
		t.Fatal("a 64x32 icon was accepted")
	}
	if !strings.Contains(err.Error(), "64x32") {
		t.Errorf("err = %v, which does not say what was wrong with the file", err)
	}
}

func TestLoadIconSetRejectsNonPNG(t *testing.T) {
	p := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(p, []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadIconSet(p); err == nil {
		t.Error("a file that is not a PNG was accepted as an icon")
	}
}

// at() returns the source untouched at its own size: re-encoding it would
// resample a 1:1 copy for nothing.
func TestIconAtSourceSizeIsTheSourceImage(t *testing.T) {
	set, err := loadIconSet(testIcon(t, 128))
	if err != nil {
		t.Fatal(err)
	}
	img, ok := set.at(128)
	if !ok {
		t.Fatal("at(128) on a 128px source reported it was too large")
	}
	if img != set.src {
		t.Error("at() resampled the source to its own size")
	}
	if _, ok := set.at(256); ok {
		t.Error("at(256) on a 128px source produced an upscaled image")
	}
}
