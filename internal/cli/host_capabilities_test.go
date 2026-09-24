package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two capabilities are published unconditionally on the Go side — Haptic and
// Lifecycle are never nil on a mobile Bridge — which makes them the host's
// responsibility: the bridge queues haptics for the host to play, and only
// the host knows when the app goes to the background. A scaffolded host that
// forgets either leaves a capability that is published and does nothing, the
// exact shape AGENTS.md forbids: ctx.Haptic().Play was silent on every
// scaffolded Android app, and ctx.Lifecycle() never left its initial state on
// either platform. The checked-in example hosts did both; the templates did
// not, and nothing compared them.
func TestScaffoldedHostsServeTheAlwaysPublishedCapabilities(t *testing.T) {
	for _, c := range []struct {
		tmpl  string
		calls []string
	}{{
		tmpl:  "templates/mobile/android/app/src/main/kotlin/MainActivity.kt.tmpl",
		calls: []string{"bridge.takeHaptic()", "bridge.setAppState(0)", "bridge.setAppState(1)", "bridge.setAppState(2)"},
	}, {
		tmpl:  "templates/mobile/ios/App/App.swift.tmpl",
		calls: []string{"bridge.takeHaptic()", "bridge.setAppState(0)", "bridge.setAppState(1)", "bridge.setAppState(2)"},
	}} {
		b, err := os.ReadFile(filepath.FromSlash(c.tmpl))
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range c.calls {
			if !strings.Contains(string(b), call) {
				t.Errorf("%s never calls %s; the capability it serves is always published and would silently do nothing",
					c.tmpl, call)
			}
		}
	}
}
