package cli

import "fmt"

// devHot is the experimental plugin-based, state-preserving hot reload. It's
// wired in a follow-up; until then, point users at the working loops.
func devHot(_ buildOpts) error {
	return fmt.Errorf("--hot (state-preserving plugin reload) is not wired yet; " +
		"use `gossamer dev -p web` (live-reload) or `gossamer dev -p desktop` (hot-restart)")
}
