package chart

import "testing"

// A bad pair in runtime data costs that pair, not the process. Chart data
// arrives from files and networks; a panic here is a crash a user sees for a
// row a log line could have covered.
func TestValuesSkipsMalformedPairs(t *testing.T) {
	got := Values("Mon", 3, 42, 5, "Wed", "not a number", "Thu", 2.5)
	if len(got) != 2 {
		t.Fatalf("got %d data, want the 2 well-formed pairs: %v", len(got), got)
	}
	if got[0].Label != "Mon" || got[1].Label != "Thu" {
		t.Errorf("kept %q and %q, want Mon and Thu", got[0].Label, got[1].Label)
	}
	// Band indexes stay dense after a skip — a gap would leave an empty band
	// in the middle of the chart where the bad pair used to be.
	if got[1].X != 1 {
		t.Errorf("Thu is at X=%v, want dense band index 1", got[1].X)
	}
}
