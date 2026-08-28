package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A bind package may not build its own app.Config.
//
// This is the drift that started all of it. The scaffold left Config() in
// package main, where gomobile cannot see it, so every app that wanted a mobile
// build wrote a second one by hand — and four of the six examples had already
// drifted: hn lost its title and re-declared its fonts, tally its AppID, and
// nothing compared the two. An app whose phone build uses a different font from
// its desktop build is not obviously broken; it just looks slightly wrong to
// whoever notices, months later, on one platform.
//
// The fix is that both sides call ui.Config(). This test is what keeps the
// second copy from growing back.
var inlineConfig = regexp.MustCompile(`app\.Config\{`)

func TestBindPackagesDoNotRebuildConfig(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		mobileDir := filepath.Join(root, d.Name(), "mobile")
		files, err := filepath.Glob(filepath.Join(mobileDir, "*.go"))
		if err != nil || len(files) == 0 {
			continue // no hand-written bind package; the CLI generates one
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			checked++
			if inlineConfig.Match(src) {
				t.Errorf("%s builds its own app.Config.\n"+
					"Call ui.Config() instead, so the mobile build and the desktop "+
					"entry point cannot disagree about fonts, background or app id. "+
					"If this app genuinely needs different settings on a phone, put "+
					"that decision in the ui package where both sides can see it.", f)
			}
		}
	}
	if checked == 0 {
		t.Skip("no hand-written bind packages left to check")
	}
}

// The generated bind package must stay free of app-specific content.
//
// It is generated precisely because nothing in it is ever app-specific; the
// moment it grows a knob, every app has to care what the generator decided for
// it. A capability an app needs belongs on the Bridge or in its ui package, not
// here.
func TestGeneratedBindPackageStaysGeneric(t *testing.T) {
	var buf strings.Builder
	err := bindTmpl.Execute(&buf, bindData{
		Pkg: "demomobile", App: "demo", UI: "example.com/demo/ui",
	})
	if err != nil {
		t.Fatal(err)
	}
	src := buf.String()

	// Exactly one exported symbol: Start.
	exported := regexp.MustCompile(`(?m)^func ([A-Z]\w*)`).FindAllStringSubmatch(src, -1)
	if len(exported) != 1 || exported[0][1] != "Start" {
		var names []string
		for _, m := range exported {
			names = append(names, m[1])
		}
		t.Errorf("generated bind package exports %v, want only Start — anything "+
			"else is app-specific content in a file the app does not own", names)
	}

	// It must read the tree and config from the app rather than inventing them,
	// which is the whole reason it can be generated.
	for _, want := range []string{"ui.Root()", "ui.Config()"} {
		if !strings.Contains(src, want) {
			t.Errorf("generated bind package does not call %s", want)
		}
	}
	if inlineConfig.MatchString(src) {
		t.Error("generated bind package builds its own app.Config")
	}
	// The forty passthrough wrappers this replaced are the thing not to grow
	// back: the CLI binds shell/mobile alongside, so the host calls the Bridge
	// directly.
	if n := strings.Count(src, "bridge."); n > 0 {
		t.Errorf("generated bind package forwards to the bridge %d times; the "+
			"host calls Bridge methods directly and needs no wrappers here", n)
	}
}
