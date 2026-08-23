package pitch

import (
	"fmt"
	"math"
)

// A Note is a MIDI note number: 60 is middle C (C4), 69 is A4. Using MIDI
// numbers rather than (name, octave) pairs makes transposition, intervals, and
// scale construction plain integer arithmetic.
type Note int

// Reference pitch. A4 = MIDI 69 = 440 Hz by convention.
const (
	A4     Note = 69
	A4Freq      = 440.0
	// MiddleC is C4, the note on the ledger line between the staves.
	MiddleC Note = 60
)

// names is the sharp spelling of the twelve pitch classes. A singing app shows
// one name per key press, so a single spelling is chosen rather than offering
// enharmonics the learner has no way to disambiguate by ear.
var names = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Freq is the note's frequency in equal temperament, in Hz.
func (n Note) Freq() float64 {
	return A4Freq * math.Exp2(float64(n-A4)/12)
}

// Class is the pitch class, 0 (C) through 11 (B).
func (n Note) Class() int {
	c := int(n) % 12
	if c < 0 {
		c += 12
	}
	return c
}

// Octave is the scientific-pitch octave number: C4 (MIDI 60) is octave 4.
func (n Note) Octave() int {
	// Floor division, so notes below MIDI 0 still name coherently.
	return int(math.Floor(float64(n)/12)) - 1
}

// Name is the pitch class without the octave, e.g. "C#".
func (n Note) Name() string { return names[n.Class()] }

// String is the full scientific name, e.g. "C#4".
func (n Note) String() string { return fmt.Sprintf("%s%d", n.Name(), n.Octave()) }

// FromFreq converts a frequency to the nearest note plus the signed deviation
// from it in cents, where 100 cents is one semitone. Positive cents are sharp
// (above the note), negative are flat.
//
// Cents rather than hertz is the unit a singer can act on: 10 Hz off matters
// enormously at the bottom of a bass's range and is inaudible at the top of a
// soprano's, whereas 20 cents sounds equally out of tune everywhere.
func FromFreq(freq float64) (Note, float64) {
	if freq <= 0 {
		return 0, 0
	}
	// Exact position on the MIDI scale, fractional part included.
	exact := 12*math.Log2(freq/A4Freq) + float64(A4)
	nearest := math.Round(exact)
	return Note(nearest), (exact - nearest) * 100
}

// Cents is the signed interval from a to b in cents. It is the honest way to
// score "how close was that note", being symmetric and pitch-independent.
func Cents(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return 1200 * math.Log2(b/a)
}

// Parse reads a scientific pitch name such as "A4", "C#3", or "Bb5" into a
// Note. Both "#"/"s" and "b" accidentals are accepted so exercise definitions
// can be written the way a musician would say them.
func Parse(s string) (Note, error) {
	if s == "" {
		return 0, fmt.Errorf("pitch: empty note name")
	}
	class := -1
	switch s[0] {
	case 'C', 'c':
		class = 0
	case 'D', 'd':
		class = 2
	case 'E', 'e':
		class = 4
	case 'F', 'f':
		class = 5
	case 'G', 'g':
		class = 7
	case 'A', 'a':
		class = 9
	case 'B', 'b':
		class = 11
	default:
		return 0, fmt.Errorf("pitch: %q does not start with a note letter", s)
	}
	i := 1
	for ; i < len(s); i++ {
		if s[i] == '#' || s[i] == 's' {
			class++
		} else if s[i] == 'b' {
			class--
		} else {
			break
		}
	}
	if i >= len(s) {
		return 0, fmt.Errorf("pitch: %q has no octave number", s)
	}
	oct, neg := 0, false
	if s[i] == '-' {
		neg, i = true, i+1
	}
	if i >= len(s) {
		return 0, fmt.Errorf("pitch: %q has no octave number", s)
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("pitch: %q has a malformed octave", s)
		}
		oct = oct*10 + int(s[i]-'0')
	}
	if neg {
		oct = -oct
	}
	return Note((oct+1)*12 + class), nil
}

// MustParse is Parse for compile-time-known names, panicking on error. Use it
// for exercise tables written by hand.
func MustParse(s string) Note {
	n, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return n
}
