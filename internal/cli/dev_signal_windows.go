//go:build windows

package cli

import (
	"os"
	"os/exec"
	"syscall"
)

// Windows has no SIGTERM to send a process. os.Process.Signal supports only
// Kill there and returns EWINDOWS for anything else, so the Unix path's SIGTERM
// was an error the restart loop swallowed: every save waited the full grace
// period and then hard-killed the app, and the state-preserving restart the
// command promises never once happened on Windows.
//
// What Windows does have is console control events. A CTRL_BREAK_EVENT sent to
// the app's process group reaches Go's runtime as os.Interrupt, which the app's
// dev-state handler already listens for alongside SIGTERM. The app has to be
// the root of its own group for that: GenerateConsoleCtrlEvent addresses a
// group, and group 0 is the whole console, this CLI included. Go's own
// os/signal tests drive a child the same way.
//
// A process in its own group does not receive the console's Ctrl-C. That is
// fine here: the CLI catches Ctrl-C itself and stops the child on the way out.

var procGenerateConsoleCtrlEvent = syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")

func askToSnapshot(p *os.Process) error {
	r, _, err := procGenerateConsoleCtrlEvent.Call(syscall.CTRL_BREAK_EVENT, uintptr(p.Pid))
	if r == 0 {
		return err
	}
	return nil
}

func ownConsoleGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
