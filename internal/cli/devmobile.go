package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// `gophics dev -p ios|android`: rebuild, reinstall, relaunch, rehydrate.
//
// This used to be a refusal ("iterate on web/desktop and use `build` for
// device tests"). It is not hot reload and cannot be: Go has no way to replace
// code in a running process, on any platform — the one mechanism that comes
// close needs cgo and does not exist for android/* or ios/* at any cgo
// setting. It is hot *restart* with the UI state carried across, which is the
// same thing the desktop loop does, and the honest claim is "hot restart
// everywhere" rather than "hot reload, except on the two platforms where it
// would matter most".
//
// Steady-state cost, measured on this machine: ~13 s on an Android emulator
// and ~21 s on a pre-booted iOS simulator, against 1.4 s for desktop. Nothing
// here is parity with desktop; `gomobile bind` is 60-70% of it and is not
// incremental in any useful sense.
//
// The order of operations is the whole design, and it comes out of what the
// platforms allow:
//
//  1. Background the app. This is what asks for the snapshot: both host
//     templates already call Bridge.SetAppState from onStop /
//     applicationDidEnterBackground, and KEYCODE_HOME (Android) and
//     foregrounding another app (simulator) both background without killing.
//     So no host template changes and no stranded hand-written hosts.
//  2. Wait for the file. The write happens on the host UI thread on the way
//     out and there is nothing to wait on, so the loop watches for it where it
//     can see it and waits a fixed moment where it cannot.
//  3. Rebuild, reinstall, relaunch — the ordinary `run` path, which kills the
//     old process on the way. The snapshot is already on disk, in the app's
//     private directory, which survives `adb install -r` and an iOS container
//     migration; nothing is pushed or pulled.
//  4. The relaunched app finds the file when the host hands it the directory
//     and lands on the same screen.
func devMobile(o buildOpts) error {
	sess := &devSession{}
	o.dev = sess
	defer sess.close()

	changes, stopWatch := watchSource(".", 250*time.Millisecond)
	defer stopWatch()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	fmt.Fprintf(os.Stderr,
		"gophics: %s dev — edit & save to rebuild + reinstall + relaunch, state preserved (Ctrl-C to stop)\n",
		o.platform.name)
	if o.device {
		fmt.Fprintln(os.Stderr,
			"gophics: on a device the loop rebuilds and relaunches but does not preserve state —\n"+
				"         a snapshot needs the app backgrounded first, and nothing reachable from\n"+
				"         here can background an app on a physical device")
	}

	done := make(chan error, 1)
	running := true
	go func() { done <- runMobile(o) }()

	for {
		select {
		case <-sig:
			return nil
		case err := <-done:
			running = false
			if err != nil {
				// Left running: a build error should not end the session, the
				// same way it does not on desktop or web. Fix and save again.
				fmt.Fprintf(os.Stderr, "gophics: %v\n", err)
			}
		case <-changes:
			t0 := time.Now()
			fmt.Fprintln(os.Stderr, "gophics: change detected — snapshot + rebuild + reinstall + relaunch")
			sess.snapshot()
			sess.stopApp()
			if running {
				// A build already in flight cannot be cancelled; wait rather
				// than run two xcodebuilds at once over one derived-data
				// directory.
				if err := <-done; err != nil {
					fmt.Fprintf(os.Stderr, "gophics: %v\n", err)
				}
			}
			drain(changes) // the burst that arrived while we were waiting
			done = make(chan error, 1)
			running = true
			go func() {
				err := runMobile(o)
				fmt.Fprintf(os.Stderr, "gophics: %.1fs\n", time.Since(t0).Seconds())
				done <- err
			}()
		}
	}
}

// devStateFile is the hand-off file's name inside the app's private
// directory. The app picks the directory, so this is the only part of the path
// both sides have to agree on; shell/mobile/devstate.go declares the same
// constant and TestDevStateFileNameAgrees reads both.
const devStateFile = "dev-state.json"

