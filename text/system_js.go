//go:build js

package text

import "errors"

// UseSystemFonts cannot work in a browser, and says so.
//
// It used to return nil here. That made it a silent no-op: an app calling it
// got "yes, system fonts are on" and then rendered tofu for every rune outside
// the faces it had registered, with nothing anywhere to suggest a reason. That
// is the failure mode gophics's capability rule exists to prevent — an
// implementation that always answers "nothing" leaves the caller unable to
// distinguish "this machine has no CJK font" from "this build cannot ask", and
// it will go on believing the first one. An error is the honest answer.
//
// The reason is not an unfinished port. A wasm page has no access to the bytes
// of an installed font: the CSS Font Loading API exposes no source for a
// FontFace, document.fonts contains only the faces the page itself declared or
// added, and FontFaceSet.check() returns true for a family that does not exist,
// so it cannot even be used to ask whether one is there. Canvas measurement can
// tell you a system face covers a rune but not what its outlines are, and
// gophics rasterises outlines in Go. The only API that hands over bytes is the
// Local Font Access API, which is desktop-Chromium-only and permission-gated;
// LocalFontSource wraps it for the apps that can use it.
//
// What works in every browser is to fetch the face. See UseWebFonts.
func (s *Shaper) UseSystemFonts(string) error {
	return errors.New("text: a browser cannot hand a wasm page the bytes of an " +
		"installed font, so there are no system fonts to scan here; register the " +
		"faces you need (SetFonts), or fetch them on demand with UseWebFonts")
}
