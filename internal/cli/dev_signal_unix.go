//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// askToSnapshot tells the running app to write its dev-state snapshot and
// exit: SIGTERM, which app/devstate_desktop.go listens for.
func askToSnapshot(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

// ownConsoleGroup is a Windows concern; see dev_signal_windows.go.
func ownConsoleGroup(*exec.Cmd) {}
