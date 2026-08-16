//go:build linux && !js

package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeSupply writes one /sys/class/power_supply-shaped directory.
func fakeSupply(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for k, v := range files {
		if err := os.WriteFile(filepath.Join(dir, k), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// withSysfs points the reader at a fake tree for one test.
func withSysfs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := powerSupplyDir
	powerSupplyDir = root
	t.Cleanup(func() { powerSupplyDir = old })
	return root
}

func TestBatteryLevelAndCharging(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "42\n", "status": "Discharging\n",
	})

	b := &linuxBattery{}
	if got := b.Level(); got != 0.42 {
		t.Errorf("Level() = %v, want 0.42", got)
	}
	if b.Charging() {
		t.Error("Charging() = true while discharging")
	}
}

// "Full" means the machine is on mains, which is what a caller deciding
// whether to warn about battery life needs to know.
func TestBatteryFullCountsAsCharging(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "100\n", "status": "Full\n",
	})
	if b := (&linuxBattery{}); !b.Charging() {
		t.Error("Charging() = false when status is Full")
	}
}

// "Not charging" is a plugged-in battery held at a charge limit — common on
// ThinkPads — and must not be confused with Full.
func TestBatteryNotChargingIsNotCharging(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "60\n", "status": "Not charging\n",
	})
	if b := (&linuxBattery{}); b.Charging() {
		t.Error(`Charging() = true for status "Not charging"`)
	}
}

// Some hardware reports Unknown; fall back to whether mains is online.
func TestBatteryUnknownStatusFallsBackToMains(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "55\n", "status": "Unknown\n",
	})
	fakeSupply(t, root, "AC0", map[string]string{"type": "Mains\n", "online": "1\n"})
	if b := (&linuxBattery{}); !b.Charging() {
		t.Error("Charging() = false with Unknown status and mains online")
	}

	root2 := withSysfs(t)
	fakeSupply(t, root2, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "55\n", "status": "Unknown\n",
	})
	fakeSupply(t, root2, "AC0", map[string]string{"type": "Mains\n", "online": "0\n"})
	if b := (&linuxBattery{}); b.Charging() {
		t.Error("Charging() = true with Unknown status and mains offline")
	}
}

// A machine with no battery must report none, so ctx.Battery() is nil and the
// UI hides the affordance instead of showing a made-up value.
func TestNoBatteryFound(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "AC0", map[string]string{"type": "Mains\n", "online": "1\n"})
	if dir := findBattery(); dir != "" {
		t.Errorf("findBattery() = %q on a machine with only mains", dir)
	}
}

// Slot names vary (BAT0, BAT1, CMB0), and a supply may be a mouse or a UPS, so
// selection is by type — and by having a charge reading at all.
func TestFindBatterySkipsSuppliesWithoutCapacity(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "hidpp_battery_0", map[string]string{"type": "Battery\n"})
	fakeSupply(t, root, "CMB0", map[string]string{
		"type": "Battery\n", "capacity": "77\n", "status": "Discharging\n",
	})
	if got := findBattery(); filepath.Base(got) != "CMB0" {
		t.Errorf("findBattery() = %q, want the supply that reports capacity", got)
	}
}

// sysfs occasionally reports out-of-range values; clamp rather than emit a
// level above 1 that a progress bar would draw off the end.
func TestBatteryLevelClamped(t *testing.T) {
	root := withSysfs(t)
	fakeSupply(t, root, "BAT0", map[string]string{
		"type": "Battery\n", "capacity": "137\n", "status": "Charging\n",
	})
	if got := (&linuxBattery{}).Level(); got != 1 {
		t.Errorf("Level() = %v, want 1 for an over-range capacity", got)
	}
}
