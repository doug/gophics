// Package testfont derives small test fonts from a real one, for exercising
// the font fallback chain without shipping a fixture file.
//
// The Go fonts all share one character set, so no pair of them can stand in
// for "a face that has a glyph the primary lacks" — the case fallback exists
// for. Rather than vendor a symbol font, Remap rewrites a font's cmap so it
// covers only the runes the test names, borrowing outlines from glyphs the
// base font already has. The result parses and shapes like any other TTF.
package testfont

import (
	"encoding/binary"
	"fmt"

	"github.com/doug/gophics/text"
)

// Remap returns a copy of base whose character map covers exactly the runes
// in mapping: each key rune is drawn with the outline base uses for its value
// rune. Every other rune — including everything base itself covered — becomes
// unmapped, so the result is a "symbol font" from the shaper's point of view.
//
// Only the cmap table is replaced (appended to the file and re-pointed from
// the table directory; the stale checksum is ignored by every parser that
// matters). Glyph data, metrics and layout tables are the base's.
func Remap(base []byte, mapping map[rune]rune) ([]byte, error) {
	src, err := text.Parse(base)
	if err != nil {
		return nil, err
	}
	type group struct {
		start, end, gid uint32
	}
	var groups []group
	for from, to := range mapping {
		gid, ok := src.NominalGID(to)
		if !ok {
			return nil, fmt.Errorf("testfont: base font has no glyph for %q", to)
		}
		groups = append(groups, group{uint32(from), uint32(from), gid})
	}
	// Format 12 requires groups sorted by start code.
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0 && groups[j].start < groups[j-1].start; j-- {
			groups[j], groups[j-1] = groups[j-1], groups[j]
		}
	}

	// cmap header + one encoding record (Windows / Unicode full) + format 12.
	const subtableOff = 4 + 8
	subLen := uint32(16 + 12*len(groups))
	cmap := make([]byte, 0, subtableOff+int(subLen))
	cmap = binary.BigEndian.AppendUint16(cmap, 0) // version
	cmap = binary.BigEndian.AppendUint16(cmap, 1) // numTables
	cmap = binary.BigEndian.AppendUint16(cmap, 3) // platformID: Windows
	cmap = binary.BigEndian.AppendUint16(cmap, 10)
	cmap = binary.BigEndian.AppendUint32(cmap, subtableOff)
	cmap = binary.BigEndian.AppendUint16(cmap, 12) // format
	cmap = binary.BigEndian.AppendUint16(cmap, 0)  // reserved
	cmap = binary.BigEndian.AppendUint32(cmap, subLen)
	cmap = binary.BigEndian.AppendUint32(cmap, 0) // language
	cmap = binary.BigEndian.AppendUint32(cmap, uint32(len(groups)))
	for _, g := range groups {
		cmap = binary.BigEndian.AppendUint32(cmap, g.start)
		cmap = binary.BigEndian.AppendUint32(cmap, g.end)
		cmap = binary.BigEndian.AppendUint32(cmap, g.gid)
	}

	if len(base) < 12 {
		return nil, fmt.Errorf("testfont: base too short")
	}
	out := make([]byte, len(base), len(base)+len(cmap)+4)
	copy(out, base)
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	newOff := uint32(len(out))
	out = append(out, cmap...)

	numTables := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		if rec+16 > len(base) {
			break
		}
		if string(out[rec:rec+4]) != "cmap" {
			continue
		}
		binary.BigEndian.PutUint32(out[rec+8:], newOff)
		binary.BigEndian.PutUint32(out[rec+12:], uint32(len(cmap)))
		return out, nil
	}
	return nil, fmt.Errorf("testfont: base has no cmap table")
}
