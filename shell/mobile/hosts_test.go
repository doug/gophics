package mobile

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// The two contracts every host-backed capability on the Bridge shares, pinned
// in one place each: a host setter announces itself, and a result-delivering
// method with no callback does nothing. Both had exceptions that nothing
// caught — three media setters that never announced, and a Listen that
// dereferenced its nil callback — because each file answered for itself.

// eventRecorder is a shell.Handler that only remembers what it was told.
type eventRecorder struct{ events []shell.Event }

func (r *eventRecorder) Frame(shell.Window, shell.Frame, float64) {}
func (r *eventRecorder) Event(_ shell.Window, e shell.Event)      { r.events = append(r.events, e) }

func (r *eventRecorder) capabilitiesChanged() int {
	n := 0
	for _, e := range r.events {
		if _, ok := e.(shell.CapabilitiesChanged); ok {
			n++
		}
	}
	return n
}

// The fakes below count every request the Bridge sends them. The nil-callback
// test asserts the count stays zero; nothing else looks inside.

type countingMedia struct{ calls int }

func (h *countingMedia) AuthorizeCamera(int)      { h.calls++ }
func (h *countingMedia) CapturePhoto(int, int)    { h.calls++ }
func (h *countingMedia) AuthorizeMic(int)         { h.calls++ }
func (h *countingMedia) StartRecording(int)       { h.calls++ }
func (h *countingMedia) StopRecording(int)        { h.calls++ }
func (h *countingMedia) PlayClip(int, []byte)     { h.calls++ }
func (h *countingMedia) SeekPlayback(int, int)    { h.calls++ }
func (h *countingMedia) StopPlayback(int)         { h.calls++ }
func (h *countingMedia) StartMonitoring(int)      { h.calls++ }
func (h *countingMedia) StopMonitoring(int)       { h.calls++ }
func (h *countingMedia) StartPreview(_, _, _ int) { h.calls++ }
func (h *countingMedia) StopPreview(int)          { h.calls++ }

type countingLocation struct{ calls int }

func (h *countingLocation) StartLocation(int, bool) { h.calls++ }
func (h *countingLocation) StopLocation(int)        { h.calls++ }

type countingFiles struct{ calls int }

func (h *countingFiles) PickFiles(int, string, bool)          { h.calls++ }
func (h *countingFiles) SaveFile(int, string, string, []byte) { h.calls++ }

type countingDevice struct{ calls int }

func (h *countingDevice) SetKeepAwake(bool)              { h.calls++ }
func (h *countingDevice) AuthorizePhotos(int)            { h.calls++ }
func (h *countingDevice) SavePhoto(int, []byte, string)  { h.calls++ }
func (h *countingDevice) BiometricKind() int             { return int(shell.BiometricFingerprint) }
func (h *countingDevice) Authenticate(int, string, bool) { h.calls++ }

type countingNotify struct{ calls int }

func (h *countingNotify) AuthorizeNotify(int)   { h.calls++ }
func (h *countingNotify) Notify(_, _, _ string) { h.calls++ }

type countingShare struct{ calls int }

func (h *countingShare) Share(int, string, string, string, string, []byte) { h.calls++ }

type countingSecure struct{}

func (countingSecure) SecureGet(string) string         { return "" }
func (countingSecure) SecureHas(string) bool           { return false }
func (countingSecure) SecureSet(string, string) string { return "" }
func (countingSecure) SecureDelete(string) string      { return "" }

