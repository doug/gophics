//go:build windows && !js

package desktop

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The call itself has to work before anything it returns means anything. A
// mis-declared struct or a bad pointer shows up here rather than as a plausible
// wrong number later.
func TestReadPowerStatusSucceeds(t *testing.T) {
	s, ok := readPowerStatus()
	if !ok {
		t.Fatal("GetSystemPowerStatus failed")
	}
	// The API reports "unknown" in band; anything outside these is a decoding
	// error, most likely a struct-layout mistake.
	if s.ACLineStatus > 1 && s.ACLineStatus != 255 {
		t.Errorf("ACLineStatus = %d, want 0, 1 or 255", s.ACLineStatus)
	}
	if s.BatteryLifePercent > 100 && s.BatteryLifePercent != 255 {
		t.Errorf("BatteryLifePercent = %d, want 0..100 or 255", s.BatteryLifePercent)
	}
}

// TestBatteryMatchesWMI cross-checks against Windows' own view, reached by a
// different route. The risk in this file is not arithmetic — it is calling the
// API wrongly, and a wrong struct layout yields numbers that look reasonable.
//
// A VM usually reports no battery, which is a legitimate configuration and the
// one where Battery() must return nil rather than a fabricated full charge.
func TestBatteryMatchesWMI(t *testing.T) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		"$b = Get-CimInstance Win32_Battery; if ($b) { $b.EstimatedChargeRemaining } else { 'NONE' }").Output()
	if err != nil {
		t.Skipf("powershell unavailable: %v", err)
	}
	wmi := strings.TrimSpace(string(out))

	s, ok := readPowerStatus()
	if !ok {
		t.Fatal("GetSystemPowerStatus failed")
	}
	hasBattery := batteryPresent(s)

	if wmi == "NONE" || wmi == "" {
		if hasBattery {
			t.Errorf("GetSystemPowerStatus reports a battery (flag %d) but WMI sees none", s.BatteryFlag)
		}
		if (&window{}).Battery() != nil {
			t.Error("Battery() returned a capability on a machine with no battery")
		}
		t.Skip("no battery on this machine")
	}

	if !hasBattery {
		t.Fatalf("WMI reports a battery at %s%% but GetSystemPowerStatus does not", wmi)
	}
	pct, err := strconv.Atoi(wmi)
	if err != nil {
		t.Skipf("unexpected WMI output %q", wmi)
	}

	b := &windowsBattery{}
	got := b.Level()
	want := float32(pct) / 100
	// The two reads are moments apart and WMI rounds, so allow a point of drift.
	if d := got - want; d > 0.02 || d < -0.02 {
		t.Errorf("Level() = %.3f, WMI says %.3f", got, want)
	}
	if got < 0 || got > 1 {
		t.Errorf("Level() = %v, outside [0,1]", got)
	}
}

// Charging must agree with the AC line, which is what a caller deciding
// whether to warn about battery life actually needs.
func TestChargingFollowsACLine(t *testing.T) {
	s, ok := readPowerStatus()
	if !ok {
		t.Fatal("GetSystemPowerStatus failed")
	}
	if !batteryPresent(s) {
		t.Skip("no battery on this machine")
	}
	b := &windowsBattery{}
	if got, want := b.Charging(), s.ACLineStatus == acOnline; got != want {
		t.Errorf("Charging() = %v, ACLineStatus = %d", got, s.ACLineStatus)
	}
}

// The API reports "I don't know" in band, twice over, and both used to be
// read as facts: flag 255 (unknown status) was published as a battery, and
// percent 255 (unknown) was reported as 0% — a fabricated empty battery on
// exactly the machine whose driver could not say.
func TestUnknownIsNotAReading(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    byte
		present bool
	}{
		{"high", 1, true},
		{"low", 2, true},
		{"critical", 4, true},
		{"charging", 8, true},
		{"no battery", batteryFlagNone, false},
		{"unknown status", batteryFlagUnknown, false},
	} {
		if got := batteryPresent(systemPowerStatus{BatteryFlag: tc.flag}); got != tc.present {
			t.Errorf("batteryPresent(flag %d %s) = %v, want %v", tc.flag, tc.name, got, tc.present)
		}
	}

	for _, tc := range []struct {
		pct   byte
		want  float32
		known bool
	}{
		{0, 0, true},
		{57, 0.57, true},
		{100, 1, true},
		{120, 1, true}, // out of range clamps rather than exceeding full
		{batteryUnknownPc, 0, false},
	} {
		got, known := levelOf(systemPowerStatus{BatteryLifePercent: tc.pct})
		if known != tc.known || (known && (got-tc.want > 0.001 || tc.want-got > 0.001)) {
			t.Errorf("levelOf(%d) = %v, %v; want %v, %v", tc.pct, got, known, tc.want, tc.known)
		}
	}
}

// Repeated reads must stay stable; the syscall writes through a pointer into a
// Go struct on every call, and a mistake there drifts rather than failing.
func TestReadPowerStatusRepeatable(t *testing.T) {
	first, ok := readPowerStatus()
	if !ok {
		t.Fatal("GetSystemPowerStatus failed")
	}
	for i := range 200 {
		s, ok := readPowerStatus()
		if !ok {
			t.Fatalf("read %d failed", i)
		}
		if s.BatteryFlag != first.BatteryFlag {
			t.Fatalf("read %d: BatteryFlag changed from %d to %d", i, first.BatteryFlag, s.BatteryFlag)
		}
	}
}
