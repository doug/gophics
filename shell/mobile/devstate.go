package mobile

import "path/filepath"

// State-preserving hot restart on a phone.
//
// There is no hot *reload* to implement here, and that is Go's doing rather
// than Apple's: Go cannot replace code in a running process on any platform,
// and the one mechanism that comes close (-buildmode=plugin) needs cgo — which
// this project does not have — and does not exist for android/* or ios/* at
// any cgo setting. Hot *restart* is a different thing and entirely possible:
// the app serialises its widget state, the CLI rebuilds and reinstalls, and the
// relaunched process picks the state back up and lands on the same screen with
// the same scroll offset, field contents and navigator stack.
//
// The machinery that does that is the desktop's, verbatim: widget.StateSnapshot
// carries no build tags and its tests pass on an Android device. Only the two
// ends needed wiring, and three properties of the platforms kept that small.
//
// The app's private directory survives `adb install -r` and an iOS container
// migration, so the snapshot never travels over adb or simctl — the app writes
// it where SetFilesDir already points and reads it back after relaunch. There
// is no file to push, no container path to discover, and no divergence between
// a simulator and a device.
//
// Both hosts already report backgrounding through SetAppState, and the CLI can
// background an app without killing it (`adb shell input keyevent
// KEYCODE_HOME`; launching another app on the simulator). So the request for a
// snapshot arrives over a path every existing host already implements: no host
// template changes, and an app with a hand-written MainActivity is not
// stranded.
//
// And SetAppState is called on the host UI thread, which is the thread that
// drives frames, so the snapshot is taken between frames. The desktop's route
// — a signal sets a flag and the next Frame snapshots and closes the window —
// would not work here anyway: once the OS has backgrounded the app there may
// be no next frame, and it is the CLI, not the app, that owns the kill.

// devRestart gates the whole of it, and is set at link time: `gophics dev`
// passes -ldflags "-X github.com/doug/gophics/shell/mobile.devRestart=1"
// through `gomobile bind`.
//
// A link-time flag rather than a build tag because it changes how no package
// compiles, so `dev` and `run` share one build cache. A tag would have made the
// first dev iteration after a `run` — and vice versa — a cold rebuild of the
// whole framework for a mobile GOARCH, which is the most expensive thing in
// this loop. `gophics build` and `gophics run` never set it, so a shipped app
// has no dev-state file and never looks for one.
var devRestart string

// devStateFile is the hand-off file's name inside the app's private directory.
const devStateFile = "dev-state.json"

// devStater is the app handler's half of the hand-off, reached by type
// assertion rather than by import so that shell/mobile does not have to know
// the snapshot format — and so a handler that is not app's (a test fake)
// simply opts out. app implements it in app/devstate_mobile.go.
type devStater interface {
	DevRestoreState(path string)
	DevSnapshotState(path string)
}

// devStatePath is the hand-off file, or "" when this is not a `gophics dev`
// build, the host has not said where the app may write, or the handler cannot
// snapshot.
func (b *Bridge) devStatePath() string {
	if devRestart == "" || b.filesDir == "" {
		return ""
	}
	if _, ok := b.handler.(devStater); !ok {
		return ""
	}
	return filepath.Join(b.filesDir, devStateFile)
}

// devRestoreState hands the previous process's snapshot to the fresh tree.
//
// Called from SetFilesDir, which is the earliest moment it can be: the path is
// what the hand-off needs, and both hosts pass it in onCreate /
// didFinishLaunching, before the first frame.
func (b *Bridge) devRestoreState() {
	if path := b.devStatePath(); path != "" {
		b.handler.(devStater).DevRestoreState(path)
	}
}

// devSnapshotState writes the current widget state out on the way to the
// background, for the process that replaces this one to read.
func (b *Bridge) devSnapshotState() {
	if path := b.devStatePath(); path != "" {
		b.handler.(devStater).DevSnapshotState(path)
	}
}
