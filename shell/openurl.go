package shell

import (
	"fmt"
	"net/url"
)

// openURLSchemes is what Window.OpenURL agrees to hand to the platform.
//
// A closed list, and a short one, because on desktop the string goes to
// `open`, `xdg-open` or rundll32, and each of those will happily run a
// file://, a javascript: or an app-registered scheme with whatever the page
// or the message put in the link. http and https are what "open in the
// browser" means; mailto and tel are the two other links every platform
// renders as a link and every user expects to work. A scheme another app
// registered is not here on purpose: it works on a phone, where the OS
// mediates it, and on desktop it is a way to launch programs from a string.
var openURLSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
	"tel":    true,
}

// CheckOpenURL reports whether Window.OpenURL will open u, and why not when it
// will not. Every shell asks it first, so the same URL is accepted or refused
// on every platform. The desktop shell used to allow only http and https while
// web and mobile accepted anything, so a mailto: link worked in the browser and
// on a phone and errored on the Mac.
func CheckOpenURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("shell: OpenURL %q: %w", u, err)
	}
	// url.Parse lower-cases the scheme, so HTTPS:// is https.
	if parsed.Scheme == "" {
		return fmt.Errorf("shell: OpenURL %q has no scheme; only http, https, mailto and tel open", u)
	}
	if !openURLSchemes[parsed.Scheme] {
		return fmt.Errorf("shell: refusing to open %q: only http, https, mailto and tel open", u)
	}
	return nil
}
