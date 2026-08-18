//go:build windows && !js

// Windows implementation of the shell battery capability (shell/battery.go),
// calling kernel32!GetSystemPowerStatus through x/sys/windows — a plain
// syscall, no CGo.

package desktop

import (
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
	acOnline         = 1
	batteryFlagNone  = 128 // no system battery
	batteryUnknownPc = 255 // BatteryLifePercent when unknown
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
	if !ok || s.BatteryFlag == batteryFlagNone {
		return nil
	}
	return &windowsBattery{}
}

type windowsBattery struct {
	batteryWatcher
}

// Level is the charge fraction, or 0 when Windows reports it as unknown.
func (b *windowsBattery) Level() float32 {
	s, ok := readPowerStatus()
	if !ok || s.BatteryLifePercent == batteryUnknownPc {
		return 0
	}
	return float32(min(s.BatteryLifePercent, 100)) / 100
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
