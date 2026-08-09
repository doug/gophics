//go:build !js

// Desktop implementation of the shell lifecycle capability
// (shell/lifecycle.go).
//
// This is real, not a stub: it is driven by the same window focus/blur routing
// the desktop shell already delivers as shell.Focus events (see desktop.go,
// app.OnFocus). A focused window is StateActive; a blurred one (another window
// or app frontmost) is StateInactive. Desktop apps are not "backgrounded" the
// way a mobile app or a hidden browser tab is — the OS keeps the process and its
// window alive — so StateBackground is intentionally never reported here; focus
// loss is the honest coarse signal a desktop can observe without deeper OS
// integration.

package desktop

import "github.com/doug/gophics/shell"

// desktopLifecycle holds the current run state and OnChange subscribers. All
// access is on the UI goroutine (the gogpu event loop): Lifecycle()/OnChange
// and the focus callback in Run both run there, so no locking is needed.
type desktopLifecycle struct {
	state shell.AppState
	cbs   []func(shell.AppState)
}

func newDesktopLifecycle() *desktopLifecycle {
	return &desktopLifecycle{state: shell.StateActive}
}

// Lifecycle makes the desktop window a shell.LifecycleWindow.
func (w *window) Lifecycle() shell.Lifecycle { return w.lc }

func (l *desktopLifecycle) State() shell.AppState { return l.state }

func (l *desktopLifecycle) OnChange(f func(shell.AppState)) {
	if f == nil {
		return
	}
	l.cbs = append(l.cbs, f)
}

// setFocused maps a focus/blur transition onto Active/Inactive and fans the new
// state out to subscribers on a change. Run's app.OnFocus callback calls it.
func (l *desktopLifecycle) setFocused(focused bool) {
	s := shell.StateActive
	if !focused {
		s = shell.StateInactive
	}
	if s == l.state {
		return
	}
	l.state = s
	for _, cb := range l.cbs {
		cb(s)
	}
}
