//go:build windows

// Windows folder chooser (shell/folder.go), driven through the same PowerShell
// host as the file chooser: the dialog is out-of-process either way, and this
// avoids hand-rolling COM vtable calls for one capability.
package desktop

import (
	"strings"

	"github.com/doug/gophics/shell"
)

// FolderPicker publishes the capability when a PowerShell host is available.
func (w *window) FolderPicker() shell.FolderPicker {
	if powershellPath() == "" {
		return nil
	}
	return winFolderPicker{}
}

type winFolderPicker struct{}

// Open presents FolderBrowserDialog.
//
// FolderBrowserDialog rather than the newer IFileDialog in FOS_PICKFOLDERS
// mode: the latter is the better dialog and reaching it from PowerShell means
// COM interop, which is what using PowerShell was meant to avoid. It runs on a
// goroutine because it blocks until the user answers.
func (winFolderPicker) Open(done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	go func() {
		const script = `
Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.FolderBrowserDialog
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.SelectedPath }
`
		out, err := runPowershell(script)
		if err != nil {
			done(nil, err)
			return
		}
		// Cancelling prints nothing, which is a cancel and not a failure.
		path := strings.TrimSpace(out)
		if path == "" {
			done(nil, nil)
			return
		}
		done(osFolder{path: path}, nil)
	}()
}
