//go:build darwin || linux

package terminal

import "golang.org/x/sys/unix"

// isatty reports whether fd is a terminal.
func isatty(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	return err == nil
}

// enterRaw puts the terminal in raw mode (no echo, no line buffering, no
// signal generation, 8-bit clean) and returns a function that restores the
// prior settings. Mirrors cfmakeraw; ISIG is cleared so Ctrl-C arrives as a
// byte we can route, rather than a signal.
func enterRaw(fd int) (restore func() error, err error) {
	old, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	prev := *old

	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1  // block until at least one byte
	raw.Cc[unix.VTIME] = 0 // no read timeout
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &raw); err != nil {
		return nil, err
	}
	return func() error {
		return unix.IoctlSetTermios(fd, ioctlWriteTermios, &prev)
	}, nil
}

// winsize returns the terminal's cell grid and pixel dimensions. Xpixel/Ypixel
// are 0 on terminals that don't report them.
func winsize(fd int) (cols, rows, xpix, ypix int, err error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return int(ws.Col), int(ws.Row), int(ws.Xpixel), int(ws.Ypixel), nil
}
