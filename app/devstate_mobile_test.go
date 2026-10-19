//go:build android || ios

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

// The mobile hand-off, on a mobile GOOS. Build-tagged, so `go test ./...` on a
// Mac does not run it; zero cgo means it cross-compiles and runs on a device:
//
//	CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test -c -o app.test.android ./app
//	adb push app.test.android /data/local/tmp/ && adb shell chmod +x …
//	adb shell "cd /data/local/tmp && ./app.test.android -test.run TestDevStateMobile -test.v"
//
// It is the same round trip the desktop test does, through the two methods the
// Bridge calls instead of a signal handler. The point of running it where it
// runs is that nothing in the snapshot machinery turns out to be desktop-only
// when it reaches an arm64 phone — which is the claim the whole mobile loop
// rests on.
func TestDevStateMobileRoundTrip(t *testing.T) {
	cfg := Config{Size: geom.Size{W: 100, H: 100}, Font: goregular.TTF}
	path := filepath.Join(t.TempDir(), "dev-state.json")

	var ref1 *devCounterState
	h1, err := NewHeadless(devCounter{Ref: &ref1}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ref1 == nil {
		t.Fatal("state not mounted")
	}
	ref1.SetState(func() { ref1.N = 42 })
	h1.Render()

	// What SetAppState(background) reaches, on the way out.
	(&shellHandler{core: h1.core}).DevSnapshotState(path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("no snapshot written: %v", err)
	}

	// What SetFilesDir reaches, in the process that replaced it.
	var ref2 *devCounterState
	h2, err := NewHeadless(devCounter{Ref: &ref2}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ref2.N != 0 {
		t.Fatalf("fresh app N = %d, want 0", ref2.N)
	}
	(&shellHandler{core: h2.core}).DevRestoreState(path)
	if ref2.N != 42 {
		t.Errorf("restored N = %d, want 42", ref2.N)
	}

	// Consumed: a later launch with no new snapshot must not resurrect this
	// one, or an app started from its icon would reopen a dev session's screen.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("hand-off file survived the restore (stat err %v)", err)
	}
}

// An empty snapshot leaves the previous one alone.
//
// A tree with nothing serializable in it would otherwise truncate good state
// the moment the app was backgrounded a second time — which the dev loop does
// on every iteration, so this would be hit immediately rather than being an
// edge case.
func TestDevStateMobileKeepsAGoodSnapshot(t *testing.T) {
	cfg := Config{Size: geom.Size{W: 100, H: 100}, Font: goregular.TTF}
	path := filepath.Join(t.TempDir(), "dev-state.json")
	if err := os.WriteFile(path, []byte(`{"keep":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// A bare Sized has no State anywhere in the tree, so SnapshotState is
	// empty (asserted, so this test fails loudly rather than passing for the
	// wrong reason if the framework ever snapshots something by default).
	h, err := NewHeadless(widget.Sized{W: 10, H: 10}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if n := len(h.core.Owner.SnapshotState()); n != 0 {
		t.Fatalf("stateless tree snapshotted %d entries; pick a root with none", n)
	}
	(&shellHandler{core: h.core}).DevSnapshotState(path)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"keep":true}` {
		t.Errorf("snapshot file = %s, want the previous contents untouched", got)
	}
}
