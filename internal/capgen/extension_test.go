package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/capgen/testdata/extcap"
	"github.com/doug/gophics/internal/capgen/testdata/hollowcap"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// These tests live beside the generator because the generator writes the
// mechanism they exercise (widget/extension_gen.go), and because the fixtures
// they need are the worked out-of-tree example under testdata/ here. They
// import nothing internal except the fixtures themselves.

// bareWindow is a shell.Window and nothing more — the platform that cannot
// scan. Every capability must be nil for it.
type bareWindow struct{}

func (bareWindow) Invalidate()                      {}
func (bareWindow) SetTitle(string)                  {}
func (bareWindow) Close()                           {}
func (bareWindow) ClipboardRead() (string, error)   { return "", nil }
func (bareWindow) ClipboardWrite(text string) error { return nil }
func (bareWindow) OpenURL(url string) error         { return nil }
func (bareWindow) DarkMode() bool                   { return false }

// scannerWindow is a shell Window whose platform does have a scanner: it
// implements extcap.ScannerWindow, which is declared outside this module's
// framework packages, which is the whole point.
type scannerWindow struct {
	bareWindow
	scanner extcap.Scanner
}

func (w scannerWindow) Scanner() extcap.Scanner { return w.scanner }

// fakeScanner records what it was asked to do and lets the test fire a code
// from a non-UI goroutine's position in the call.
type fakeScanner struct {
	found   func(string)
	stopped bool
}

func (f *fakeScanner) Scan(found func(code string)) { f.found = found }
func (f *fakeScanner) Stop()                        { f.stopped = true }

// scanning is a Stateless widget that reads the out-of-tree capability during
// Build, exactly as an app would, and records what it saw. It is the proof the
// capability reaches the widget tree rather than merely the registry.
type scanning struct {
	seen  *extcap.Scanner
	built *int
}

func (s scanning) Build(ctx widget.Ctx) widget.Widget {
	*s.built++
	*s.seen = extcap.Cap.Of(ctx)
	return widget.Sized{W: 1, H: 1}
}

// A third-party capability must reach a widget's Build through Ctx, appear
// only once a window that provides it is wired, and carry its callbacks
// through Owner.Post like an in-tree one.
func TestOutOfTreeCapabilityReachesTheWidgetTree(t *testing.T) {
	var posted []func()
	o := &widget.Owner{Post: func(fn func()) { posted = append(posted, fn) }}

	var seen extcap.Scanner
	builds := 0
	o.SetRoot(scanning{seen: &seen, built: &builds})
	o.FlushBuilds()
	if builds == 0 {
		t.Fatal("the widget never built")
	}
	// Mounted but no window yet: nil, the same answer the in-tree capabilities
	// give on the first pass. A plugin that read the capability in Init would
	// miss it, which is documented on widget.State and true out of tree too.
	if seen != nil {
		t.Fatalf("capability was non-nil before any window was wired: %#v", seen)
	}

	// A window whose platform cannot scan leaves it nil rather than publishing
	// something that does nothing.
	o.WireCapabilities(bareWindow{})
	o.RebuildAll()
	o.FlushBuilds()
	if seen != nil {
		t.Fatalf("a window with no ScannerWindow published a capability: %#v", seen)
	}

	// A window that provides it publishes it.
	fake := &fakeScanner{}
	o.WireCapabilities(scannerWindow{scanner: fake})
	o.RebuildAll()
	o.FlushBuilds()
	if seen == nil {
		t.Fatal("a window implementing extcap.ScannerWindow did not reach the widget tree")
	}

	// And the callback is posted, not called inline: the contract the framework
	// enforces centrally for in-tree capabilities, which out of tree the
	// provider opts into by using the post it was handed.
	var codes []string
	seen.Scan(func(code string) { codes = append(codes, code) })
	if fake.found == nil {
		t.Fatal("Scan did not reach the platform implementation")
	}
	fake.found("4006381333931")
	if len(codes) != 0 {
		t.Fatalf("callback fired inline instead of being posted: %v", codes)
	}
	if len(posted) != 1 {
		t.Fatalf("want 1 posted closure, got %d", len(posted))
	}
	posted[0]()
	if len(codes) != 1 || codes[0] != "4006381333931" {
		t.Fatalf("posted callback delivered %v", codes)
	}

	// A capability can also go away — a shell sends CapabilitiesChanged when
	// hardware is removed — and re-wiring must take it back rather than leave
	// the previous value published.
	o.WireCapabilities(bareWindow{})
	o.RebuildAll()
	o.FlushBuilds()
	if seen != nil {
		t.Fatalf("capability survived a re-wire to a window without it: %#v", seen)
	}
}

