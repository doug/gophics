//go:build !js && !android && !ios

package app

import "github.com/doug/gophics/paint"

// loadSystemFonts backs Config.SystemFonts on desktop, where the OS keeps a
// font directory the shaper can scan and the machine's own CJK and emoji
// faces are the natural answer for runes the app did not bundle.
func loadSystemFonts(p *paint.Painter) error { return p.LoadSystemFonts() }