// drain empties a coalescing watch channel without blocking.
func drain(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// devApp is what the run path learned about the app it installed.
type devApp struct {
	adb    string   // android: the adb binary ("" on iOS)
	target []string // android: -s <serial>, when several devices are attached
	udid   string   // ios: the simulator
	appID  string   // android package name / iOS bundle id
}

// background backs the app out to the home screen without killing it, which is
// what asks it for a snapshot (shell/mobile/devstate.go). Reports whether
// there was anything to ask.
func (a devApp) background() bool {
	switch {
	case a.adb != "":
		// Measured: the launcher comes to the front and the app's process
		// stays alive, so onStop runs and the snapshot is written. `adb
		// install -r` kills with no warning at all, which is why this has to
		// come first rather than being left to the reinstall.
		return a.adbRun("shell", "input", "keyevent", "KEYCODE_HOME")
	case a.udid != "":
		// Foregrounding Settings backgrounds ours; measured to leave the
		// process alive (launchctl still lists it afterwards).
		return exec.Command("xcrun", "simctl", "launch", a.udid, "com.apple.Preferences").Run() == nil
	}
	return false // a physical iOS device: nothing here can background an app
}

// removeState deletes any previous hand-off file, so that watching for one to
// appear is unambiguous. Reports whether it could reach the file at all.
func (a devApp) removeState() bool {
	switch {
	case a.adb != "" && a.appID != "":
		// run-as, which works because a debuggable app's own private
		// directory is readable as itself. It is also the only way in: nothing
		// else on the device may touch it.
		return a.adbRun("shell", "run-as", a.appID, "rm", "-f", "files/"+devStateFile)
	case a.udid != "":
		if a.simContainer() == "" {
			return false // nothing to look at, so nothing to watch either
		}
		if p := a.simStateFile(); p != "" {
			return os.Remove(p) == nil
		}
		return true // reachable, and already absent
	}
	return false
}

// hasState reports whether the app has written a hand-off file.
func (a devApp) hasState() bool {
	switch {
	case a.adb != "" && a.appID != "":
		return a.adbRun("shell", "run-as", a.appID, "ls", "files/"+devStateFile)
	case a.udid != "":
		return a.simStateFile() != ""
	}
	return false
}

// simStateFile is the hand-off file inside a simulator's data container, or ""
// when there is not one.
//
// Searched rather than assumed. The host decides what directory it hands the
// app (the stock one passes NSDocumentDirectory), and an app with its own
// Swift host may reasonably pass something else — Library/Application Support
// is the conventional answer for data the user should not see.
func (a devApp) simStateFile() string {
	dir := a.simContainer()
	if dir == "" {
		return ""
	}
	for _, pat := range []string{
		filepath.Join(dir, devStateFile),
		filepath.Join(dir, "*", devStateFile),
		filepath.Join(dir, "*", "*", devStateFile),
	} {
		if found, _ := filepath.Glob(pat); len(found) > 0 {
			return found[0]
		}
	}
	return ""
}

// simContainer is the app's data container on a simulator, or "" when it has
// none (not installed, or no simulator).
func (a devApp) simContainer() string {
	if a.udid == "" || a.appID == "" {
		return ""
	}
	out, err := exec.Command("xcrun", "simctl", "get_app_container",
		a.udid, a.appID, "data").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (a devApp) adbRun(args ...string) bool {
	return exec.Command(a.adb, append(append([]string{}, a.target...), args...)...).Run() == nil
}

// devSession is the handle the dev loop keeps on the running app.
//
// Its fields are filled in by runMobile on its own goroutine and read by the
// loop on the main one, so everything goes through the mutex — the race
// detector finds this one immediately otherwise.
type devSession struct {
	mu        sync.Mutex
	app       devApp
	running   bool      // the app was launched and has not been stopped since
	hostNoted bool      // the "generated a stock host" notice has been printed
	console   *exec.Cmd // a launch streaming the app's output (iOS)
	logcat    *exec.Cmd // the Android log stream, one per session
}

// notedHost reports whether the stock-host notice has already been printed,
// marking it printed either way.
//
// The host is deliberately re-rendered on every build, so in a loop the notice
// is true every time and worth reading once — forty copies of a four-line
// paragraph is how the one line that matters (a build error) gets lost.
func (d *devSession) notedHost() bool {
	if d == nil {
		return false // `run` and `build` say it every time, as they always did
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	was := d.hostNoted
	d.hostNoted = true
	return was
}

// installed records what the run path found out about the app. Called once per
// iteration, after the install.
func (d *devSession) installed(a devApp) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.app = a
}

// launchedApp records that the app is now up, so the next iteration knows
// there is something to ask for a snapshot.
func (d *devSession) launchedApp() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = true
}

// following records a per-iteration console stream for stopApp to end.
func (d *devSession) following(cmd *exec.Cmd) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.console = cmd
}

