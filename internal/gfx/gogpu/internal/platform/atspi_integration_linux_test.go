//go:build linux

package platform

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// walkScript asks AT-SPI, through pyatspi, to find our application on the
// accessibility bus and print the tree it sees. Every line it prints is
// something a screen reader would have read.
const walkScript = `
import sys, pyatspi
desktop = pyatspi.Registry.getDesktop(0)
for i in range(desktop.childCount):
    app = desktop.getChildAtIndex(i)
    if app is None or app.name != "gophics-atspi-test":
        continue
    def walk(o, depth=0):
        ext = o.queryComponent().getExtents(pyatspi.DESKTOP_COORDS) if "Component" in pyatspi.listInterfaces(o) else None
        acts = []
        if "Action" in pyatspi.listInterfaces(o):
            a = o.queryAction()
            acts = [a.getName(k) for k in range(a.nActions)]
        states = sorted(pyatspi.stateToString(st) for st in o.getState().getStates())
        print("NODE|%d|%s|%s|%s|%s|%s" % (
            depth, o.getRoleName(), o.name,
            "%d,%d,%d,%d" % (ext.x, ext.y, ext.width, ext.height) if ext else "-",
            ",".join(acts), ",".join(states)))
        if o.name == "Send" and acts:
            o.queryAction().doAction(0)
            print("DIDACTION")
        for j in range(o.childCount):
            c = o.getChildAtIndex(j)
            if c is not None:
                walk(c, depth + 1)
    walk(app)
    sys.exit(0)
print("NOTFOUND")
sys.exit(1)
`

// TestATSPIServesTreeToPyatspi is the end-to-end proof: a tree published from
// Go, read back through the real AT-SPI client stack.
//
// pyatspi is the right client for this rather than Orca. It reaches the same
// bus through the same libraries a screen reader uses, but it returns data
// instead of speech, so the tree's shape, roles, labels, bounds and actions
// become assertions rather than something to be read off a screen.
//
// Skips without an accessibility bus, which includes CI and any developer
// machine that is not Linux. Run it with:
//
//	podman run --rm -v "$PWD":/src:ro -w /src golang:1.26 bash -c '
//	  apt-get update -qq && apt-get install -y at-spi2-core python3-pyatspi
//	  export $(dbus-launch)
//	  /usr/libexec/at-spi-bus-launcher --launch-immediately &
//	  sleep 2 && go test ./internal/gfx/gogpu/internal/platform/ -run ATSPI -v'
func TestATSPIServesTreeToPyatspi(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	if addr, _ := a11yBusAddress(); addr == "" {
		t.Skip("no accessibility bus")
	}

	var (
		activatedMu sync.Mutex
		activated   []int
	)

	b := newATSPIBridge("gophics-atspi-test")
	if b == nil {
		t.Skip("could not reach the accessibility bus")
	}
	defer b.Close()

	// A small tree with the shapes that matter: a container, a labelled
	// button that can be activated, static text, and a checked checkbox.
	b.SetTree([]A11yNode{
		{ID: 1, ParentID: -1, Role: "group", Label: "Root", X: 0, Y: 0, W: 400, H: 300},
		{ID: 2, ParentID: 1, Role: "button", Label: "Send", X: 10, Y: 20, W: 80, H: 30, Tappable: true},
		{ID: 3, ParentID: 1, Role: "text", Label: "Hello", X: 10, Y: 60, W: 200, H: 20},
		{ID: 4, ParentID: 1, Role: "checkbox", Label: "Agree", X: 10, Y: 90, W: 120, H: 24,
			Checkable: true, Checked: true, Tappable: true},
	}, func(id int) {
		activatedMu.Lock()
		activated = append(activated, id)
		activatedMu.Unlock()
	})

	// Embed is asynchronous; give the registry a moment to answer before the
	// desktop is asked to list us.
	time.Sleep(1500 * time.Millisecond)

	out, err := exec.Command("python3", "-c", walkScript).CombinedOutput()
	text := string(out)
	if err != nil {
		t.Fatalf("pyatspi walk failed: %v\n%s", err, text)
	}
	t.Logf("tree as AT-SPI sees it:\n%s", text)

	// The names here are the client's, not ours. at-spi2 maps the numeric role
	// through its own table — role 43 prints as "button", though the spec
	// spells it "push button" and that is what our GetRoleName returns. The
	// number is the contract; almost nothing asks the app for the name.
	for _, want := range []string{
		"NODE|0|application|gophics-atspi-test",
		"NODE|1|frame|Root",
		"NODE|2|button|Send",
		"NODE|2|label|Hello",
		"NODE|2|check box|Agree",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// Bounds must survive the trip: a screen reader uses them to place its
	// highlight and to hit-test.
	if !strings.Contains(text, "|10,20,80,30|") {
		t.Errorf("button extents missing or wrong in:\n%s", text)
	}
	// Only the tappable nodes advertise an action.
	if !strings.Contains(text, "|10,20,80,30|click") {
		t.Errorf("button did not advertise its click action in:\n%s", text)
	}

	// States have to survive too: a checkbox that does not report CHECKED is
	// read out as unchecked, which is worse than silence.
	if !strings.Contains(text, "check box|Agree|10,90,120,24|click|checkable,checked,enabled") {
		t.Errorf("checkbox states missing or wrong in:\n%s", text)
	}
	// The static label must not claim to be focusable, or a screen reader
	// offers it as a stop on the tab ring.
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "|label|Hello|") && strings.Contains(line, "focusable") {
			t.Errorf("static text advertised focusable: %s", line)
		}
	}

	// Activation is the half that makes the tree usable rather than merely
	// readable: DoAction on the bus has to reach the Go callback.
	if !strings.Contains(text, "DIDACTION") {
		t.Fatalf("pyatspi never performed the action:\n%s", text)
	}
	activatedMu.Lock()
	defer activatedMu.Unlock()
	if len(activated) != 1 || activated[0] != 2 {
		t.Errorf("activate got %v, want [2] — the Send button's node ID", activated)
	}
}
