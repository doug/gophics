package prefs

import (
	"os"
	"path/filepath"
	"strings"
)

// AppDirName sanitizes an app identifier into a single directory name — the
// per-app element under os.UserConfigDir. It lives here rather than in one
// shell because two shells need the same answer: desktop and terminal builds
// of the same app must read the same preferences file, and they would not if
// each sanitized the identifier its own way.
//
// It falls an app identifier into a single directory name, falls back to the executable's name so an app that sets no identifier still gets a
func AppDirName(id string) string {
	if id == "" {
		if exe, err := os.Executable(); err == nil {
			id = filepath.Base(exe)
		}
	}
	if id == "" {
		id = "gophics-app"
	}
	// Keep it a plain single path element.
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, id)
	if id == "" || id == "." || id == ".." {
		id = "gophics-app"
	}
	return id
}