// Capabilities are wired once, when the shell hands over its Window. A host
// registers its backends after that, so every setter that makes a capability
// appear has to say so — or the runtime reads nil on the first frame and
// keeps it for the life of the window. SetMediaHost, SetMonitorHost and
// SetPreviewHost did not, which the example hosts hid by registering before
// the first frame.
func TestHostSettersAnnounceCapabilitiesChanged(t *testing.T) {
	setters := []struct {
		name string
		set  func(*Bridge)
	}{
		{"SetMediaHost", func(b *Bridge) { b.SetMediaHost(&countingMedia{}) }},
		{"SetMonitorHost", func(b *Bridge) { b.SetMonitorHost(&countingMedia{}) }},
		{"SetPreviewHost", func(b *Bridge) { b.SetPreviewHost(&countingMedia{}) }},
		{"SetShareHost", func(b *Bridge) { b.SetShareHost(&countingShare{}) }},
		{"SetNotifyHost", func(b *Bridge) { b.SetNotifyHost(&countingNotify{}) }},
		{"SetSecureHost", func(b *Bridge) { b.SetSecureHost(countingSecure{}) }},
		{"SetFileHost", func(b *Bridge) { b.SetFileHost(&countingFiles{}) }},
		{"SetLocationHost", func(b *Bridge) { b.SetLocationHost(&countingLocation{}) }},
		{"SetDeviceHost", func(b *Bridge) { b.SetDeviceHost(&countingDevice{}) }},
		{"SetFilesDir", func(b *Bridge) { b.SetFilesDir(t.TempDir()) }},
		{"SetOnline (first report)", func(b *Bridge) { b.SetOnline(true) }},
		{"SetBattery (first report)", func(b *Bridge) { b.SetBattery(0.5, false) }},
		{"SetLocale (first report)", func(b *Bridge) { b.SetLocale("en-US") }},
	}
	for _, tc := range setters {
		t.Run(tc.name, func(t *testing.T) {
			rec := &eventRecorder{}
			b := NewBridge(rec)
			tc.set(b)
			if n := rec.capabilitiesChanged(); n != 1 {
				t.Errorf("%s sent %d CapabilitiesChanged events, want exactly 1 — "+
					"a host wiring this after the first frame would leave the "+
					"capability nil for the life of the window", tc.name, n)
			}
		})
	}
}

// The nil rule from the shell package doc: a method whose only purpose is to
// deliver a result is a no-op when its callback is nil. On the Bridge that
// means the host is never asked, because the host's answer would either
// dereference the nil (Listen with only a MediaHost registered panicked) or
// start hardware nothing can stop (Record left a live microphone, Current left
// the location service running).
func TestNilCallbacksAreNoOps(t *testing.T) {
	type fixture struct {
		b     *Bridge
		media *countingMedia
		loc   *countingLocation
		files *countingFiles
		dev   *countingDevice
		note  *countingNotify
		calls func() int
	}
	newFixture := func(mediaHost, monitorHost, previewHost bool) fixture {
		f := fixture{
			b:     NewBridge(&eventRecorder{}),
			media: &countingMedia{},
			loc:   &countingLocation{},
			files: &countingFiles{},
			dev:   &countingDevice{},
			note:  &countingNotify{},
		}
		if mediaHost {
			f.b.SetMediaHost(f.media)
		}
		if monitorHost {
			f.b.SetMonitorHost(f.media)
		}
		if previewHost {
			f.b.SetPreviewHost(f.media)
		}
		f.b.SetLocationHost(f.loc)
		f.b.SetFileHost(f.files)
		f.b.SetDeviceHost(f.dev)
		f.b.SetNotifyHost(f.note)
		f.calls = func() int {
			return f.media.calls + f.loc.calls + f.files.calls + f.dev.calls + f.note.calls
		}
		return f
	}

	cases := []struct {
		name                                string
		mediaHost, monitorHost, previewHost bool
		call                                func(*Bridge)
	}{
		{"Microphone.Listen with only a MediaHost", true, false, false,
			func(b *Bridge) { b.Microphone().Listen(nil) }},
		{"Microphone.Listen with a MonitorHost", false, true, false,
			func(b *Bridge) { b.Microphone().Listen(nil) }},
		{"Microphone.Record", true, false, false,
			func(b *Bridge) { b.Microphone().Record(shell.RecordOptions{}, nil) }},
		{"Microphone.Record with no MediaHost", false, true, false,
			func(b *Bridge) { b.Microphone().Record(shell.RecordOptions{}, nil) }},
		{"Microphone.Authorize", true, true, false,
			func(b *Bridge) { b.Microphone().Authorize(nil) }},
		{"Microphone.Authorize with no host at all (host cleared)", true, false, false,
			func(b *Bridge) { m := b.Microphone(); b.SetMediaHost(nil); m.Authorize(nil) }},
		{"Camera.Authorize", true, false, false,
			func(b *Bridge) { b.Camera().Authorize(nil) }},
		{"Camera.Capture", true, false, false,
			func(b *Bridge) { b.Camera().Capture(shell.CaptureOptions{}, nil) }},
		{"CameraPreview.Authorize", false, false, true,
			func(b *Bridge) { b.CameraPreview().Authorize(nil) }},
		{"CameraPreview.Authorize after the host is cleared", false, false, true,
			func(b *Bridge) { p := b.CameraPreview(); b.SetPreviewHost(nil); p.Authorize(nil) }},
		{"CameraPreview.Start", false, false, true,
			func(b *Bridge) { b.CameraPreview().Start(shell.PreviewOptions{}, nil) }},
		{"CameraPreview.Start after the host is cleared", false, false, true,
			func(b *Bridge) { p := b.CameraPreview(); b.SetPreviewHost(nil); p.Start(shell.PreviewOptions{}, nil) }},
		{"Speakers.Play", true, false, false,
			func(b *Bridge) { b.Speakers().Play(shell.Clip{Data: []byte{1}}, nil) }},
		{"Geolocation.Current", false, false, false,
			func(b *Bridge) { b.Geolocation().Current(nil) }},
		{"Geolocation.Watch", false, false, false,
			func(b *Bridge) { b.Geolocation().Watch(nil)() }},
		{"FilePicker.Open", false, false, false,
			func(b *Bridge) { b.FilePicker().Open(shell.OpenOptions{}, nil) }},
		{"Photos.Authorize", false, false, false,
			func(b *Bridge) { b.Photos().Authorize(nil) }},
		{"Biometric.Authenticate", false, false, false,
			func(b *Bridge) { b.Biometric().Authenticate("why", false, nil) }},
		{"Notifier.Authorize", false, false, false,
			func(b *Bridge) { b.Notifier().Authorize(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(tc.mediaHost, tc.monitorHost, tc.previewHost)
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked on a nil callback: %v", r)
				}
			}()
			tc.call(f.b)
			if n := f.calls(); n != 0 {
				t.Errorf("the host received %d request(s) for a nil callback; "+
					"whatever it started, nothing can stop", n)
			}
		})
	}
}