// Registration refuses a provider that answers without consulting the window.
// This is the invariant most likely to rot, so it fails loudly: a panic during
// package initialization, before the app can start.
func TestHollowCapabilityIsRejectedAtRegistration(t *testing.T) {
	err := recovered(func() {
		widget.NewCapability[hollowcap.Scanner]("example.com/hollowcap.IgnoresWindow",
			func(w shell.Window, post func(func())) hollowcap.Scanner {
				return hollowcap.IgnoresWindow(w, post)
			})
	})
	if err == "" {
		t.Fatal("a provider that ignores the Window was accepted; every platform would report the capability as present")
	}
	for _, want := range []string{"hollow", "example.com/hollowcap.IgnoresWindow"} {
		if !strings.Contains(err, want) {
			t.Errorf("panic message %q does not mention %q", err, want)
		}
	}
}

// The other hollow shape: a provider that consults the window correctly but
// wraps without a nil check, so a window that implements the opt-in and
// returns nil yields a live-looking wrapper around nothing.
//
// The framework cannot catch this at registration — reaching the wrap needs a
// window implementing the declaring package's own <X>Window, which the
// framework cannot name — so the mechanism hands the declaring package the
// check instead. This test is both halves: CheckAbsent rejects the hollow
// wrapper and accepts the correct one, so it is detecting the defect rather
// than objecting to any wrapper at all.
func TestHollowWrapperIsCaughtByCheckAbsent(t *testing.T) {
	// It registers, and must: this shape answers the registration probe
	// correctly, because the probe window implements no ScannerWindow.
	k := widget.NewCapability[hollowcap.Scanner]("example.com/hollowcap.UnsafeWrap",
		func(w shell.Window, post func(func())) hollowcap.Scanner {
			return hollowcap.UnsafeWrap(w, post)
		})

	// The window implements the opt-in and has no device: nil in, and the
	// wrapper turns that into a live-looking wrapper around nothing.
	err := k.CheckAbsent(bareWindow{}, hollowScannerWindow{})
	if err == nil {
		t.Fatalf("CheckAbsent passed a hollow wrapper: %#v", k.ProvidedBy(hollowScannerWindow{}))
	}
	for _, want := range []string{"hollow", "example.com/hollowcap.UnsafeWrap"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// The honest provider passes the identical check, including the window
	// shape that caught the hollow one.
	if err := extcap.Cap.CheckAbsent(bareWindow{}, scannerWindow{scanner: nil}); err != nil {
		t.Errorf("extcap.Cap failed the hollow check: %v", err)
	}
	// And still reports a scanner that is really there — a check that only ever
	// says nil would pass CheckAbsent trivially.
	if got := extcap.Cap.ProvidedBy(scannerWindow{scanner: &fakeScanner{}}); got == nil {
		t.Error("extcap.Cap reported a present scanner as absent")
	}
}

// hollowScannerWindow implements hollowcap.ScannerWindow and has no scanner —
// the "hardware disabled" window the framework's own probe cannot build.
type hollowScannerWindow struct {
	bareWindow
}

func (hollowScannerWindow) Scanner() hollowcap.Scanner { return nil }

// nil must stay able to mean "this platform cannot". A struct or pointer
// capability has no value that means absent once it is boxed, so those are
// refused at registration rather than reported as present everywhere.
func TestCapabilityMustBeAnInterface(t *testing.T) {
	type notAnInterface struct{ N int }

	if err := recovered(func() {
		widget.NewCapability[notAnInterface]("example.com/x.Struct",
			func(w shell.Window, post func(func())) notAnInterface { return notAnInterface{} })
	}); err == "" {
		t.Error("a struct capability was accepted")
	}
	if err := recovered(func() {
		widget.NewCapability[*notAnInterface]("example.com/x.Pointer",
			func(w shell.Window, post func(func())) *notAnInterface { return nil })
	}); err == "" {
		t.Error("a pointer capability was accepted")
	}
}

// Two plugins must not be able to claim the same name: the name is what a
// diagnostic reports and what tells two registrations apart.
func TestDuplicateRegistrationIsRejected(t *testing.T) {
	name := "example.com/dup.Thing"
	provide := func(w shell.Window, post func(func())) hollowcap.Scanner { return nil }
	widget.NewCapability[hollowcap.Scanner](name, provide)
	if err := recovered(func() { widget.NewCapability[hollowcap.Scanner](name, provide) }); err == "" {
		t.Error("the same capability name was registered twice")
	}
}

// Registration is visible, because the usual way to get a plugin wrong is to
// forget to import it and then read nil forever.
func TestRegisteredCapabilitiesNamesTheFixture(t *testing.T) {
	names := widget.RegisteredCapabilities()
	found := false
	for _, n := range names {
		if n == extcap.Cap.Name() {
			found = true
		}
	}
	if !found {
		t.Errorf("%q is registered but not listed: %v", extcap.Cap.Name(), names)
	}
}

// The fixture is a stand-in for a package in another module, and that claim is
// only true if it needs nothing internal. Assert it rather than trust it: an
// internal import would compile here and make the whole mechanism a lie
// everywhere else.
func TestFixtureUsesOnlyPublicAPI(t *testing.T) {
	for _, dir := range []string{"testdata/extcap", "testdata/hollowapp"} {
		pkgs, err := parser.ParseDir(token.NewFileSet(), dir, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		if len(pkgs) == 0 {
			t.Fatalf("%s: no package found", dir)
		}
		for _, pkg := range pkgs {
			for path, f := range pkg.Files {
				for _, imp := range f.Imports {
					p := strings.Trim(imp.Path.Value, `"`)
					if strings.Contains(p, "/internal/") && !strings.Contains(p, "/capgen/testdata/") {
						t.Errorf("%s imports %s: an out-of-tree package could not", path, p)
					}
				}
			}
		}
	}
}

// A hollow plugin must stop the program, not degrade it. The in-process test
// above shows NewCapability panics; this shows what that means for an app,
// which is the claim that actually matters: registration is a package-level
// var, so the panic happens during initialization and main never runs.
func TestHollowPluginStopsTheProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles and runs a fixture binary; skipped in -short (a cost, not a capability gap)")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH; cannot run the fixture (a missing tool, not a defect)")
	}
	cmd := exec.Command("go", "run", "./testdata/hollowapp")
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("an app with a hollow capability started successfully:\n%s", out)
	}
	if !strings.Contains(string(out), "is hollow") {
		t.Errorf("the failure did not say why:\n%s", out)
	}
	if strings.Contains(string(out), "unreachable") {
		t.Errorf("initialization did not stop the program; main ran:\n%s", out)
	}
}

