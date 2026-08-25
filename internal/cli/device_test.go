package cli

import "testing"

// devicectl reports two identifiers per device and they are not the same
// value: "identifier" is the handle devicectl itself takes, while
// hardwareProperties.udid is what xcodebuild's -destination id= expects.
// Handing either tool the other one's value fails, so both are carried.
const devicesJSON = `{"result":{"devices":[
 {"identifier":"AAAA-1111","deviceProperties":{"name":"Unpaired phone"},
  "hardwareProperties":{"udid":"00008110-AAAA","platform":"iOS"},
  "connectionProperties":{"tunnelState":"connected","pairingState":"unpaired"}},
 {"identifier":"BBBB-2222","deviceProperties":{"name":"A watch"},
  "hardwareProperties":{"udid":"00008110-BBBB","platform":"watchOS"},
  "connectionProperties":{"tunnelState":"connected","pairingState":"paired"}},
 {"identifier":"CCCC-3333","deviceProperties":{"name":"Paired but idle"},
  "hardwareProperties":{"udid":"00008110-CCCC","platform":"iOS"},
  "connectionProperties":{"tunnelState":"disconnected","pairingState":"paired"}},
 {"identifier":"DDDD-4444","deviceProperties":{"name":"Live phone"},
  "hardwareProperties":{"udid":"00008110-DDDD","platform":"iOS"},
  "connectionProperties":{"tunnelState":"connected","pairingState":"paired"}}
]}}`

func TestSelectDevicePrefersAConnectedPairedPhone(t *testing.T) {
	got, err := selectDevice([]byte(devicesJSON))
	if err != nil {
		t.Fatal(err)
	}
	if got.name != "Live phone" {
		t.Errorf("picked %q; want the connected paired iOS device", got.name)
	}
	// The two identifiers must not be conflated: each tool gets its own.
	if got.identifier != "DDDD-4444" {
		t.Errorf("identifier %q, want DDDD-4444 (devicectl's handle)", got.identifier)
	}
	if got.udid != "00008110-DDDD" {
		t.Errorf("udid %q, want 00008110-DDDD (xcodebuild's -destination id=)", got.udid)
	}
}

func TestSelectDeviceFallsBackToAPairedButIdleOne(t *testing.T) {
	// The same list without the live phone: a paired device whose tunnel is
	// down is still usable — devicectl brings it up — so it beats nothing.
	trimmed := `{"result":{"devices":[
 {"identifier":"CCCC-3333","deviceProperties":{"name":"Paired but idle"},
  "hardwareProperties":{"udid":"00008110-CCCC","platform":"iOS"},
  "connectionProperties":{"tunnelState":"disconnected","pairingState":"paired"}}
]}}`
	got, err := selectDevice([]byte(trimmed))
	if err != nil {
		t.Fatal(err)
	}
	if got.name != "Paired but idle" {
		t.Errorf("picked %q", got.name)
	}
}

func TestSelectDeviceRefusesUnpairedAndNonPhones(t *testing.T) {
	// A connected watch and an unpaired phone are both present, and neither
	// is something an iOS app can be installed on from here.
	only := `{"result":{"devices":[
 {"identifier":"AAAA-1111","deviceProperties":{"name":"Unpaired phone"},
  "hardwareProperties":{"udid":"00008110-AAAA","platform":"iOS"},
  "connectionProperties":{"tunnelState":"connected","pairingState":"unpaired"}},
 {"identifier":"BBBB-2222","deviceProperties":{"name":"A watch"},
  "hardwareProperties":{"udid":"00008110-BBBB","platform":"watchOS"},
  "connectionProperties":{"tunnelState":"connected","pairingState":"paired"}}
]}}`
	if got, err := selectDevice([]byte(only)); err == nil {
		t.Errorf("picked %q; want an error naming what to do", got.name)
	}
}

func TestSelectDeviceReportsNoDevices(t *testing.T) {
	if _, err := selectDevice([]byte(`{"result":{"devices":[]}}`)); err == nil {
		t.Error("empty device list should be an error, not a zero device")
	}
}

// devicectl has no flag for "set this variable in the app": it forwards
// whatever it finds in its own environment under DEVICECTL_CHILD_. Forwarding
// GOPHICS_* that way is the only route a knob like GOPHICS_PACING has to a
// process running on a phone.
func TestChildEnvForwardsOnlyGophicsVars(t *testing.T) {
	got := childEnv([]string{"PATH=/usr/bin", "GOPHICS_PACING=1", "HOME=/x", "GOPHICS_NO_DAMAGE=1"})
	want := map[string]bool{
		"DEVICECTL_CHILD_GOPHICS_PACING=1":    true,
		"DEVICECTL_CHILD_GOPHICS_NO_DAMAGE=1": true,
	}
	seen := map[string]bool{}
	for _, kv := range got {
		if len(kv) > 16 && kv[:16] == "DEVICECTL_CHILD_" {
			seen[kv] = true
		}
	}
	for w := range want {
		if !seen[w] {
			t.Errorf("missing %q", w)
		}
	}
	for s := range seen {
		if !want[s] {
			t.Errorf("forwarded %q, which is not a gophics variable", s)
		}
	}
	// Only the additions: run appends these to the real environment, so
	// returning a copy of it here would duplicate every variable.
	if len(got) != 2 {
		t.Errorf("got %d entries, want just the 2 forwarded", len(got))
	}
}
