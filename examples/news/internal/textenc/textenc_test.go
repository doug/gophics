package textenc

import (
	"testing"
	"unicode/utf8"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"UTF-8":        "utf8",
		"utf_8":        "utf8",
		`"utf-8"`:      "utf8",
		"ISO-8859-1":   "latin1",
		"iso88591":     "latin1",
		"latin1":       "latin1",
		"windows-1252": "cp1252",
		"CP1252":       "cp1252",
		"US-ASCII":     "ascii",
		"UTF-16LE":     "utf16le",
		"UTF-16BE":     "utf16be",
		"shift_jis":    "shiftjis", // unrecognised, passed through normalised
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCP1252ToUTF8(t *testing.T) {
	// 0x91-0x94 are the curly quotes that differ from Latin-1; 0xE9 is é.
	in := []byte{'a', 0x91, 'b', 0x92, ' ', 0x93, 'c', 0x94, ' ', 0xE9, ' ', 0x80}
	got := string(CP1252ToUTF8(in))
	want := "a‘b’ “c” é €"
	if got != want {
		t.Errorf("CP1252ToUTF8 = %q, want %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Error("output is not valid UTF-8")
	}
}

func TestCP1252UnmappedByteBecomesReplacement(t *testing.T) {
	// 0x81 has no mapping in Windows-1252.
	got := string(CP1252ToUTF8([]byte{0x81}))
	if got != string(utf8.RuneError) {
		t.Errorf("got %q, want the replacement character", got)
	}
}

func TestToUTF8PrefersValidUTF8OverLabel(t *testing.T) {
	// Correct UTF-8 mislabelled as Latin-1 must survive untouched: documents
	// that lie about their charset outnumber ones that do not.
	in := []byte("café €10")
	got := string(ToUTF8(in, "iso-8859-1"))
	if got != "café €10" {
		t.Errorf("ToUTF8 mangled valid UTF-8: %q", got)
	}
}

func TestToUTF8DecodesLatin1(t *testing.T) {
	in := []byte{'C', 'a', 'f', 0xE9} // not valid UTF-8
	if got := string(ToUTF8(in, "iso-8859-1")); got != "Café" {
		t.Errorf("got %q, want Café", got)
	}
	// The same bytes with no declaration should still be salvaged.
	if got := string(ToUTF8(in, "")); got != "Café" {
		t.Errorf("undeclared: got %q, want Café", got)
	}
}

func TestUTF16(t *testing.T) {
	// "hi" little-endian with a BOM.
	le := []byte{0xFF, 0xFE, 'h', 0, 'i', 0}
	if got := string(ToUTF8(le, "")); got != "hi" {
		t.Errorf("LE with BOM = %q", got)
	}
	be := []byte{0xFE, 0xFF, 0, 'h', 0, 'i'}
	if got := string(ToUTF8(be, "")); got != "hi" {
		t.Errorf("BE with BOM = %q", got)
	}
	// A BOM must override a contradicting label.
	if got := string(ToUTF8(be, "utf-16le")); got != "hi" {
		t.Errorf("BOM should win over the label, got %q", got)
	}
	// Without a BOM, the label decides.
	if got := string(UTF16ToUTF8([]byte{0, 'h', 0, 'i'}, true)); got != "hi" {
		t.Errorf("BE without BOM = %q", got)
	}
}

func TestUTF16SurrogatePair(t *testing.T) {
	// U+1F600 encodes as the surrogate pair D83D DE00.
	in := []byte{0x3D, 0xD8, 0x00, 0xDE}
	if got := string(UTF16ToUTF8(in, false)); got != "😀" {
		t.Errorf("surrogate pair = %q, want an emoji", got)
	}
	// An unpaired surrogate must not corrupt the output.
	got := string(UTF16ToUTF8([]byte{0x3D, 0xD8}, false))
	if !utf8.ValidString(got) {
		t.Errorf("unpaired surrogate produced invalid UTF-8: %q", got)
	}
}

func TestUTF16OddLengthDoesNotPanic(t *testing.T) {
	// A truncated final byte must be ignored rather than read out of range.
	if got := string(UTF16ToUTF8([]byte{'h', 0, 'i'}, false)); got != "h" {
		t.Errorf("got %q", got)
	}
}

func TestToUTF8Empty(t *testing.T) {
	if got := ToUTF8(nil, "utf-8"); len(got) != 0 {
		t.Errorf("got %q", got)
	}
	if got := ToUTF8([]byte{}, "iso-8859-1"); len(got) != 0 {
		t.Errorf("got %q", got)
	}
}

// ISO-8859-2 shares almost none of Latin-1's upper half, so decoding a Central
// European feed as Latin-1 produces confident nonsense with no replacement
// character to show anything went wrong.
func TestLatin2IsNotDecodedAsLatin1(t *testing.T) {
	// "Zażółć gęślą jaźń" — the Polish pangram — in ISO-8859-2.
	in := []byte{0x5A, 0x61, 0xBF, 0xF3, 0xB3, 0xE6, 0x20, 0x67, 0xEA, 0xB6, 0x6C, 0xB1, 0x20, 0x6A, 0x61, 0xBC, 0xF1}
	if got, want := string(ToUTF8(in, "ISO-8859-2")), "Zażółć gęślą jaźń"; got != want {
		t.Errorf("ToUTF8(latin2) = %q, want %q", got, want)
	}
	if Normalize("iso-8859-2") != "latin2" {
		t.Errorf("Normalize(iso-8859-2) = %q", Normalize("iso-8859-2"))
	}
}

// ISO-8859-15 differs from Latin-1 in eight places, one of which is the whole
// reason the encoding exists.
func TestLatin9EuroSign(t *testing.T) {
	if got, want := string(ToUTF8([]byte{0xA4, 0x35, 0x30}, "ISO-8859-15")), "€50"; got != want {
		t.Errorf("ToUTF8(latin9) = %q, want %q", got, want)
	}
	// The characters it shares with Latin-1 must be unchanged.
	if got, want := string(ToUTF8([]byte{0xE9, 0xE8}, "latin9")), "éè"; got != want {
		t.Errorf("ToUTF8(latin9) = %q, want %q", got, want)
	}
}
