//go:build !js

package desktop

import (
	"sync"
	"time"
)

// batteryPoll is how often the OS is re-asked once something is watching.
//
// Battery state moves over minutes, and each sample is cheap on every platform
// here — two small reads from a virtual filesystem on Linux, one syscall on
// Windows, a cached IOKit copy on macOS. Each OS does offer a real
// notification (udev netlink, WM_POWERBROADCAST, an IOKit run-loop source), and
// each wants a listener this layer does not own; polling a cheap sample is the
// honest trade until one of them is worth its own plumbing.
const batteryPoll = 30 * time.Second

// batteryWatcher is the shared "poll, and notify when something changed" half
// of the battery capability.
//
// The three platform implementations differ only in how they take a sample —
// sysfs, GetSystemPowerStatus, IOKit — and had identical copies of this logic
// around it. Sharing it means the debouncing rule is decided once: callbacks
// fire on a real change, never on an unchanged poll, so a UI bound to battery
// level does not rebuild every thirty seconds for nothing.
type batteryWatcher struct {
	mu      sync.Mutex
	watches []func()
	polling bool
}

// sampler reports the current level and charging state.
type sampler func() (level float32, charging bool)

// watch registers f, starting the poll on the first registration. The goroutine
// then runs for the life of the process, which is the same lifetime as the
// window that published the capability.
func (w *batteryWatcher) watch(f func(), s sampler) {
	if f == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.watches = append(w.watches, f)
	if w.polling {
		return
	}
	w.polling = true
	go w.poll(s)
}

func (w *batteryWatcher) poll(s sampler) {
	level, charging := s()
	for range time.Tick(batteryPoll) {
		nl, nc := s()
		if nl == level && nc == charging {
			continue
		}
		level, charging = nl, nc

		// Copy under the lock, then call outside it: a callback that registers
		// another watcher would otherwise deadlock.
		w.mu.Lock()
		watches := append([]func(){}, w.watches...)
		w.mu.Unlock()
		for _, f := range watches {
			f()
		}
	}
}
