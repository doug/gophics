//go:build linux

package terminal

import "golang.org/x/sys/unix"

// termios get/set ioctl requests on Linux.
const (
	ioctlReadTermios  = unix.TCGETS
	ioctlWriteTermios = unix.TCSETS
)
