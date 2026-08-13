package bean

import (
	"os"
	"path/filepath"
)

// Load reads a beancount file, resolves its includes, and processes the result.
//
// Included paths resolve relative to the file that names them, which is what lets
// a ledger split into `accounts.beancount` and `2026.beancount` alongside each
// other. A cycle is broken rather than followed, and a file that cannot be read is
// reported without abandoning the rest.
func Load(path string) (*Ledger, error) {
	var (
		dirs    []Directive
		errs    ErrorList
		visited = map[string]bool{}
	)

	var read func(path string, from Position)
	read = func(path string, from Position) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if visited[abs] {
			return // already included; a cycle ends here
		}
		visited[abs] = true

		src, err := os.ReadFile(abs)
		if err != nil {
			errs = append(errs, &LoadError{Pos: from, Path: path, Err: err})
			return
		}
		f, perr := Parse(abs, string(src))
		if perr != nil {
			if list, ok := perr.(ErrorList); ok {
				errs = append(errs, list...)
			} else {
				errs = append(errs, perr)
			}
		}
		if f == nil {
			return
		}
		for _, d := range f.Directives {
			inc, ok := d.(*Include)
			if !ok {
				dirs = append(dirs, d)
				continue
			}
			target := inc.Path
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(abs), target)
			}
			read(target, inc.Where())
		}
	}

	read(path, Position{})

	l := Process(dirs)
	l.Problems = append(l.Problems, errs...)
	return l, errs.Err()
}

// LoadString processes source text directly, without include resolution — the
// path is used only for diagnostics.
func LoadString(path, src string) (*Ledger, error) {
	f, err := Parse(path, src)
	if f == nil {
		return nil, err
	}
	l := Process(f.Directives)
	if err != nil {
		if list, ok := err.(ErrorList); ok {
			l.Problems = append(l.Problems, list...)
		} else {
			l.Problems = append(l.Problems, err)
		}
	}
	return l, err
}

// LoadError reports an include that could not be read.
type LoadError struct {
	Pos  Position
	Path string
	Err  error
}

func (e *LoadError) Error() string {
	where := ""
	if e.Pos.Line > 0 {
		where = e.Pos.String() + ": "
	}
	return where + "cannot read " + strconvQuote(e.Path) + ": " + e.Err.Error()
}

func (e *LoadError) Unwrap() error { return e.Err }
