// Package bind is the gomobile-facing surface of the shell bridge.
//
// A native host cannot talk to shell/mobile.Bridge directly. gomobile exposes
// only the packages it is told to bind, and shell/mobile cannot be one of them:
// Bridge.TakeHaptic returns (int, bool) and gomobile permits a second result
// only when it is an error, and gobind copies Go doc comments into Javadoc,
// where prose like "Deliver*/Fail*" closes the comment early and turns English
// into invalid Java. Both are fixable, but fixing them would bind shell/mobile
// to gomobile's vocabulary — and to its comment syntax — permanently.
//
// So this package absorbs those restrictions instead. It is deliberately
// bind-shaped: single returns, scalars and []byte, no maps, no doc comment
// containing a slash-star. That is its whole job, and it is why the
// restrictions stop here rather than reaching a package that also serves
// desktop and terminal.
//
// An app binds this alongside its own bind package:
//
//	gomobile bind ./mobile github.com/doug/gophics/shell/mobile/bind
//
// and the app's package shrinks to Start plus whatever host traffic is its
// own — see the health example, which carries HealthKit samples nothing else
// needs. Everything generic lives here, written once.
package bind

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/mobile"
)

// b is the process's bridge. A gomobile app has exactly one.
var b *mobile.Bridge

// Attach hands the bridge over. The app's bind package calls it from Start,
// once, before the host calls anything else here.
func Attach(x *mobile.Bridge) { b = x }

// Attached reports whether Attach has run — the host's way of catching a
// start-up ordering bug rather than a nil dereference.
func Attached() bool { return b != nil }

// --- frame loop -------------------------------------------------------------

// Resize sets the surface size in physical pixels and the density scale.
func Resize(widthPx, heightPx int, scale float64) {
	b.Resize(widthPx, heightPx, float32(scale))
}

// NeedsFrame reports whether the UI wants a repaint; poll it each vsync.
func NeedsFrame() bool { return b.NeedsFrame() }

// RenderFrame renders one frame to the surface set by SetSurface.
func RenderFrame(dtSeconds float64) { b.RenderFrame(dtSeconds) }

// SetSurface hands over the host's native render surface so rendering runs on
// the GPU (iOS: a CAMetalLayer*; Android: an ANativeWindow*). Call after the
// surface is created and on every resize or rotation.
func SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float64) {
	b.SetSurface(displayHandle, windowHandle, widthPx, heightPx, float32(scale))
}

// ClearSurface releases the GPU surface when the host surface is destroyed.
func ClearSurface() { b.ClearSurface() }

// GpuActive reports whether GPU rendering is live. When false, present with
// the CPU path: each vsync call Snapshot and blit the returned pixels.
func GpuActive() bool { return b.GPUActive() }

// Snapshot renders one frame offscreen and returns RGBA8888 pixels — for
// screenshots and tests, not the live loop.
func Snapshot(dtSeconds float64) []byte { return b.Snapshot(dtSeconds) }

// FrameWidth and FrameHeight are the pixel dimensions of the last Snapshot.
func FrameWidth() int  { return b.FrameWidth() }
func FrameHeight() int { return b.FrameHeight() }

// --- input ------------------------------------------------------------------

// Touch forwards a touch: phase 0 down, 1 move, 2 up, 3 cancel.
func Touch(phase int, xPx, yPx float64) { b.Touch(phase, float32(xPx), float32(yPx)) }

// Text forwards committed keyboard text.
func Text(s string) { b.Text(s) }

// Key forwards a key by shell.KeyCode value (1 Enter, 2 Backspace, …).
func Key(code int, pressed bool) { b.Key(code, pressed) }

// Composition forwards IME preedit: kind 0 start, 1 update, 2 end.
func Composition(kind int, preedit string, cursor int, committed string) {
	b.Composition(kind, preedit, cursor, committed)
}

// TextInputActive reports whether the UI wants the on-screen keyboard.
func TextInputActive() bool { return b.TextInputActive() }

// --- lifecycle and chrome ---------------------------------------------------

// Focused forwards window focus. This is focus, not visibility: it fires for a
// dialog appearing over the app. Use SetAppState for the run state.
func Focused(f bool) { b.Focused(f) }

// SetAppState forwards the run state: 0 active, 1 inactive, 2 background.
func SetAppState(state int) { b.SetAppState(state) }

// AppStateName renders a state for logging, so a host need not keep its own
// table in step with shell.AppState.
func AppStateName(state int) string { return shell.AppState(state).String() }

// SetDarkMode forwards the host colour scheme.
func SetDarkMode(dark bool) { b.SetDarkMode(dark) }

// SetInsets forwards safe-area insets in physical pixels.
func SetInsets(top, right, bottom, left float64) {
	b.SetInsets(float32(top), float32(right), float32(bottom), float32(left))
}

// TakeOpenedURL returns a URL the UI asked to open, or "" — the host launches
// it in the browser.
func TakeOpenedURL() string { return b.TakeOpenedURL() }

