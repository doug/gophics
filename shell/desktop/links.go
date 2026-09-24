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
	"strings"

	"github.com/doug/gophics/shell"
)

// Links makes the desktop window a shell.LinksWindow.
func (w *window) Links() shell.Links { return desktopLinks{initial: initialURL(os.Args)} }

type desktopLinks struct{ initial string }

func (l desktopLinks) Initial() string { return l.initial }

// OnLink is a no-op subscription: see the package note on desktop re-launch
// semantics. f is never called (never a fake event).
func (desktopLinks) OnLink(func(string)) {}

// initialURL returns the first argument (after the program name) that names an
// existing path or looks like a URL, or "" if none. An existing file path is
// normalized to a file:// URL so callers see a single URL currency; a
// scheme'd URL is returned as given.
//
// The path check runs first, and a scheme has to be longer than one letter.
// Both are for Windows, where url.Parse reads "C:\Users\doug\notes.txt" as a
// URL with scheme "c" — so a launch argument that was a file was handed back
// raw, contradicting the doc above, whether or not the file existed.
func initialURL(args []string) string {
	for _, a := range args[min(1, len(args)):] {
		if a == "" {
			continue
		}
		if abs, err := filepath.Abs(a); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return fileURL(abs)
			}
		}
		if u, err := url.Parse(a); err == nil && len(u.Scheme) > 1 && u.Scheme != "file" {
			return a // e.g. myapp://…, https://…
		}
	}
	return ""
}

// fileURL renders an absolute path as file:///…, with the slashes a URL wants.
// A Windows path needs the leading slash added by hand: url.URL would
// otherwise print "C:/Users/…" as the host.
func fileURL(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
