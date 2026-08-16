//go:build linux

package platform

import (
	"os"
	"strings"
	"testing"
)

// TestA11yBusAddressWithoutSessionBus is the case that actually ships: a
// machine with no session bus must answer "no accessibility here" rather than
// erroring, because that answer is what makes ctx.Accessibility() correctly
// nil instead of a capability that silently discards everything.
func TestA11yBusAddressWithoutSessionBus(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	addr, err := a11yBusAddress()
	if err != nil {
		t.Errorf("err = %v, want nil when there is no session bus", err)
	}
	if addr != "" {
		t.Errorf("addr = %q, want empty", addr)
	}
	if a11yBusReachable() {
		t.Error("a11yBusReachable() = true with no session bus")
	}
}

// An address that cannot be dialled must be reported as unreachable rather
// than hanging or panicking.
func TestA11yBusUnreachableAddress(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/gophics-test-socket")
	if a11yBusReachable() {
		t.Error("a11yBusReachable() = true for a socket that does not exist")
	}
}

// TestA11yBusReachableOnRealBus is the spike proper: on a machine that has a
// session bus and at-spi2 installed, we must be able to find the accessibility
// bus and complete the auth + Hello handshake on it.
//
// That handshake is the whole feasibility question. AT-SPI is a D-Bus protocol
// rather than a C API, so if a pure-Go peer can register on the accessibility
// bus, the rest is protocol surface — exporting objects and answering calls —
// not a question of whether it can be done without CGo.
//
// Skips where there is no bus, which includes CI.
func TestA11yBusReachableOnRealBus(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}
	addr, err := a11yBusAddress()
	if err != nil {
		t.Fatalf("a11yBusAddress: %v", err)
	}
	if addr == "" {
		t.Skip("no org.a11y.Bus on this machine (at-spi2 not running)")
	}
	if !strings.HasPrefix(addr, "unix:") {
		t.Errorf("accessibility bus address = %q, want a unix transport", addr)
	}
	t.Logf("accessibility bus: %s", addr)

	if !a11yBusReachable() {
		t.Error("found the accessibility bus address but could not complete auth + Hello")
	}
}
