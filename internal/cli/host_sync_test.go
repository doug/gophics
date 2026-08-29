package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scaffolded hosts are copies of the reference hosts, and nothing said so.
//
// shell/mobile/native/GophicsPlatform.{kt,swift} are what the docs point at;
// internal/cli/templates/... are what `gophics create` actually writes. They
// are the same file bar one line — the package declaration on Android, the
// framework import on iOS — but they are two files on disk with no link
// between them, so an edit to either silently drifts from the other. That is
// the same shape as the bug scripts/gates.sh was written for, where `gophics
// build` and package/android.sh held separate copies of a linker flag and only
// a device could tell you they disagreed.
//
// It is not hypothetical here either: wiring setLocale into the reference hosts
// left the templates behind, so every app `gophics create` produced would have
// kept formatting as en-US on a German device while the reference host was
// fixed. This test is what caught it.
//
// Cheap and always on, unlike the swiftc check above — a string compare is not
// a reason to need an opt-in.
func TestScaffoldedHostsMatchTheReferenceHosts(t *testing.T) {
	for _, c := range []struct {
		native, tmpl string
		from, to     string
	}{{
		native: "shell/mobile/native/GophicsPlatform.kt",
		tmpl:   "internal/cli/templates/mobile/android/app/src/main/kotlin/GophicsPlatform.kt.tmpl",
		from:   "package dev.gophics.host",
		to:     "package {{.AndroidPkg}}",
	}, {
		native: "shell/mobile/native/GophicsPlatform.swift",
		tmpl:   "internal/cli/templates/mobile/ios/App/GophicsPlatform.swift.tmpl",
		from:   "import Mobile",
		to:     "import {{.Framework}}",
	}} {
		t.Run(filepath.Base(c.native), func(t *testing.T) {
			native, err := os.ReadFile(filepath.Join("..", "..", c.native))
			if err != nil {
				t.Fatal(err)
			}
			tmpl, err := os.ReadFile(filepath.Join("..", "..", c.tmpl))
			if err != nil {
				t.Fatal(err)
			}
			// Exactly one substitution, so a native file that stops containing
			// the line fails here rather than producing a template that is
			// quietly missing its package declaration.
			if n := strings.Count(string(native), c.from+"\n"); n != 1 {
				t.Fatalf("%s contains %q %d times, want exactly 1", c.native, c.from, n)
			}
			want := strings.Replace(string(native), c.from+"\n", c.to+"\n", 1)
			if string(tmpl) != want {
				t.Errorf("%s has drifted from %s.\n"+
					"They must be identical except for %q -> %q.\n"+
					"Edit the native file, then regenerate:\n"+
					"    sed 's|^%s$|%s|' %s > %s",
					c.tmpl, c.native, c.from, c.to, c.from, c.to, c.native, c.tmpl)
			}
		})
	}
}
