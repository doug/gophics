// Package mirrormobile is the gomobile-bind surface for the mirror app on
// Android and iOS: a flat, bind-friendly API (ints, floats, strings, []byte)
// over shell/mobile.
//
// gomobile cannot bind package main and carries only a restricted vocabulary
// across the boundary, so the app itself lives in ../ui and this is a thin
// adapter over it — the same split the desktop command and the tests use.
//
// Build the Android library:
//
//	gophics run -platform android -host examples/mirror/android ./examples/mirror/mobile
package mirrormobile

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/mirror/ui"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/mobile"
)

var bridge *mobile.Bridge

// Start builds the app and must be called once from the host before anything
// else. It returns "" on success, or the error to show.
func Start() string {
	h, err := app.NewHandler(ui.App{}, ui.Config())
	if err != nil {
		return err.Error()
	}
	bridge = mobile.NewBridge(h)
	return ""
}

// Resize sets the surface size in physical pixels and the density scale.
func Resize(widthPx, heightPx int, scale float64) {
	bridge.Resize(widthPx, heightPx, float32(scale))
}

// NeedsFrame reports whether the UI wants a repaint (poll each vsync).
func NeedsFrame() bool { return bridge.NeedsFrame() }

// SetSurface hands over the host's native render surface so rendering runs on
// the GPU. Call after the surface is created and on every resize/rotation.
func SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float64) {
	bridge.SetSurface(displayHandle, windowHandle, widthPx, heightPx, float32(scale))
}

// ClearSurface releases the GPU surface (call when the host surface is destroyed).
func ClearSurface() { bridge.ClearSurface() }

// GpuActive reports whether GPU rendering is live.
func GpuActive() bool { return bridge.GPUActive() }

// RenderFrame renders one frame to the surface set by SetSurface.
func RenderFrame(dtSeconds float64) { bridge.RenderFrame(dtSeconds) }

// Snapshot renders one frame offscreen and returns RGBA8888 pixels.
func Snapshot(dtSeconds float64) []byte { return bridge.Snapshot(dtSeconds) }

// FrameWidth / FrameHeight are the pixel dimensions of the last Snapshot.
func FrameWidth() int  { return bridge.FrameWidth() }
func FrameHeight() int { return bridge.FrameHeight() }

// Touch forwards a touch event: phase 0 down, 1 move, 2 up, 3 cancel.
func Touch(phase int, xPx, yPx float64) {
	bridge.Touch(phase, float32(xPx), float32(yPx))
}

// Text forwards committed keyboard text.
func Text(s string) { bridge.Text(s) }

// Key forwards a key by shell.KeyCode value (1=Enter, 2=Backspace, ...).
func Key(code int, pressed bool) { bridge.Key(code, pressed) }

// SetDarkMode forwards the host color scheme.
func SetDarkMode(dark bool) { bridge.SetDarkMode(dark) }

// TakeOpenedURL returns a URL the UI asked to open ("" if none); the host
// launches it in the browser.
func TakeOpenedURL() string { return bridge.TakeOpenedURL() }

// TakeHaptic returns the next queued haptic event's kind (see shell.HapticKind:
// 0 selection, 1 light, 2 medium, 3 heavy, 4 success, 5 warning, 6 error), or -1
// when none are pending. The host drains it each frame and plays it on the OS
// generator (View.performHapticFeedback / Vibrator).
func TakeHaptic() int {
	if k, ok := bridge.TakeHaptic(); ok {
		return k
	}
	return -1
}

// SetInsets forwards safe-area insets in physical pixels.
func SetInsets(top, right, bottom, left float64) {
	bridge.SetInsets(float32(top), float32(right), float32(bottom), float32(left))
}

// TextInputActive reports whether the UI wants the on-screen keyboard.
func TextInputActive() bool { return bridge.TextInputActive() }

// Composition forwards IME preedit (kind: 0 start, 1 update, 2 end).
func Composition(kind int, preedit string, cursor int, committed string) {
	bridge.Composition(kind, preedit, cursor, committed)
}

// Accessibility: the host refreshes then reads the flat node tree and
// activates by ID. Rects are physical pixels.
func A11yRefresh() int             { return bridge.A11yRefresh() }
func A11yID(i int) int             { return bridge.A11yID(i) }
func A11yParent(i int) int         { return bridge.A11yParent(i) }
func A11yRole(i int) string        { return bridge.A11yRole(i) }
func A11yLabel(i int) string       { return bridge.A11yLabel(i) }
func A11yValue(i int) string       { return bridge.A11yValue(i) }
func A11yHint(i int) string        { return bridge.A11yHint(i) }
func A11yX(i int) int              { return bridge.A11yX(i) }
func A11yY(i int) int              { return bridge.A11yY(i) }
func A11yW(i int) int              { return bridge.A11yW(i) }
func A11yH(i int) int              { return bridge.A11yH(i) }
func A11yTappable(i int) bool      { return bridge.A11yTappable(i) }
func A11yChildCount(i int) int     { return bridge.A11yChildCount(i) }
func A11yChild(i int, j int) int   { return bridge.A11yChild(i, j) }
func A11yActivate(id int)          { bridge.A11yActivate(id) }
func A11yHitTest(xPx, yPx int) int { return bridge.A11yHitTest(xPx, yPx) }

