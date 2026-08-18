//go:build linux && !android && !js

// Linux implementation of the shell battery capability (shell/battery.go),
// reading sysfs directly.
//
// sysfs rather than upower over D-Bus: the numbers live in
// /sys/class/power_supply, they are plain text, and reading them needs no
// daemon, no session bus and no dependency. A headless box, a container and a
// laptop all answer the same way — and a machine with no battery has no
// Battery-type supply, which is exactly the nil-where-unsupported signal the
// capability wants.

package desktop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/doug/gophics/shell"
)

// powerSupplyDir is a var, not a const, so the tests can point it at a fake
// sysfs tree — the real one cannot be arranged to have a battery at 3% or to
// report "Unknown" on demand.
var powerSupplyDir = "/sys/class/power_supply"

// Battery makes the desktop window a shell.BatteryWindow. It returns nil when
// the machine exposes no battery — a desktop tower, or a container — so callers
// hide the affordance rather than showing a hardcoded 100%.
func (w *window) Battery() shell.Battery {
	if findBattery() == "" {
		return nil
	}
	return &linuxBattery{}
}

// findBattery returns the sysfs path of the first supply of type "Battery", or
// "" if there is none. Slot names are not stable across machines (BAT0, BAT1,
// CMB0 on some ThinkPads), and a supply may be a UPS or a wireless mouse, so
// match on the type file rather than the name.
func findBattery() string {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		dir := filepath.Join(powerSupplyDir, e.Name())
		if strings.TrimSpace(readFile(filepath.Join(dir, "type"))) != "Battery" {
			continue
		}
		// A supply can be present but have no charge reading (some UPS and
		// peripheral batteries); without capacity there is nothing to report.
		if readFile(filepath.Join(dir, "capacity")) == "" {
			continue
		}
		return dir
	}
	return ""
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

type linuxBattery struct {
	batteryWatcher
}

// Level reads the charge percentage. sysfs reports whole percent, so this is
// quantised to 0.01 — enough for any UI that shows a bar or a number.
func (b *linuxBattery) Level() float32 {
	dir := findBattery()
	if dir == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(readFile(filepath.Join(dir, "capacity"))))
	if err != nil {
		return 0
	}
	return float32(min(max(n, 0), 100)) / 100
}

// Charging reports whether the battery is gaining charge or running on external
// power. "Full" counts as charging: the machine is on mains, which is what a
// caller deciding whether to warn about battery life actually wants to know.
// "Not charging" does not — that is a plugged-in battery held at a charge limit.
func (b *linuxBattery) Charging() bool {
	dir := findBattery()
	if dir == "" {
		return false
	}
	switch strings.TrimSpace(readFile(filepath.Join(dir, "status"))) {
	case "Charging", "Full":
		return true
	case "Discharging", "Not charging":
		return false
	}
	// Status is "Unknown" on some hardware; fall back to whether any mains
	// supply reports itself online.
	return mainsOnline()
}

// mainsOnline reports whether any non-battery supply is plugged in.
func mainsOnline() bool {
	entries, err := os.ReadDir(powerSupplyDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		dir := filepath.Join(powerSupplyDir, e.Name())
		if strings.TrimSpace(readFile(filepath.Join(dir, "type"))) == "Battery" {
			continue
		}
		if strings.TrimSpace(readFile(filepath.Join(dir, "online"))) == "1" {
			return true
		}
	}
	return false
}

// OnChange registers f, called when the level or charging state changes.
func (b *linuxBattery) OnChange(f func()) {
	b.watch(f, func() (float32, bool) { return b.Level(), b.Charging() })
}
