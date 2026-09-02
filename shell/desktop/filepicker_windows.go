//go:build windows

// Windows implementation of the file-picking capability (shell/filepicker.go).
//
// The common file dialogs live in comdlg32/shell32 and are COM/Win32 APIs. The
// project forbids CGo, and hand-rolling the COM vtable calls for
// IFileOpenDialog through syscall would be a large amount of unsafe pointer
// work for one capability. PowerShell is present on every supported Windows
// version and hosts the same dialogs through System.Windows.Forms, so this
// drives it as a subprocess — the dialog is out-of-process either way.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/doug/gophics/shell"
)

// FilePicker publishes the capability when a PowerShell host is available.
func (w *window) FilePicker() shell.FilePicker {
	if powershellPath() == "" {
		return nil
	}
	return winPicker{}
}

type winPicker struct{}

// ErrNoFileChooser is reported when no PowerShell host can be found.
var ErrNoFileChooser = errors.New("desktop: no file chooser found (PowerShell not available)")

func powershellPath() string {
	for _, bin := range []string{"powershell.exe", "pwsh.exe"} {
		if p, err := exec.LookPath(bin); err == nil {
			return p
		}
	}
	return ""
}

// Open presents OpenFileDialog and reads the chosen files.
//
// The dialog blocks its own process, so it runs on a goroutine; the generated
// PostedFilePicker wrapper marshals the callback back to the UI goroutine.
func (winPicker) Open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	if done == nil {
		return
	}
	go func() {
		script := `
Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Filter = '` + psEscape(winFilter(opts.Accept)) + `'
$d.Multiselect = $` + boolLit(opts.Multiple) + `
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileNames -join "` + "`n" + `" }
`
		out, err := runPowershell(script)
		if err != nil || strings.TrimSpace(out) == "" {
			done(nil, err) // cancelled reports no files and no error
			return
		}
		var files []shell.PickedFile
		for _, name := range strings.Split(strings.TrimSpace(out), "\n") {
			name = strings.TrimSpace(strings.TrimSuffix(name, "\r"))
			if name == "" {
				continue
			}
			data, rerr := os.ReadFile(name)
			if rerr != nil {
				done(nil, rerr)
				return
			}
			files = append(files, shell.PickedFile{
				Name: filepath.Base(name), Data: data, Path: name,
			})
		}
		done(files, nil)
	}()
}

// Save presents SaveFileDialog and writes data to the chosen path.
func (winPicker) Save(opts shell.SaveOptions, data []byte, done func(error)) {
	go func() {
		script := `
Add-Type -AssemblyName System.Windows.Forms | Out-Null
$d = New-Object System.Windows.Forms.SaveFileDialog
$d.Filter = '` + psEscape(winFilter(opts.Accept)) + `'
$d.FileName = '` + psEscape(opts.Name) + `'
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $d.FileName }
`
		out, err := runPowershell(script)
		if err != nil {
			reportWin(done, err)
			return
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(out), "\r"))
		if name == "" {
			reportWin(done, nil) // cancelled
			return
		}
		reportWin(done, writeFileAtomic(name, data))
	}()
}

func reportWin(done func(error), err error) {
	if done != nil {
		done(err)
	}
}

func runPowershell(script string) (string, error) {
	bin := powershellPath()
	if bin == "" {
		return "", ErrNoFileChooser
	}
	cmd := exec.Command(bin, "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil // treat a failed dialog host as a cancel
		}
		return "", err
	}
	return string(out), nil
}

// winFilter builds the "Label|glob" pairs the Win32 dialogs expect. MIME types
// have no reliable extension mapping, so anything that isn't an extension
// widens the filter to everything rather than hiding the user's file.
func winFilter(accept []string) string {
	var globs []string
	for _, a := range accept {
		a = strings.TrimSpace(a)
		switch {
		case a == "":
		case strings.HasPrefix(a, "."):
			globs = append(globs, "*"+a)
		case strings.Contains(a, "/"):
			// MIME type: no mapping.
		default:
			globs = append(globs, "*."+a)
		}
	}
	if len(globs) == 0 {
		return "All files|*.*"
	}
	joined := strings.Join(globs, ";")
	return "Supported files|" + joined + "|All files|*.*"
}

// psEscape makes a value safe inside a PowerShell single-quoted string, where
// the only metacharacter is the quote itself (doubled to escape).
func psEscape(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	// A newline would end the statement and turn the rest into code.
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
