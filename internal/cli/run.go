package cli

import (
	"flag"
	"fmt"
	"os"
)

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var o buildOpts
	var platName string
	var port int
	addBuildFlags(fs, &o, &platName)
	fs.IntVar(&port, "port", 8080, "web server port (web platform)")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if err := o.resolve(fs, platName); err != nil {
		return err
	}
	out, err := build(o)
	if err != nil {
		return err
	}
	return launch(o, out, port)
}

// launch runs the freshly built artifact: serve the web dir, exec a native
// binary, or print the next step for a mobile bundle.
func launch(o buildOpts, out string, port int) error {
	switch o.platform.name {
	case "web":
		return serve(out, port, nil)
	case "ios", "android":
		fmt.Fprintf(os.Stderr, "gossamer: built %s → %s\n"+
			"Open the platform project to run on a device/simulator "+
			"(gossamer create scaffolds it).\n", o.platform.name, out)
		return nil
	default: // desktop, terminal — hand the terminal to the app
		return run("", o.rendererEnv(), out)
	}
}
