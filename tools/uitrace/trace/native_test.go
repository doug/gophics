package trace

import (
	"math"
	"path/filepath"
	"testing"
)

// Every native recording under testdata/ is replayed through gophics and the
// two curves are compared as numbers. This is the harness's purpose in one
// test: "tuned to NSScrollView; feel-test" becomes "τ within 10% of a
// recorded flick, and the same test fails when it drifts".
//
// The file is the reference. Re-recording it is how the reference changes,
// and the diff of a JSON file is how that change is reviewed.
func TestNativeRecordingsAgreeWithGophics(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.json"))
	if len(files) == 0 {
		t.Skip("no native recordings under testdata/; record one with tools/native-twin")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			native, err := ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if native.Source == "gophics" {
				t.Skip("a gophics trace is not a reference")
			}
			ours, err := Replay(native.Input, ReplayOptions{Hz: native.Hz})
			if err != nil {
				t.Fatal(err)
			}
			mn, mg := native.Compute(), ours.Compute()
			t.Logf("\n-- %s --\n%s\n-- gophics --\n%s", native.Source, mn, mg)

			// The decay is the thing. A 10% band on τ is roughly the difference
			// between a flick that feels the same and one that feels "a bit
			// heavy"; tighten it once the recordings say it can be.
			if rel := math.Abs(mg.Tau-mn.Tau) / mn.Tau; rel > 0.10 {
				t.Errorf("decay tau: gophics %.3fs vs native %.3fs (%.0f%% off)", mg.Tau, mn.Tau, rel*100)
			}
			// Momentum distance is τ and release velocity together; a 15% band
			// leaves room for the velocity estimator to differ from the OS's
			// while still catching a fling that is plainly shorter or longer.
			if rel := math.Abs(mg.MomentumDist-mn.MomentumDist) / mn.MomentumDist; rel > 0.15 {
				t.Errorf("momentum distance: gophics %.0fpx vs native %.0fpx (%.0f%% off)", mg.MomentumDist, mn.MomentumDist, rel*100)
			}
			// And the native curve must actually be exponential for τ to mean
			// anything; a low R² here is a finding about the platform, not a
			// failure of gophics.
			if mn.TauR2 < 0.95 {
				t.Logf("note: native decay fit R²=%.3f — the platform curve is not a clean exponential", mn.TauR2)
			}
		})
	}
}
