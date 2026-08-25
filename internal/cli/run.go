package cli

import (
	"flag"
)

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var o buildOpts
	var platName string
	var port int
	addBuildFlags(fs, &o, &platName)
	fs.IntVar(&port, "port", 8080, "web server port (web platform)")
	fs.StringVar(&o.host, "host", "", "mobile host project dir (default: sibling ios/ or android/ of the package)")
	fs.BoolVar(&o.device, "device", false, "run on a connected device rather than a simulator (ios)")
	fs.StringVar(&o.team, "team", "", "Apple Developer team ID for device signing (default: from the codesigning identity)")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if err := o.resolve(fs, platName); err != nil {
		return err
	}
	// Mobile binds straight into the host project and drives its build +
	// install + launch; it doesn't use the standalone build/<platform> artifact.
	if o.platform.name == "ios" || o.platform.name == "android" {
		return runMobile(o)
	}
	out, err := build(o)
	if err != nil {
		return err
	}
	return launch(o, out, port)
}

// launch runs the freshly built artifact: serve the web dir or exec a native
// binary. (Mobile is handled earlier by runMobile.)
func launch(o buildOpts, out string, port int) error {
	switch o.platform.name {
	case "web":
		return serve(out, port, nil)
	default: // desktop, terminal — hand the terminal to the app
		return run("", o.rendererEnv(), out)
	}
}
