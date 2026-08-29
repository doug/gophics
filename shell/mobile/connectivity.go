package mobile

import "github.com/doug/gophics/shell"

// Connectivity, Battery and Links share a shape: the platform already knows the
// answer and changes it on its own schedule, so the host pushes state in and
// the capability reads a field. None of them needs a host interface or a
// request id — a Go-side request would only ever be answered with what the last
// push said.
//
// Each starts out unknown rather than optimistic. A phone that has not yet been
// told is not "online"; reporting it that way makes an app skip the offline
// path exactly once, at launch, which is the moment it matters most.

// Connectivity makes the Bridge a shell.ConnectivityWindow.
func (b *Bridge) Connectivity() shell.Connectivity {
	if !b.netKnown {
		return nil // the host has not wired SetOnline; say so rather than guess
	}
	return mobileConnectivity{b}
}

type mobileConnectivity struct{ b *Bridge }

func (c mobileConnectivity) Online() bool { return c.b.online }

func (c mobileConnectivity) OnChange(fn func(online bool)) {
	c.b.netSubs = append(c.b.netSubs, fn)
}

// SetOnline delivers reachability. Hosts call it once at startup and on every
// change (iOS NWPathMonitor, Android ConnectivityManager.NetworkCallback).
func (b *Bridge) SetOnline(online bool) {
	first := !b.netKnown
	b.netKnown = true
	if !first && b.online == online {
		return
	}
	if first {
		// The capability did not exist a moment ago; the runtime has to re-read
		// what this Bridge offers or it stays nil for the life of the window.
		b.capabilitiesChanged()
	}
	b.online = online
	for _, fn := range b.netSubs {
		fn(online)
	}
	b.Invalidate()
}

// Battery makes the Bridge a shell.BatteryWindow.
func (b *Bridge) Battery() shell.Battery {
	if !b.batKnown {
		return nil // no host reporting; an app hides the affordance
	}
	return mobileBattery{b}
}

type mobileBattery struct{ b *Bridge }

func (m mobileBattery) Level() float32 { return m.b.batLevel }
func (m mobileBattery) Charging() bool { return m.b.batCharging }
func (m mobileBattery) OnChange(fn func()) {
	m.b.batSubs = append(m.b.batSubs, fn)
}

// SetBattery delivers the charge level (0..1) and whether the device is on
// power. Hosts call it at startup and on the platform's battery notifications
// (iOS UIDevice batteryLevel/batteryState, Android ACTION_BATTERY_CHANGED).
func (b *Bridge) SetBattery(level float32, charging bool) {
	switch {
	case level < 0:
		level = 0
	case level > 1:
		level = 1
	}
	first := !b.batKnown
	b.batKnown = true
	if !first && b.batLevel == level && b.batCharging == charging {
		return
	}
	if first {
		b.capabilitiesChanged()
	}
	b.batLevel, b.batCharging = level, charging
	for _, fn := range b.batSubs {
		fn()
	}
	b.Invalidate()
}

// Links makes the Bridge a shell.LinksWindow.
//
// This is the inbound direction — the URL that launched the app, and any that
// arrive while it runs. Outbound (OpenURL) already worked and is unrelated.
func (b *Bridge) Links() shell.Links { return mobileLinks{b} }

type mobileLinks struct{ b *Bridge }

// Initial returns the launch URL, or "" when the app was started normally.
func (l mobileLinks) Initial() string { return l.b.initialLink }

func (l mobileLinks) OnLink(fn func(url string)) {
	l.b.linkSubs = append(l.b.linkSubs, fn)
}

// DeliverLink hands the bridge a deep link.
//
// The host calls it before driving the first frame for a launch URL (iOS
// application(_:open:), Android the launch Intent's data), and on every later
// arrival. The first one delivered before any frame becomes Links.Initial, so
// an app that reads Initial in its first Build sees the URL it was opened with;
// later ones only reach OnLink subscribers, because by then the app is running
// and "initial" would be a lie.
func (b *Bridge) DeliverLink(url string) {
	if url == "" {
		return
	}
	if !b.linkStarted {
		b.initialLink = url
	}
	for _, fn := range b.linkSubs {
		fn(url)
	}
	b.Invalidate()
}
