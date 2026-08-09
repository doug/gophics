//go:build !js

// Desktop implementation of the shell links capability (shell/links.go).
//
// Initial() reports the launch URL/path: the first os.Args argument that parses
// as a URL with a scheme (e.g. "myapp://open/42", "https://…") or names an
// existing filesystem path (which is returned as a file:// URL). This is the
// portable slice of desktop deep-linking that works without OS-specific plumbing
// — registering a URL scheme handler or receiving an AppleEvent/DDE re-open
// while already running is platform-specific and not yet wired.
//
// OnLink is therefore a no-op subscription: a running desktop app is typically
// re-launched (a second process) rather than handed a URL in-session, and those
// re-launch/scheme-handoff semantics vary per OS, so we never invent an event.
// When per-OS scheme handling is wired, Initial() keeps its meaning and OnLink
// begins delivering in-session URLs.

package desktop

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/doug/gophics/shell"
)

// Links makes the desktop window a shell.LinksWindow.
func (w *window) Links() shell.Links { return desktopLinks{initial: initialURL(os.Args)} }

type desktopLinks struct{ initial string }

func (l desktopLinks) Initial() string { return l.initial }

// OnLink is a no-op subscription: see the package note on desktop re-launch
// semantics. f is never called (never a fake event).
func (desktopLinks) OnLink(func(string)) {}

// initialURL returns the first argument (after the program name) that looks like
// a URL or an existing path, or "" if none. A bare scheme'd URL is returned as
// given; an existing file path is normalized to a file:// URL so callers see a
// single URL currency.
func initialURL(args []string) string {
	for _, a := range args[min(1, len(args)):] {
		if a == "" {
			continue
		}
		if u, err := url.Parse(a); err == nil && u.Scheme != "" && u.Scheme != "file" {
			return a // e.g. myapp://…, https://…
		}
		if abs, err := filepath.Abs(a); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return (&url.URL{Scheme: "file", Path: abs}).String()
			}
		}
	}
	return ""
}