// The three checked-in generated files must be exactly what the generator
// produces for the current tree. gates.sh checks this by regenerating in place
// and diffing against git, which mutates the working tree and cannot run
// without one; this checks the same fact by generating into a temporary
// directory, so a change to the generator that happens to alter an in-tree
// capability's wiring fails here first, with a diff that names the file.
func TestInTreeGenerationIsUnchanged(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "widget"), 0o755); err != nil {
		t.Fatal(err)
	}
	quiet = true
	defer func() { quiet = false }()

	g := &gen{fset: token.NewFileSet(), ifaces: map[string]*ifaceInfo{}, structs: map[string]*structInfo{}}
	g.parseShell(filepath.Join(root, "shell"))
	g.collectShellDecls(filepath.Join(root, "shell"))
	g.computeWrappers()
	g.writeWidget(tmp)
	g.writeApp(tmp)
	g.writePosted(tmp)
	g.writeExtension(tmp)

	for _, name := range []string{"capabilities_gen.go", "wire_gen.go", "posted_gen.go", "extension_gen.go"} {
		want, err := os.ReadFile(filepath.Join(root, "widget", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(tmp, "widget", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("widget/%s is stale: regenerate with `go generate ./widget`", name)
		}
	}
}

// recovered runs fn and returns the panic message, or "" if it did not panic.
func recovered(fn func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			if s, ok := r.(string); ok {
				msg = s
				return
			}
			msg = "panic"
		}
	}()
	fn()
	return ""
}
