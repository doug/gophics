//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

// Linux/BSD implementation of the notification capability (shell/notify.go)
// over notify-send(1), libnotify's CLI to the session's notification daemon.
// Published only where the binary exists, nil otherwise — the file-chooser
// bargain again.
package desktop

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/doug/gophics/shell"
)

func (w *window) Notifier() shell.Notifier {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return nil
	}
	return &unixNotifier{ids: map[string]string{}}
}

type unixNotifier struct {
	mu  sync.Mutex
	ids map[string]string // Notification.Tag -> daemon id, for -r replacement
}

func (n *unixNotifier) Authorize(cb func(shell.Permission)) {
	if cb == nil {
		return
	}
	cb(shell.PermissionGranted) // the daemon has no grant model
}

// Notify posts the notification, honoring Tag via notify-send's -p/-r pair:
// -p prints the daemon's id for the new notification, -r replaces one by id.
// On a notify-send too old for those flags the tagged path fails, and the
// retry without them means the notification still appears — stacked, which is
// the same degradation the macOS backend documents.
func (n *unixNotifier) Notify(note shell.Notification) {
	go func() {
		args := []string{}
		if note.Tag != "" {
			n.mu.Lock()
			prev := n.ids[note.Tag]
			n.mu.Unlock()
			args = append(args, "-p")
			if prev != "" {
				args = append(args, "-r", prev)
			}
		}
		args = append(args, note.Title, note.Body)
		out, err := exec.Command("notify-send", args...).Output()
		if err != nil && note.Tag != "" {
			_ = exec.Command("notify-send", note.Title, note.Body).Run()
			return
		}
		if note.Tag != "" {
			if id := strings.TrimSpace(string(out)); id != "" {
				n.mu.Lock()
				n.ids[note.Tag] = id
				n.mu.Unlock()
			}
		}
	}()
}
