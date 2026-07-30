package cli

import (
	"flag"
	"strings"
)

// flagsFirst reorders args so every recognized flag (and its value) precedes
// the positional operands, letting users intermix them — `gossamer dev ./app
// -p web` works the same as `-p web ./app`. Go's flag package otherwise stops
// at the first operand. Call after the flags are registered on fs, before
// Parse.
func flagsFirst(fs *flag.FlagSet, args []string) []string {
	isBool := func(name string) bool {
		f := fs.Lookup(name)
		if f == nil {
			return false
		}
		bf, ok := f.Value.(interface{ IsBoolFlag() bool })
		return ok && bf.IsBoolFlag()
	}
	var flags, operands []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			operands = append(operands, args[i+1:]...)
			return append(flags, operands...)
		case len(a) > 1 && a[0] == '-':
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			// A separate value follows only for non-bool flags written as
			// "-flag value" (not "-flag=value").
			if !strings.ContainsRune(name, '=') && !isBool(name) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			operands = append(operands, a)
		}
	}
	return append(flags, operands...)
}
