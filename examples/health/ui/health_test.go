package healthui

import (
	"testing"

	"github.com/doug/gophics/apptest"
)

// healthApp mounts the dashboard over p (nil → the synthetic live source).
func healthApp(t *testing.T, p Provider) *apptest.App {
	t.Helper()
	a := apptest.New(t, App{Provider: p}, apptest.WithConfig(Config()))
	a.Render()
	return a
}

// frames advances the app by n frames. Settle is not usable on the
// dashboard: the live source ticks for as long as it is showing.
func frames(a *apptest.App, n int) {
	for range n {
		a.Step(1.0 / 60)
		a.Render()
	}
}

// Every card → back cycle used to leave one more ticker registered: the
// detail page added itself in Init and nothing removed it, so an unmounted
// page kept ticking — and pinning the frame loop — forever.
func TestNavigationLeaksNoTickers(t *testing.T) {
	t.Setenv("HEALTH_VIEW", "dashboard")
	a := healthApp(t, nil)
	a.AssertText("Heart Rate")
	base := a.Owner().TickerCount()

	for round := 1; round <= 3; round++ {
		a.TapText("Heart Rate")
		frames(a, 60)
		if !a.HasText("Back") {
			t.Fatalf("round %d: detail page not pushed; labels=%v", round, a.Labels())
		}
		a.TapText("Back")
		frames(a, 60)
		if a.HasText("Back") {
			t.Fatalf("round %d: detail page not popped; labels=%v", round, a.Labels())
		}
	}
	if got := a.Owner().TickerCount(); got != base {
		t.Errorf("tickers after 3 push/pop cycles = %d, want %d — a page outlived its widget", got, base)
	}
}

// The detail page has no ticker of its own, so this is the check that it
// still follows the live source: the root's per-frame rebuild must reach a
// pushed page, or the number on it freezes at the moment it was opened.
func TestDetailFollowsTheLiveSource(t *testing.T) {
	t.Setenv("HEALTH_VIEW", "dashboard")
	p := newSynthProvider()
	a := healthApp(t, p)
	a.TapText("Steps")
	frames(a, 30)
	a.AssertText("Back")

	before, _ := p.Latest(Steps)
	a.AssertText(fmtInt(before.V))
	frames(a, 120) // two seconds: the synthetic step count climbs every second
	after, _ := p.Latest(Steps)
	if after.V == before.V {
		t.Fatal("the synthetic source did not advance under the detail page")
	}
	if !a.HasText(fmtInt(after.V)) {
		t.Errorf("detail still shows %s after the source moved to %s", fmtInt(before.V), fmtInt(after.V))
	}
}

// A source the user has not let the app read gets a no-access screen — not a
// dashboard of four cards reading 0, which is what a person with no data would
// see. When the host reports the grant, the dashboard appears without a tap:
// the provider's OnChange is the repaint.
func TestUnauthorizedProviderShowsNoAccess(t *testing.T) {
	p := NewDeviceProvider("Health Connect")
	a := healthApp(t, p)
	a.AssertText("Connect Health Connect")

	a.TapText("Connect Health Connect")
	a.Render()
	if a.HasText("Heart Rate") {
		t.Fatalf("dashboard shown for a provider that reports no access; labels=%v", a.Labels())
	}
	a.AssertText("No access to Health Connect")

	p.SetAuthorized(true) // the host's permission callback
	a.Render()
	a.AssertText("Heart Rate")
	if a.HasText("No access") {
		t.Error("no-access screen still up after the grant")
	}
}

// A store that is not on the device is told apart from one the user has not
// opened up. The no-access screen used to ask for a permission in a Health
// Connect that was not installed, which nobody could follow.
func TestUnavailableProviderDoesNotAskForPermission(t *testing.T) {
	p := NewDeviceProvider("Health Connect")
	p.SetUnavailable() // the host, finding no Health Connect
	a := healthApp(t, p)
	a.TapText("Connect Health Connect")
	a.Render()

	a.AssertText("Health Connect is not available")
	if a.HasText("Allow this app") || a.HasText("No access") {
		t.Errorf("asked for a permission in a store that is not there; labels=%v", a.Labels())
	}
	if a.HasText("Heart Rate") {
		t.Error("dashboard shown with no store to read")
	}
}

// A host push repaints the dashboard by itself; the UI no longer polls.
func TestDevicePushRepaints(t *testing.T) {
	p := NewDeviceProvider("Apple Health")
	p.SetAuthorized(true)
	a := healthApp(t, p)
	a.TapText("Connect Apple Health")
	a.Render()
	a.AssertText("Heart Rate")

	p.Push(Steps, 9, 4321, 0)
	a.Render()
	a.AssertText("4,321")
}

// Only a live source that is actually showing keeps the frame loop awake. The
// onboarding screen and a device-fed dashboard have nothing to animate, and a
// ticker that reported true there ran a full rebuild per frame on a phone.
func TestIdleScreensLetTheFrameLoopSleep(t *testing.T) {
	a := healthApp(t, nil)
	if a.Step(1.0 / 60) {
		t.Error("the onboarding screen keeps ticking")
	}
	a.TapText("Connect Sample data")
	a.Render()
	if !a.Step(1.0 / 60) {
		t.Error("the live dashboard stopped ticking")
	}

	p := NewDeviceProvider("Apple Health")
	p.SetAuthorized(true)
	d := healthApp(t, p)
	d.TapText("Connect Apple Health")
	d.Render()
	if d.Step(1.0 / 60) {
		t.Error("a device-fed dashboard keeps ticking with nothing to advance")
	}
}