// Focused forwards window focus. Note this is *focus*, not visibility: it fires
// for a dialog appearing over the app. Use SetAppState for the run state.
func Focused(f bool) { bridge.Focused(f) }

// SetAppState forwards the host's run state: 0 active, 1 inactive, 2
// background, matching shell.AppState. Android drives it from onResume,
// onPause and onStop.
//
// It matters more here than in most apps: leaving the foreground has to release
// the camera, and a capture light that stays on after you switch away is the
// kind of bug a user notices and does not forgive.
func SetAppState(state int) {
	log.Printf("mirror: app state -> %s", shell.AppState(state))
	bridge.SetAppState(state)
}

// --- native capture hosts ---------------------------------------------------
//
// The host registers its camera and microphone backends here rather than on
// shell/mobile.Bridge directly, because gomobile binds only the packages it is
// given and shell/mobile is not bindable: Bridge.TakeHaptic returns (int, bool),
// and gomobile allows a second result only when it is an error. So the bridge's
// host interfaces are re-declared in this flat surface, which is the same job
// the rest of this file does for every other Bridge method.

// PreviewHost is the native camera backend (Android Camera2, iOS AVFoundation).
// It mirrors shell/mobile.PreviewHost; see that type for the threading rules,
// which matter: frames must be delivered from the camera thread.
type PreviewHost interface {
	AuthorizeCamera(reqID int)
	StartPreview(reqID, facing, width int)
	StopPreview(reqID int)
}

// MonitorHost is the native microphone backend. It mirrors
// shell/mobile.MonitorHost; PCM must be delivered from the audio thread.
type MonitorHost interface {
	AuthorizeMic(reqID int)
	StartMonitoring(reqID int)
	StopMonitoring(reqID int)
}

// SetPreviewHost registers the camera backend. Until it is called,
// ctx.CameraPreview() is nil and an app hides the affordance.
func SetPreviewHost(h PreviewHost) { bridge.SetPreviewHost(previewHost{h}) }

// SetMonitorHost registers the microphone backend.
func SetMonitorHost(h MonitorHost) { bridge.SetMonitorHost(monitorHost{h}) }

type previewHost struct{ h PreviewHost }

func (p previewHost) AuthorizeCamera(reqID int)             { p.h.AuthorizeCamera(reqID) }
func (p previewHost) StartPreview(reqID, facing, width int) { p.h.StartPreview(reqID, facing, width) }
func (p previewHost) StopPreview(reqID int)                 { p.h.StopPreview(reqID) }

type monitorHost struct{ h MonitorHost }

func (m monitorHost) AuthorizeMic(reqID int)    { m.h.AuthorizeMic(reqID) }
func (m monitorHost) StartMonitoring(reqID int) { m.h.StartMonitoring(reqID) }
func (m monitorHost) StopMonitoring(reqID int)  { m.h.StopMonitoring(reqID) }

// DeliverPermission reports a permission decision. Call on the UI thread.
func DeliverPermission(reqID int, granted bool) { bridge.DeliverPermission(reqID, granted) }

// DeliverPreviewReady signals the camera is open. Call on the UI thread.
func DeliverPreviewReady(reqID int) { bridge.DeliverPreviewReady(reqID) }

// FailPreview reports that the camera could not be opened. Call on the UI thread.
func FailPreview(reqID int, msg string) { bridge.FailPreview(reqID, msg) }

// DeliverPreviewFrame hands one RGBA8888 frame over. Call from the camera
// thread, not the UI thread — see shell/mobile.Bridge.DeliverPreviewFrame.
func DeliverPreviewFrame(reqID int, rgba []byte, w, h int) {
	bridge.DeliverPreviewFrame(reqID, rgba, w, h)
}

// DeliverMonitorReady signals capture is live at the given rate. UI thread.
func DeliverMonitorReady(reqID, sampleRate int) { bridge.DeliverMonitorReady(reqID, sampleRate) }

// FailMonitoring reports that the microphone could not be opened. UI thread.
func FailMonitoring(reqID int, msg string) { bridge.FailMonitoring(reqID, msg) }

// DeliverMonitorPCM hands over 16-bit little-endian mono samples. Call from the
// audio thread, not the UI thread.
func DeliverMonitorPCM(reqID int, pcm []byte) { bridge.DeliverMonitorPCM(reqID, pcm) }
