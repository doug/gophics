//go:build windows && !js

// Windows implementation of the shell battery capability (shell/battery.go),
// calling kernel32!GetSystemPowerStatus through x/sys/windows — a plain
// syscall, no CGo.

package desktop

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/doug/gophics/shell"
)

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemPowerStat = kernel32.NewProc("GetSystemPowerStatus")
)

// systemPowerStatus mirrors the Win32 SYSTEM_POWER_STATUS struct. The four
// leading bytes are not padded up to word alignment by the API, so the layout
// here is byte-exact on purpose.
type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

// Sentinels from the Win32 docs: the API reports "I don't know" in-band.
const (
	acOnline           = 1
	batteryFlagNone    = 128 // no system battery
	batteryFlagUnknown = 255 // "unable to read the battery flag information"
	batteryUnknownPc   = 255 // BatteryLifePercent when unknown
)

func readPowerStatus() (systemPowerStatus, bool) {
	var s systemPowerStatus
	r, _, _ := procGetSystemPowerStat.Call(uintptr(unsafe.Pointer(&s)))
	return s, r != 0
}

// Battery makes the desktop window a shell.BatteryWindow. It returns nil on a
// machine with no battery — a desktop or a VM — so callers hide the affordance
// instead of showing a fabricated full charge.
func (w *window) Battery() shell.Battery {
	s, ok := readPowerStatus()
	if !ok || !batteryPresent(s) {
		return nil
	}
	return &windowsBattery{last: 1}
}

// batteryPresent reads the flag the way the docs mean it: 128 is "no system
// battery" and 255 is "unknown status", and a driver that cannot say whether
// there is a battery must not be published as one — the capability's nil is
// how an app learns there is nothing to show.
func batteryPresent(s systemPowerStatus) bool {
	return s.BatteryFlag != batteryFlagNone && s.BatteryFlag != batteryFlagUnknown
}

type windowsBattery struct {
	batteryWatcher

	mu   sync.Mutex
	last float32 // the last level Windows actually knew; see Level
}

// Level is the charge fraction. When Windows reports the percentage as
// unknown, this returns the last value it did know rather than 0: an unknown
// used to read as an empty battery, which is the fabricated reading the file
// exists to avoid, and the watcher would have announced a drop to 0% on every
// blip. Before any reading, it is a full battery — the answer that raises no
// alarm, matching Charging's reasoning that a battery on mains is not a worry.
func (b *windowsBattery) Level() float32 {
	s, ok := readPowerStatus()
	b.mu.Lock()
	defer b.mu.Unlock()
	if lvl, known := levelOf(s); ok && known {
		b.last = lvl
	}
	return b.last
}

// levelOf converts the raw percentage, reporting false for the in-band
// "unknown" so the caller can keep what it had.
func levelOf(s systemPowerStatus) (float32, bool) {
	if s.BatteryLifePercent == batteryUnknownPc {
		return 0, false
	}
	return float32(min(s.BatteryLifePercent, 100)) / 100, true
}

// Charging reports mains power rather than current-into-the-cell, matching the
// other platforms: a caller asking this wants to know whether to worry about
// running out, and a full battery on mains is not a worry.
func (b *windowsBattery) Charging() bool {
	s, ok := readPowerStatus()
	return ok && s.ACLineStatus == acOnline
}

// OnChange registers f, called when the level or charging state changes.
func (b *windowsBattery) OnChange(f func()) {
	b.watch(f, func() (float32, bool) { return b.Level(), b.Charging() })
}