// Fakes for the GPU handles, recording the order they were given back in.
type releaseLog struct{ order []string }

type fakeCanvas struct{ log *releaseLog }
type fakeCloser struct {
	log  *releaseLog
	name string
}
type fakeReleaser struct {
	log  *releaseLog
	name string
}

func (f fakeCanvas) Close() error { f.log.order = append(f.log.order, "canvas"); return nil }
func (f fakeCloser) Close()       { f.log.order = append(f.log.order, f.name) }
func (f fakeReleaser) Release()   { f.log.order = append(f.log.order, f.name) }

// Every rotation rebuilds the GPU through release, and it used to give back
// only the surface: the device, adapter, instance and canvas — the things that
// actually own GPU memory — were dropped on the floor once per orientation
// flip. The order is part of the contract: the canvas and the accelerator hold
// resources created on the device and must go first; the surface is retired
// before the device that configured it.
func TestReleaseGivesBackEveryGPUHandleOnceInOrder(t *testing.T) {
	log := &releaseLog{}
	g := &mobileGPU{held: gpuHandles{
		canvas:   fakeCanvas{log},
		accel:    fakeCloser{log, "accelerator"},
		surface:  fakeReleaser{log, "surface"},
		device:   fakeReleaser{log, "device"},
		adapter:  fakeReleaser{log, "adapter"},
		instance: fakeReleaser{log, "instance"},
	}}
	b := NewBridge(nil)
	b.gpu = g

	b.ClearSurface()
	want := []string{"canvas", "accelerator", "surface", "device", "adapter", "instance"}
	if len(log.order) != len(want) {
		t.Fatalf("released %v, want %v", log.order, want)
	}
	for i := range want {
		if log.order[i] != want[i] {
			t.Fatalf("released %v, want %v", log.order, want)
		}
	}
	if b.gpu != nil || b.GPUActive() {
		t.Error("the Bridge still reports a GPU after ClearSurface")
	}

	// Idempotent: a host that clears on background and again on destroy must
	// not double-free.
	g.release()
	if len(log.order) != len(want) {
		t.Errorf("a second release gave handles back again: %v", log.order)
	}
}

// A device-less mobileGPU — what the fallback tests build — releases nothing
// and does not panic.
func TestReleaseWithNoHandlesIsSafe(t *testing.T) {
	g := &mobileGPU{}
	g.release()
	g.release()
}