// follow starts a session-long log stream, once. Takes a constructor rather
// than a command so a second iteration does not spawn a second adb logcat
// (and so nothing is started at all when the first iteration fails).
func (d *devSession) follow(open func() (*exec.Cmd, error)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	already := d.logcat != nil
	d.mu.Unlock()
	if already {
		return
	}
	cmd, err := open()
	if err != nil {
		return // no log is a degraded loop, not a broken one
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logcat = cmd
}

// snapshot asks the running app for a hot-restart hand-off and waits until it
// is on disk.
//
// Backgrounding is the request, and then the wait is a *look* rather than a
// guess. The write happens on the host UI thread on the way out and there is
// nothing to wait on; a flat 700 ms was enough on an Android emulator and not
// on the iOS simulator, and the way that failed is the lesson — the loop
// rebuilt, reinstalled, relaunched and came back on a fresh screen, with
// nothing in any log to say why. So the previous hand-off is deleted first and
// the loop watches for a new one to appear, which cannot be misread. Both
// targets allow that: a debuggable Android app's private directory through
// run-as, a simulator's data container through simctl.
//
// Nothing happens when the app is not up — after a failed build it has been
// terminated, there is no snapshot coming, and the file already there is the
// state the developer last saw. Deleting it then would turn one build error
// into a lost session.
func (d *devSession) snapshot() {
	d.mu.Lock()
	a, up := d.app, d.running
	d.mu.Unlock()
	if !up {
		return
	}
	watchable := a.removeState()
	if !a.background() {
		return
	}
	if !watchable {
		// A physical device, or a container we cannot read. Fall back to a
		// wait long enough for the simulator case that caught us out.
		time.Sleep(2 * time.Second)
		return
	}
	for i := 0; i < 40; i++ {
		time.Sleep(100 * time.Millisecond)
		if a.hasState() {
			return
		}
	}
	fmt.Fprintln(os.Stderr, "gophics: the app wrote no state snapshot; "+
		"this restart will come back on a fresh screen")
}

// stopApp ends this iteration's app and its console stream.
//
// The app first: a `simctl launch --console-pty` exits of its own accord once
// the app it is following is gone, and killing the relay before the app would
// leave the next iteration's output interleaved with a process nobody owns.
// Android needs none of this, because `adb install -r` replaces a running app.
func (d *devSession) stopApp() {
	d.mu.Lock()
	a, console := d.app, d.console
	d.console, d.running = nil, false
	d.mu.Unlock()

	if a.udid != "" && a.appID != "" {
		_ = exec.Command("xcrun", "simctl", "terminate", a.udid, a.appID).Run()
	}
	reap(console)
}

// close ends everything the session started, including the log stream that
// outlives an iteration.
func (d *devSession) close() {
	d.stopApp()
	d.mu.Lock()
	logcat := d.logcat
	d.logcat = nil
	d.mu.Unlock()
	reap(logcat)
}

// reap kills a child and waits for it, so the loop does not accumulate
// zombies over a session of a hundred iterations.
func reap(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}
