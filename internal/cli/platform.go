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
	case "terminal":
		return platform{name: "terminal", tags: []string{"gossamer_term"}}, nil
	case "ios":
		return platform{name: "ios"}, nil // via gomobile
	case "android":
		return platform{name: "android"}, nil // via gomobile
	default:
		return platform{}, fmt.Errorf("unknown platform %q (want desktop|web|terminal|ios|android)", name)
	}
}

// tagList joins the platform's implied tags with any extras. The GPU backend
// is no longer a build tag — it is the runtime default (see -renderer).
func tagList(p platform, extra string) string {
	tags := append([]string{}, p.tags...)
	if extra = strings.TrimSpace(extra); extra != "" {
		tags = append(tags, strings.Split(extra, ",")...)
	}
	return strings.Join(tags, ",")
}

// buildOpts is the resolved configuration shared by build/run/dev.
type buildOpts struct {
	platform platform
	renderer string // "", "auto", "gpu", or "cpu"
	tags     string
	out      string
	pkg      string
	host     string // mobile host project dir (run only; "" = sibling convention)
}

// addBuildFlags registers the flags common to build/run/dev on fs. platName is
// resolved to a platform after parsing.
func addBuildFlags(fs *flag.FlagSet, o *buildOpts, platName *string) {
	fs.StringVar(platName, "platform", "desktop", "target: desktop|web|terminal|ios|android")
	fs.StringVar(platName, "p", "desktop", "shorthand for -platform")
	fs.StringVar(&o.renderer, "renderer", "", "renderer: auto|gpu|cpu (default auto = GPU with CPU fallback; native run/dev)")
	fs.StringVar(&o.tags, "tags", "", "extra comma-separated build tags")
	fs.StringVar(&o.out, "o", "", "output path (default build/<platform>)")
}

// rendererEnv returns the GOSSAMER_RENDERER assignment for launching a native
// app, or nil when unset. Applies to desktop/terminal run/dev; web reads its
// renderer from Config (default Auto) since the browser has no process env.
func (o buildOpts) rendererEnv() []string {
	switch strings.ToLower(strings.TrimSpace(o.renderer)) {
	case "", "auto", "gpu", "cpu":
		if o.renderer == "" {
			return nil
		}
		return []string{"GOSSAMER_RENDERER=" + strings.ToLower(strings.TrimSpace(o.renderer))}
	default:
		return nil
	}
}

// resolve fills o.platform and o.pkg after fs has been parsed.
func (o *buildOpts) resolve(fs *flag.FlagSet, platName string) error {
	p, err := platformByName(platName)
	if err != nil {
		return err
	}
	o.platform = p
	switch strings.ToLower(strings.TrimSpace(o.renderer)) {
	case "", "auto", "gpu", "cpu":
	default:
		return fmt.Errorf("unknown renderer %q (want auto|gpu|cpu)", o.renderer)
	}
	o.pkg = "."
	if fs.NArg() > 0 {
		o.pkg = fs.Arg(0)
	}
	return nil
}
