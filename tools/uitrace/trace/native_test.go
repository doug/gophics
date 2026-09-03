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

			// A recording must be one gesture: every finger event before the
			// release. The first macOS recording had two — a flick and a
			// reflexive flick back — and the second's finger phase was replayed
			// into the first's momentum. The twin now refuses the second
			// gesture; this refuses the file if one slips through.
			for _, in := range native.Input {
				if in.T > native.ReleaseT {
					t.Fatalf("input at t=%.3f is after release at t=%.3f: two gestures in one recording", in.T, native.ReleaseT)
				}
			}

			// What a recording is evidence of depends on where it came from.
			// gophics's own fling runs on touch platforms — mobile, web-touch —
			// where there is no OS momentum, so an iOS recording is the
			// reference that binds. A macOS recording is Apple's trackpad
			// momentum, which the desktop shell passes through untouched; it
			// is the closest thing to a reference available without a device,
			// and the numbers are reported, but gophics is not held to it. The
			// first one measured τ=0.186s against gophics's 0.498: same travel,
			// three times the settle — recorded in tools/uitrace/README.md.
			strict := native.Source != "macos-appkit"
			check := func(name string, got, want float64, tol float64, unit string) {
				rel := math.Abs(got-want) / math.Abs(want)
				msg := "%s: gophics %.3f vs %s %.3f %s (%.0f%% off)"
				if rel > tol && strict {
					t.Errorf(msg, name, got, native.Source, want, unit, rel*100)
				} else if rel > tol {
					t.Logf("informational — "+msg, name, got, native.Source, want, unit, rel*100)
				}
			}

			// The decay is the thing. A 10% band on τ is roughly the difference
			// between a flick that feels the same and one that feels "a bit
			// heavy"; tighten it once the recordings say it can be.
			check("decay tau", mg.Tau, mn.Tau, 0.10, "s")
			// Momentum distance is τ and release velocity together; a 15% band
			// leaves room for the velocity estimator to differ from the OS's
			// while still catching a fling that is plainly shorter or longer.
			check("momentum distance", mg.MomentumDist, mn.MomentumDist, 0.15, "px")
			// The velocity the fling actually starts from, as the fit's
			// intercept — not the finite difference at release, which straddles
			// the finger's last frame. gophics's estimator read half of
			// macOS's on the first recording, which is how a 2.7× slower decay
			// still lands at the same distance.
			check("fling start (fit v0)", mg.FitV0, mn.FitV0, 0.15, "px/s")
			// And the native curve must actually be exponential for τ to mean
			// anything; a low R² here is a finding about the platform, not a
			// failure of gophics.
			if mn.TauR2 < 0.95 {
				t.Logf("note: native decay fit R²=%.3f — the platform curve is not a clean exponential", mn.TauR2)
			}
		})
	}
}
