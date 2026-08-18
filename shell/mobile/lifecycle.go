package mobile

import "github.com/doug/gophics/shell"

// Lifecycle for embedded hosts. The host activity or scene drives it through
// Bridge.SetAppState; see the ladder in shell/lifecycle.go.
//
// This is the capability mobile needs most, because mobile is the only place
// the OS routinely kills a running app. "Persist before you are stopped" is
// what most apps mean when they ask for background support, and it needs no
// scheduler — only knowing the moment has come.

// mobileLifecycle holds the state and its subscribers.
//
// A mutex rather than the desktop's bare fields: SetAppState is called from the
// host's UI thread, which is not the goroutine that registered the callbacks.
// The generated PostedLifecycle wrapper marshals the callback itself, so this
// only has to keep its own state coherent.
type mobileLifecycle struct {
	b *Bridge
}

// Lifecycle makes the Bridge a shell.LifecycleWindow.
func (b *Bridge) Lifecycle() shell.Lifecycle { return mobileLifecycle{b: b} }

func (l mobileLifecycle) State() shell.AppState {
	l.b.lcMu.Lock()
	defer l.b.lcMu.Unlock()
	return l.b.lcState
}

func (l mobileLifecycle) OnChange(f func(shell.AppState)) {
	if f == nil {
		return
	}
	l.b.lcMu.Lock()
	defer l.b.lcMu.Unlock()
	l.b.lcSubs = append(l.b.lcSubs, f)
}

// SetAppState reports a run-state transition from the host.
//
// Android calls it from onResume (active), onPause (inactive) and onStop
// (background); iOS from sceneDidBecomeActive, sceneWillResignActive and
// sceneDidEnterBackground. Values outside the ladder are ignored rather than
// clamped — a host passing something unexpected is a wiring bug, and silently
// mapping it to "background" would pause an app that is running fine.
//
// Deliberately not derived from Bridge.Focused. That signal is window focus: it
// fires for a dialog appearing over the app, which is not the same as being
// backgrounded, and treating it as such would persist state far too eagerly.
func (b *Bridge) SetAppState(state int) {
	s := shell.AppState(state)
	if s != shell.StateActive && s != shell.StateInactive && s != shell.StateBackground {
		return
	}
	b.lcMu.Lock()
	if b.lcState == s {
		b.lcMu.Unlock()
		return
	}
	b.lcState = s
	subs := append([]func(shell.AppState){}, b.lcSubs...)
	b.lcMu.Unlock()

	// Called outside the lock: a subscriber that registers another would
	// otherwise deadlock.
	for _, f := range subs {
		f(s)
	}
}
