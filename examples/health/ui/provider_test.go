package healthui

import (
	"sync"
	"testing"
)

// TestSynthProviderDeterministic verifies the synthetic provider is seeded
// deterministically (so thumbnails and any golden tests are stable) and streams
// forward under Advance.
func TestSynthProviderDeterministic(t *testing.T) {
	a, b := newSynthProvider(), newSynthProvider()
	if got, want := a.Series(HeartRate)[0].V, b.Series(HeartRate)[0].V; got != want {
		t.Fatalf("synthProvider not deterministic: %v vs %v", got, want)
	}
	before := len(a.Series(HeartRate))
	for range 300 { // ~5s at 60fps
		a.Advance(1.0 / 60.0)
	}
	if lat, ok := a.Latest(HeartRate); !ok || lat.V < 30 || lat.V > 220 {
		t.Fatalf("heart rate out of range after Advance: %+v ok=%v", lat, ok)
	}
	// The rolling window stays bounded, not unboundedly growing.
	if grew := len(a.Series(HeartRate)); grew > before+5 {
		t.Fatalf("HR window unbounded: %d → %d", before, grew)
	}
}

// TestDeviceProviderPush covers the native-facing path: Push appends and bounds
// history, Steps reports the cumulative latest, and unknown metrics are ignored.
func TestDeviceProviderPush(t *testing.T) {
	d := NewDeviceProvider("Apple Health")
	if d.Name() != "Apple Health" || d.Authorized() {
		t.Fatalf("unexpected initial state: name=%q authed=%v", d.Name(), d.Authorized())
	}
	d.SetAuthorized(true)
	if !d.Authorized() {
		t.Fatal("SetAuthorized(true) did not stick")
	}

	for i := range 10 {
		d.Push(HeartRate, float64(i), 60+float64(i), 5) // cap 5
	}
	if got := len(d.Series(HeartRate)); got != 5 {
		t.Fatalf("cap not enforced: len=%d want 5", got)
	}
	if lat, ok := d.Latest(HeartRate); !ok || lat.V != 69 {
		t.Fatalf("latest HR = %+v ok=%v want V=69", lat, ok)
	}

	d.Push(Steps, 14, 8432, 0)
	if lat, ok := d.Latest(Steps); !ok || lat.V != 8432 {
		t.Fatalf("steps latest = %+v want 8432", lat)
	}

	d.ReplaceSeries(Weight, []Sample{{T: -1, V: 75.5}, {T: 0, V: 75.1}})
	if lat, _ := d.Latest(Weight); lat.V != 75.1 {
		t.Fatalf("weight latest = %v want 75.1", lat.V)
	}

	d.Push(Metric(99), 0, 0, 0) // out of range: must not panic
}

// TestDeviceProviderConcurrent ensures Push (host threads) and Series/Latest
// (frame thread) are race-free — run with -race.
func TestDeviceProviderConcurrent(t *testing.T) {
	d := NewDeviceProvider("Health Connect")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			d.Push(HeartRate, float64(i), 70, 64)
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			_ = d.Series(HeartRate)
			_, _ = d.Latest(HeartRate)
		}
	}()
	wg.Wait()
}

var _ Provider = (*DeviceProvider)(nil)
