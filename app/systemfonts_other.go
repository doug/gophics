//go:build js || android || ios

package app

import "github.com/doug/gophics/paint"

// loadSystemFonts is a no-op here, so Config.SystemFonts is safe to set in a
// Config shared across targets. On the web there is no font directory to
// scan and text.UseSystemFonts is itself a stub. On mobile the scanner would
// link, but it needs a cache directory the host app must hand it and reads
// system paths a sandboxed process cannot rely on, and no mobile shell has
// been through that yet — an app that needs CJK or emoji there bundles the
// face in Config.Fallbacks, which works on every target.
func loadSystemFonts(*paint.Painter) error { return nil }
