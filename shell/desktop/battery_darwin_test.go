//go:build darwin && !ios && !js

package desktop

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/objc"
)

// TestReadPowerSourceMatchesPmset checks the IOKit path against pmset, which
// reads the same power sources through a different interface.
//
// A self-consistency check would not be worth much here: the risk in this file
// is not arithmetic, it is calling the C API wrongly — a mis-declared
// signature, a key that does not exist, an unowned reference released. Any of
// those produce a plausible-looking number (or a crash), so the assertion has
// to come from outside.
//
// Skips on a Mac with no battery, which is a legitimate configuration and the
// one where Battery() correctly returns nil.
func TestReadPowerSourceMatchesPmset(t *testing.T) {
	got := readPowerSource()

	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		t.Skipf("pmset unavailable: %v", err)
	}
	text := string(out)

	// "Now drawing from 'AC Power'" / "'Battery Power'"
	wantCharging := strings.Contains(text, "'AC Power'")

	m := regexp.MustCompile(`(\d+)%`).FindStringSubmatch(text)
	if m == nil {
		if got.present {
			t.Errorf("IOKit reports a battery but pmset shows no percentage:\n%s", text)
		}
		t.Skip("no battery on this machine")
	}
	if !got.present {
		t.Fatalf("pmset reports a battery but IOKit found none:\n%s", text)
	}

	pct, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	wantLevel := float32(pct) / 100

	// pmset rounds to whole percent and the two reads are moments apart, so
	// allow a percentage point of drift rather than demanding equality.
	if d := got.level - wantLevel; d > 0.01 || d < -0.01 {
		t.Errorf("Level() = %.3f, pmset says %.3f", got.level, wantLevel)
	}
	if got.charging != wantCharging {
		t.Errorf("Charging() = %v, pmset says %v\n%s", got.charging, wantCharging, text)
	}
	if got.level < 0 || got.level > 1 {
		t.Errorf("Level() = %v, outside [0,1]", got.level)
	}
}

// TestReadPowerSourceRepeatable runs the read many times. The FFI path copies
// two CoreFoundation objects per call and releases both; a mistake there —
// releasing something not owned, or failing to release something that is —
// shows up as a crash or as drift, not as a wrong first answer.
func TestReadPowerSourceRepeatable(t *testing.T) {
	first := readPowerSource()
	if !first.present {
		t.Skip("no battery on this machine")
	}
	for i := range 200 {
		r := readPowerSource()
		if !r.present {
			t.Fatalf("read %d: battery disappeared", i)
		}
		if d := r.level - first.level; d > 0.02 || d < -0.02 {
			t.Fatalf("read %d: level drifted from %.3f to %.3f", i, first.level, r.level)
		}
	}
}

// fakeDescription builds an NSDictionary shaped like a power-source
// description. IOKit hands back a CFDictionary, which is the same object, so
// the extraction path does not know the difference.
func fakeDescription(t *testing.T, current, max int64, state string) objc.ID {
	t.Helper()
	if !loadIOKit() {
		t.Skip("IOKit unavailable")
	}
	d := objc.Class("NSMutableDictionary").Send("dictionary")
	if !d.Valid() {
		t.Fatal("could not create NSMutableDictionary")
	}
	num := func(v int64) objc.ID {
		return objc.Class("NSNumber").Send("numberWithInteger:", objc.Int(v))
	}
	d.SendVoid("setObject:forKey:", objc.Obj(num(current)), objc.Obj(keyCurrent))
	d.SendVoid("setObject:forKey:", objc.Obj(num(max)), objc.Obj(keyMax))
	d.SendVoid("setObject:forKey:", objc.Obj(objc.String(state)), objc.Obj(keyState))
	return d
}

// TestReadDescription covers the part of the IOKit path a machine without a
// battery would never run: the key lookups, the integer conversion and the
// mains comparison. These are the failures that produce a plausible wrong
// number rather than a crash.
func TestReadDescription(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cur, max     int64
		state        string
		wantLevel    float32
		wantCharging bool
		wantOK       bool
	}{
		{"half on battery", 2500, 5000, "Battery Power", 0.5, false, true},
		{"full on mains", 5000, 5000, "AC Power", 1, true, true},
		{"empty", 0, 5000, "Battery Power", 0, false, true},
		{"over-range clamps", 6000, 5000, "AC Power", 1, true, true},
		{"zero max is not a battery", 50, 0, "AC Power", 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := readDescription(fakeDescription(t, tc.cur, tc.max, tc.state))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got.level != tc.wantLevel {
				t.Errorf("level = %v, want %v", got.level, tc.wantLevel)
			}
			if got.charging != tc.wantCharging {
				t.Errorf("charging = %v, want %v", got.charging, tc.wantCharging)
			}
		})
	}
}

// A dictionary missing the capacity keys must be rejected rather than read as
// a battery at 0% — that is how a UPS or a wireless mouse shows up.
func TestReadDescriptionRejectsNonBattery(t *testing.T) {
	if !loadIOKit() {
		t.Skip("IOKit unavailable")
	}
	d := objc.Class("NSMutableDictionary").Send("dictionary")
	d.SendVoid("setObject:forKey:", objc.Obj(objc.String("AC Power")), objc.Obj(keyState))
	if _, ok := readDescription(d); ok {
		t.Error("a description with no capacity keys was accepted as a battery")
	}
	if _, ok := readDescription(0); ok {
		t.Error("a nil description was accepted as a battery")
	}
}

// The cache must not stop the first read from working, and must return the
// same values within its window.
func TestBatterySampleCaches(t *testing.T) {
	if !readPowerSource().present {
		t.Skip("no battery on this machine")
	}
	b := &darwinBattery{}
	l1, c1 := b.Level(), b.Charging()
	l2, c2 := b.Level(), b.Charging()
	if l1 != l2 || c1 != c2 {
		t.Errorf("cached sample changed within the TTL: %v/%v then %v/%v", l1, c1, l2, c2)
	}
	if l1 <= 0 || l1 > 1 {
		t.Errorf("Level() = %v, want a real fraction", l1)
	}
}
