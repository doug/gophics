// Package textenc converts the legacy character encodings that appear in feeds
// and web pages into UTF-8, using only the standard library.
//
// The coverage is deliberately narrow: UTF-8, UTF-16, Latin-1 and Windows-1252
// account for essentially everything still served in the wild, and Windows-1252
// is the browser fallback for unrecognised labels. Conversion never fails —
// refusing a document over a charset label is worse than a few replacement
// characters.
package textenc

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Normalize canonicalises a charset label: "ISO-8859-1" and "iso88591" both
// become "latin1".
func Normalize(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	label = strings.NewReplacer("-", "", "_", "", " ", "", `"`, "", "'", "").Replace(label)
	switch label {
	case "iso88591", "latin1", "l1", "cp819":
		return "latin1"
	case "iso88592", "latin2", "l2":
		return "latin2"
	case "iso885915", "latin9", "l9":
		return "latin9"
	case "windows1252", "cp1252", "ansi", "ansix341968":
		return "cp1252"
	case "utf8", "utf8mb4":
		return "utf8"
	case "usascii", "ascii":
		return "ascii"
	case "utf16":
		return "utf16"
	case "utf16le", "unicode":
		return "utf16le"
	case "utf16be":
		return "utf16be"
	}
	return label
}

// ToUTF8 converts b to UTF-8. The declared label is a hint; when the bytes are
// already valid UTF-8 they are returned untouched regardless of the label,
// because mislabelled-but-correct documents outnumber correctly labelled ones.
func ToUTF8(b []byte, declared string) []byte {
	switch Normalize(declared) {
	case "utf16", "utf16le":
		return UTF16ToUTF8(b, false)
	case "utf16be":
		return UTF16ToUTF8(b, true)
	}
	// A byte-order mark overrides any declaration.
	if len(b) >= 2 {
		if b[0] == 0xFE && b[1] == 0xFF {
			return UTF16ToUTF8(b, true)
		}
		if b[0] == 0xFF && b[1] == 0xFE {
			return UTF16ToUTF8(b, false)
		}
	}
	if utf8.Valid(b) {
		return b
	}
	switch Normalize(declared) {
	case "latin2":
		return highTableToUTF8(b, latin2High)
	case "latin9":
		return highTableToUTF8(b, latin9High)
	}
	return CP1252ToUTF8(b)
}

// latin2High and latin9High map 0xA0..0xFF for the two eight-bit encodings that
// are close enough to Latin-1 to be mistaken for it and different enough to
// matter.
//
// These exist because treating them as Latin-1 corrupts silently rather than
// visibly. ISO-8859-2 is Central European and shares almost none of Latin-1's
// upper half, so a Polish or Czech feed decoded as Latin-1 comes out as
// confident nonsense — every accented letter wrong, and no replacement
// character to show that anything happened. ISO-8859-15 differs in only eight
// places, but one of them is the euro sign, which is the whole reason the
// encoding exists.
var latin2High = [96]rune{
	0x00A0, 0x0104, 0x02D8, 0x0141, 0x00A4, 0x013D, 0x015A, 0x00A7,
	0x00A8, 0x0160, 0x015E, 0x0164, 0x0179, 0x00AD, 0x017D, 0x017B,
	0x00B0, 0x0105, 0x02DB, 0x0142, 0x00B4, 0x013E, 0x015B, 0x02C7,
	0x00B8, 0x0161, 0x015F, 0x0165, 0x017A, 0x02DD, 0x017E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7,
	0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7,
	0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7,
	0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7,
	0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}

// latin9High is Latin-1 with the eight ISO-8859-15 revisions applied.
var latin9High = func() [96]rune {
	var t [96]rune
	for i := range t {
		t[i] = rune(0xA0 + i)
	}
	for b, r := range map[int]rune{
		0xA4: 0x20AC, 0xA6: 0x0160, 0xA8: 0x0161, 0xB4: 0x017D,
		0xB8: 0x017E, 0xBC: 0x0152, 0xBD: 0x0153, 0xBE: 0x0178,
	} {
		t[b-0xA0] = r
	}
	return t
}()

// highTableToUTF8 decodes an eight-bit encoding that is ASCII below 0xA0 and
// table-driven above it. Bytes in 0x80..0x9F are C1 controls in these
// encodings and are not expected in text.
func highTableToUTF8(b []byte, table [96]rune) []byte {
	var out bytes.Buffer
	out.Grow(len(b) * 2)
	for _, c := range b {
		switch {
		case c < 0x80:
			out.WriteByte(c)
		case c < 0xA0:
			out.WriteRune(utf8.RuneError)
		default:
			out.WriteRune(table[c-0xA0])
		}
	}
	return out.Bytes()
}

// cp1252Extra maps 0x80..0x9F, the only range where Windows-1252 differs from
// Latin-1. A zero entry means the byte is unmapped and becomes U+FFFD.
var cp1252Extra = [32]rune{
	0x20AC, 0, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021,
	0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0, 0x017D, 0,
	0, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0, 0x017E, 0x0178,
}

// CP1252ToUTF8 decodes Windows-1252 (a superset of Latin-1) to UTF-8.
func CP1252ToUTF8(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) * 2)
	for _, c := range b {
		switch {
		case c < 0x80:
			out.WriteByte(c)
		case c < 0xA0:
			r := cp1252Extra[c-0x80]
			if r == 0 {
				r = utf8.RuneError
			}
			out.WriteRune(r)
		default:
			out.WriteRune(rune(c)) // Latin-1 bytes are their own code points
		}
	}
	return out.Bytes()
}

// UTF16ToUTF8 decodes UTF-16, honouring a leading byte-order mark over the
// bigEndian argument.
func UTF16ToUTF8(b []byte, bigEndian bool) []byte {
	if len(b) >= 2 {
		switch {
		case b[0] == 0xFE && b[1] == 0xFF:
			bigEndian, b = true, b[2:]
		case b[0] == 0xFF && b[1] == 0xFE:
			bigEndian, b = false, b[2:]
		}
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			units = append(units, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	var out bytes.Buffer
	out.Grow(len(units) * 2)
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u >= 0xD800 && u < 0xDC00 && i+1 < len(units) &&
			units[i+1] >= 0xDC00 && units[i+1] < 0xE000:
			out.WriteRune(rune(u-0xD800)<<10 | rune(units[i+1]-0xDC00) + 0x10000)
			i++
		case u >= 0xD800 && u < 0xE000:
			out.WriteRune(utf8.RuneError) // unpaired surrogate
		default:
			out.WriteRune(rune(u))
		}
	}
	return out.Bytes()
}
