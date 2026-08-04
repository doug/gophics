//go:build android || ios

package app

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// Mobile hosts embed via NewHandler + shell/mobile.Bridge; there is no
// self-running shell on these platforms.
func desktopRun(shell.Handler, shell.Config) error {
	return errors.New("app: Run is not supported on mobile; use app.NewHandler with shell/mobile")
}
