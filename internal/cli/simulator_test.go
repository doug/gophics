package cli

import "testing"

// simctl lists devices per runtime in a JSON object, which arrives as a Go
// map, and map iteration is randomised: with nothing booted, `gophics run -p
// ios` picked a different simulator on each invocation. The pick is now the
// first iPhone of the newest runtime, compared numerically so iOS 18 beats
// iOS 9 — as a string it does not.
func TestPickFromSimulatorsIsDeterministic(t *testing.T) {
	devices := map[string][]simDevice{
		"com.apple.CoreSimulator.SimRuntime.iOS-9-3": {
			{UDID: "old", Name: "iPhone 6s", State: "Shutdown"},
		},
		"com.apple.CoreSimulator.SimRuntime.iOS-17-5": {
			{UDID: "mid-pad", Name: "iPad Air", State: "Shutdown"},
			{UDID: "mid", Name: "iPhone 15", State: "Shutdown"},
		},
		"com.apple.CoreSimulator.SimRuntime.iOS-18-0": {
			{UDID: "new-pad", Name: "iPad Pro", State: "Shutdown"},
			{UDID: "new", Name: "iPhone 16", State: "Shutdown"},
		},
	}
	for i := 0; i < 20; i++ {
		udid, name, ok := pickFromSimulators(devices)
		if !ok || udid != "new" {
			t.Fatalf("run %d: picked %q (%s), want the iPhone on the newest runtime", i, udid, name)
		}
	}

	// A booted simulator wins whatever its runtime: it is the one the
	// developer is looking at.
	devices["com.apple.CoreSimulator.SimRuntime.iOS-9-3"][0].State = "Booted"
	if udid, _, _ := pickFromSimulators(devices); udid != "old" {
		t.Errorf("picked %q over the booted simulator", udid)
	}

	if _, _, ok := pickFromSimulators(map[string][]simDevice{
		"com.apple.CoreSimulator.SimRuntime.iOS-18-0": {{UDID: "p", Name: "iPad Pro"}},
	}); ok {
		t.Error("an iPad-only list reported an iPhone")
	}
}

func TestRuntimeNewerComparesNumerically(t *testing.T) {
	const p = "com.apple.CoreSimulator.SimRuntime."
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{p + "iOS-18-0", p + "iOS-9-3", true},
		{p + "iOS-9-3", p + "iOS-18-0", false},
		{p + "iOS-17-5", p + "iOS-17-4", true},
		{p + "iOS-17-5", p + "iOS-17-5", false},
		{p + "iOS-26-0", p + "iOS-18-0", true},
	} {
		if got := runtimeNewer(c.a, c.b); got != c.want {
			t.Errorf("runtimeNewer(%s, %s) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