// TakeHaptic returns the next queued haptic's kind (see shell.HapticKind), or
// -1 when none are pending.
//
// The Bridge reports (kind, ok); gomobile allows a second result only when it
// is an error, so the absence becomes -1 here. Every app's bind package used
// to write this same wrapper.
func TakeHaptic() int {
	if k, ok := b.TakeHaptic(); ok {
		return k
	}
	return -1
}

// --- accessibility ----------------------------------------------------------
//
// The platform pulls this tree on its own schedule — Android's
// AccessibilityNodeProvider and iOS's UIAccessibility both query when focus
// moves — so it is a flat, index-addressed surface rather than a callback.
// Rects are physical pixels.

// A11yRefresh rebuilds the node tree and returns the node count.
func A11yRefresh() int             { return b.A11yRefresh() }
func A11yID(i int) int             { return b.A11yID(i) }
func A11yParent(i int) int         { return b.A11yParent(i) }
func A11yRole(i int) string        { return b.A11yRole(i) }
func A11yLabel(i int) string       { return b.A11yLabel(i) }
func A11yValue(i int) string       { return b.A11yValue(i) }
func A11yHint(i int) string        { return b.A11yHint(i) }
func A11yX(i int) int              { return b.A11yX(i) }
func A11yY(i int) int              { return b.A11yY(i) }
func A11yW(i int) int              { return b.A11yW(i) }
func A11yH(i int) int              { return b.A11yH(i) }
func A11yTappable(i int) bool      { return b.A11yTappable(i) }
func A11yChildCount(i int) int     { return b.A11yChildCount(i) }
func A11yChild(i, j int) int       { return b.A11yChild(i, j) }
func A11yActivate(id int)          { b.A11yActivate(id) }
func A11yHitTest(xPx, yPx int) int { return b.A11yHitTest(xPx, yPx) }

// --- capture hosts ----------------------------------------------------------
//
// The interfaces are re-declared rather than reused so gomobile can see them:
// it exports only what the bound packages declare. See shell/mobile for the
// threading rules, which are load-bearing — frames and PCM come from their own
// threads, everything else from the UI thread.

// PreviewHost is the native camera backend (Android Camera2, iOS AVFoundation).
type PreviewHost interface {
	AuthorizeCamera(reqID int)
	StartPreview(reqID, facing, width int)
	StopPreview(reqID int)
}

// MonitorHost is the native microphone backend.
type MonitorHost interface {
	AuthorizeMic(reqID int)
	StartMonitoring(reqID int)
	StopMonitoring(reqID int)
}

// SetPreviewHost registers the camera backend. Until it is called,
// ctx.CameraPreview() is nil and an app hides the affordance.
func SetPreviewHost(h PreviewHost) { b.SetPreviewHost(previewAdapter{h}) }

// SetMonitorHost registers the microphone backend.
func SetMonitorHost(h MonitorHost) { b.SetMonitorHost(monitorAdapter{h}) }

type previewAdapter struct{ h PreviewHost }

func (a previewAdapter) AuthorizeCamera(r int)             { a.h.AuthorizeCamera(r) }
func (a previewAdapter) StartPreview(r, facing, width int) { a.h.StartPreview(r, facing, width) }
func (a previewAdapter) StopPreview(r int)                 { a.h.StopPreview(r) }

type monitorAdapter struct{ h MonitorHost }

func (a monitorAdapter) AuthorizeMic(r int)    { a.h.AuthorizeMic(r) }
func (a monitorAdapter) StartMonitoring(r int) { a.h.StartMonitoring(r) }
func (a monitorAdapter) StopMonitoring(r int)  { a.h.StopMonitoring(r) }

// DeliverPermission reports a permission decision. Call on the UI thread.
func DeliverPermission(reqID int, granted bool) { b.DeliverPermission(reqID, granted) }

// DeliverPreviewReady signals the camera is open. Call on the UI thread.
func DeliverPreviewReady(reqID int) { b.DeliverPreviewReady(reqID) }

// FailPreview reports that the camera could not be opened. UI thread.
func FailPreview(reqID int, msg string) { b.FailPreview(reqID, msg) }

// DeliverPreviewFrame hands over one RGBA8888 frame. Call from the camera
// thread, not the UI thread: it touches no app code, and hopping threads adds
// a frame of latency to the one path whose job is to be current.
func DeliverPreviewFrame(reqID int, rgba []byte, w, h int) {
	b.DeliverPreviewFrame(reqID, rgba, w, h)
}

// DeliverMonitorReady signals capture is live at the rate the device granted.
// UI thread. Report the rate actually granted, not the one requested.
func DeliverMonitorReady(reqID, sampleRate int) { b.DeliverMonitorReady(reqID, sampleRate) }

// FailMonitoring reports that the microphone could not be opened. UI thread.
func FailMonitoring(reqID int, msg string) { b.FailMonitoring(reqID, msg) }

// DeliverMonitorPCM hands over 16-bit little-endian mono samples. Call from
// the audio thread, not the UI thread.
func DeliverMonitorPCM(reqID int, pcm []byte) { b.DeliverMonitorPCM(reqID, pcm) }
