//go:build darwin

package terminal

import "golang.org/x/sys/unix"

// termios get/set ioctl requests on Darwin/BSD.
const (
	ioctlReadTermios  = unix.TIOCGETA
	ioctlWriteTermios = unix.TIOCSETA
)
