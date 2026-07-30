package cli

import (
	"flag"
	"fmt"
	"strings"
)

// platform captures how to compile for one target.
type platform struct {
	name   string
	goos   string   // "" → host OS
	goarch string   // "" → host arch
	tags   []string // build tags implied by the platform
	wasm   bool
}

func platformByName(name string) (platform, error) {
	switch name {
	case "", "desktop":
		return platform{name: "desktop"}, nil
	case "web":
		return platform{name: "web", goos: "js", goarch: "wasm", wasm: true}, nil
	case "terminal", "term":
		return platform{name: "terminal", tags: []string{"gossamer_term"}}, nil
	case "ios":
		return platform{name: "ios"}, nil // via gomobile
	case "android":
		return platform{name: "android"}, nil // via gomobile
	default:
		return platform{}, fmt.Errorf("unknown platform %q (want desktop|web|terminal|ios|android)", name)
	}
}

// tagList joins the platform's implied tags with the GPU tag and any extras.
func tagList(p platform, gpu bool, extra string) string {
	tags := append([]string{}, p.tags...)
	if gpu {
		tags = append(tags, "gossamer_gpu")
	}
	if extra = strings.TrimSpace(extra); extra != "" {
		tags = append(tags, strings.Split(extra, ",")...)
	}
	return strings.Join(tags, ",")
}

// buildOpts is the resolved configuration shared by build/run/dev.
type buildOpts struct {
	platform platform
	gpu      bool
	tags     string
	out      string
	pkg      string
}

// addBuildFlags registers the flags common to build/run/dev on fs. platName is
// resolved to a platform after parsing.
func addBuildFlags(fs *flag.FlagSet, o *buildOpts, platName *string) {
	fs.StringVar(platName, "platform", "desktop", "target: desktop|web|terminal|ios|android")
	fs.StringVar(platName, "p", "desktop", "shorthand for -platform")
	fs.BoolVar(&o.gpu, "gpu", false, "enable the GPU backend (build tag gossamer_gpu)")
	fs.StringVar(&o.tags, "tags", "", "extra comma-separated build tags")
	fs.StringVar(&o.out, "o", "", "output path (default build/<platform>)")
}

// resolve fills o.platform and o.pkg after fs has been parsed.
func (o *buildOpts) resolve(fs *flag.FlagSet, platName string) error {
	p, err := platformByName(platName)
	if err != nil {
		return err
	}
	o.platform = p
	o.pkg = "."
	if fs.NArg() > 0 {
		o.pkg = fs.Arg(0)
	}
	return nil
}
