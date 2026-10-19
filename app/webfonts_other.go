//go:build !js

package app

import "github.com/doug/gophics/text"

// loadWebFonts is a no-op off the web, so Config.WebFonts is safe to set in a
// Config shared across targets — the same contract Config.SystemFonts has.
//
// There is nothing to do here rather than nothing to be done: every other
// target can scan the machine's own fonts (desktop) or expects the app to
// bundle the faces it needs (mobile), so fetching a face over the network
// would be a worse answer than the one those platforms already have.
func loadWebFonts(*core, map[text.Script]string) error { return nil }
