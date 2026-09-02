//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

// Linux/BSD implementation of the file-picking capability
// (shell/filepicker.go). There is no OS-level file dialog on these platforms —
// the dialog belongs to the desktop environment — so this drives whichever
// standard chooser the session provides, in the order a desktop is most likely
// to have one: zenity (GNOME and most others), kdialog (KDE), then the
// zenity-compatible forks.
//
// Shelling out is the pure-Go answer here. The alternative is binding GTK or Qt,
// which would mean CGo — the one thing the project's first principle rules out —
// and would drag a toolkit into a framework that draws its own pixels. The
// chooser is a separate process either way: this just skips the FFI.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/doug/gophics/shell"
)

// FilePicker publishes the capability, but only when the session actually has a
// chooser: reporting a capability that always errors is worse than reporting
// none, because callers use nil to decide whether to offer the button at all.
func (w *window) FilePicker() shell.FilePicker {
	if chooserPath() == "" {
		return nil
	}
	return unixPicker{}
}

type unixPicker struct{}

// choosers are tried in order. Each entry knows how to build its own argv
// because kdialog's interface is nothing like zenity's.
var choosers = []struct {
	bin  string
	open func(opts shell.OpenOptions) []string
	save func(opts shell.SaveOptions) []string
	dir  func() []string // directory chooser, for shell/folder.go
}{
	{
		bin:  "zenity",
		open: zenityOpenArgs,
		save: zenitySaveArgs,
		dir:  zenityDirArgs,
	},
	{
		bin: "kdialog",
		open: func(o shell.OpenOptions) []string {
			args := []string{"--getopenfilename", ".", kdialogFilter(o.Accept)}
			if o.Multiple {
				args = append(args, "--multiple", "--separate-output")
			}
			return args
		},
		save: func(o shell.SaveOptions) []string {
			return []string{"--getsavefilename", nonEmpty(o.Name, "."), kdialogFilter(o.Accept)}
		},
		dir: func() []string { return []string{"--getexistingdirectory", "."} },
	},
	// qarma and matedialog are drop-in zenity forks, so they take its argv.
	{bin: "qarma", open: zenityOpenArgs, save: zenitySaveArgs, dir: zenityDirArgs},
	{bin: "matedialog", open: zenityOpenArgs, save: zenitySaveArgs, dir: zenityDirArgs},
}

// chooserPath returns the absolute path of the first available chooser, or "".
func chooserPath() string {
	for _, c := range choosers {
		if p, err := exec.LookPath(c.bin); err == nil {
			return p
		}
	}
	return ""
}

// chooser returns the first available entry.
func chooser() (int, string) {
	for i, c := range choosers {
		if p, err := exec.LookPath(c.bin); err == nil {
			return i, p
		}
	}
	return -1, ""
}

// ErrNoFileChooser is reported when the session has no usable dialog program.
var ErrNoFileChooser = errors.New("gophics: no file chooser found (install zenity or kdialog)")

// Open runs the chooser and reads back the selected files.
//
// The dialog is a separate process, so it runs on its own goroutine and never
// blocks the frame loop; the generated PostedFilePicker wrapper marshals the
// callback back to the UI goroutine.
func (unixPicker) Open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	if done == nil {
		return
	}
	go func() {
		idx, path := chooser()
		if idx < 0 {
			done(nil, ErrNoFileChooser)
			return
		}
		out, err := runChooser(path, choosers[idx].open(opts))
		if err != nil || out == "" {
			// A cancelled dialog exits non-zero with no output; that is not an
			// error to the caller, it is an empty selection.
			done(nil, err)
			return
		}
		var files []shell.PickedFile
		for _, name := range splitSelection(out, opts.Multiple) {
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

// Save asks for a destination and writes data there.
func (unixPicker) Save(opts shell.SaveOptions, data []byte, done func(error)) {
	go func() {
		idx, path := chooser()
		if idx < 0 {
			report(done, ErrNoFileChooser)
			return
		}
		out, err := runChooser(path, choosers[idx].save(opts))
		if err != nil || out == "" {
			report(done, err) // cancelled
			return
		}
		name := strings.TrimSpace(out)
		// Writing through a temp file in the same directory keeps a failed or
		// truncated write from destroying whatever the user picked.
		report(done, writeAtomic(name, data))
	}()
}

func report(done func(error), err error) {
	if done != nil {
		done(err)
	}
}

// runChooser executes the dialog and returns its trimmed stdout. A non-zero
// exit with no output means the user cancelled, which is reported as ("", nil).
func runChooser(bin string, args []string) (string, error) {
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	text := strings.TrimRight(string(out), "\n")
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", nil // cancelled or dismissed
		}
		return "", err
	}
	return text, nil
}

// splitSelection parses the chooser's output. zenity separates multiple
// selections with "|"; kdialog is asked for --separate-output, so newlines.
func splitSelection(out string, multiple bool) []string {
	if !multiple {
		if s := strings.TrimSpace(out); s != "" {
			return []string{s}
		}
		return nil
	}
	fields := strings.FieldsFunc(out, func(r rune) bool { return r == '|' || r == '\n' })
	var names []string
	for _, f := range fields {
		if s := strings.TrimSpace(f); s != "" {
			names = append(names, s)
		}
	}
	return names
}

func zenityOpenArgs(o shell.OpenOptions) []string {
	args := []string{"--file-selection"}
	if o.Multiple {
		args = append(args, "--multiple", "--separator=|")
	}
	if f := zenityFilter(o.Accept); f != "" {
		args = append(args, "--file-filter="+f)
	}
	return args
}

func zenitySaveArgs(o shell.SaveOptions) []string {
	args := []string{"--file-selection", "--save", "--confirm-overwrite"}
	if o.Name != "" {
		args = append(args, "--filename="+o.Name)
	}
	if f := zenityFilter(o.Accept); f != "" {
		args = append(args, "--file-filter="+f)
	}
	return args
}

// zenityFilter turns the portable Accept list into zenity's glob syntax.
// MIME types are dropped rather than guessed at: a wrong filter hides the
// user's file, which is worse than no filter at all.
func zenityFilter(accept []string) string {
	var globs []string
	for _, a := range accept {
		if g := globFor(a); g != "" {
			globs = append(globs, g)
		}
	}
	if len(globs) == 0 {
		return ""
	}
	return strings.Join(globs, " ")
}

func kdialogFilter(accept []string) string {
	var globs []string
	for _, a := range accept {
		if g := globFor(a); g != "" {
			globs = append(globs, g)
		}
	}
	if len(globs) == 0 {
		return "*|All files"
	}
	return strings.Join(globs, " ")
}

// globFor maps one Accept entry to a shell glob, or "" if it cannot.
func globFor(a string) string {
	a = strings.TrimSpace(a)
	switch {
	case a == "":
		return ""
	case strings.HasPrefix(a, "."):
		return "*" + a
	case strings.HasSuffix(a, "/*"):
		// "image/*" and friends have no glob equivalent; a filter of "*" is
		// honest about accepting everything.
		return ""
	case strings.Contains(a, "/"):
		return "" // a concrete MIME type: no reliable extension mapping
	default:
		return "*." + a
	}
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// writeAtomic writes data to name via a sibling temp file and a rename.
func writeAtomic(name string, data []byte) error {
	dir := filepath.Dir(name)
	f, err := os.CreateTemp(dir, ".gophics-*")
	if err != nil {
		// A directory that won't take a temp file (a FUSE mount, say) is still
		// worth trying directly rather than failing the save outright.
		return os.WriteFile(name, data, 0o644)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
