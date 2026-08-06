// Package healthmobile is the gomobile-bind surface for the health app: a flat,
// bind-friendly API (ints, floats, strings, []byte) over shell/mobile, plus the
// Push* calls the native host uses to feed real HealthKit / Health Connect
// samples into the shared Go UI (package healthui). One widget tree, real device
// data. See design/health-native-providers.md for the iOS/Android host wiring.
//
// Build the frameworks (needs the Android NDK / Xcode):
//
//	go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
//	gomobile bind -target=ios     -o examples/health/ios/Healthmobile.xcframework       ./examples/health/mobile
//	gomobile bind -target=android -androidapi 26 -o examples/health/android/app/libs/healthmobile.aar ./examples/health/mobile
package healthmobile

import (
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	healthui "github.com/doug/gophics/examples/health/ui"
	"github.com/doug/gophics/shell/mobile"
)

var (
	bridge *mobile.Bridge
	dev    *healthui.DeviceProvider
)

// Start builds the app with a device-backed provider and must be called once
// from the host before any other call. storeName labels the source in the UI
// ("Apple Health" on iOS, "Health Connect" on Android). Returns "" on success
// or an error string.
func Start(storeName string) string {
	dev = healthui.NewDeviceProvider(storeName)
	h, err := app.NewHandler(healthui.App{Provider: dev}, app.Config{
		Font:       goregular.TTF,
		Background: healthui.BG,
	})
	if err != nil {
		return err.Error()
	}
	bridge = mobile.NewBridge(h)
	return ""
}

// --- health data feed (called from the native health-store callbacks) ---

// Metric codes for PushSample, matching healthui.Metric.
const (
	MetricHeartRate = int(healthui.HeartRate)
	MetricSteps     = int(healthui.Steps)
	MetricWeight    = int(healthui.Weight)
	MetricSleep     = int(healthui.Sleep)
)

// SetAuthorized records the result of the platform permission prompt.
func SetAuthorized(ok bool) { dev.SetAuthorized(ok) }

// PushSample feeds one reading from the native health store into metric m. t is
// a metric-relative x coordinate (seconds for the live heart rate, days for
// weight/sleep, hours for steps — see healthui.Sample); capN bounds retained
// history (0 = keep all). Safe to call from any thread.
//
// To backfill a whole series, call this in a loop oldest→newest: the provider
// is fresh each Start, so appended samples build the series. (There is
// deliberately no batch PushSeries — gomobile can't bind a []float64 parameter,
// only []byte, so such a method never appears in the generated iOS/Android
// binding. See design/health-native-providers.md.)
func PushSample(m int, t, v float64, capN int) {
	dev.Push(healthui.Metric(m), t, v, capN)
}

// --- render bridge (mirror of hnmobile; driven by the host each vsync) ---

// Resize sets the surface size in physical pixels and the density scale.
func Resize(widthPx, heightPx int, scale float64) {
	bridge.Resize(widthPx, heightPx, float32(scale))
}

// Touch forwards a touch event: phase 0 down, 1 move, 2 up, 3 cancel — the host
// calls this so taps (Connect, cards, Back) and scrolling work.
func Touch(phase int, xPx, yPx float64) {
	bridge.Touch(phase, float32(xPx), float32(yPx))
}

// NeedsFrame reports whether the UI wants a repaint (poll each vsync). The
// health UI animates continuously, so this stays true while data streams.
func NeedsFrame() bool { return bridge.NeedsFrame() }

// SetSurface hands over the host's native render surface (iOS: CAMetalLayer*;
// Android: ANativeWindow*) so rendering runs on the GPU. Call after the surface
// is created and on every resize/rotation.
func SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float64) {
	bridge.SetSurface(displayHandle, windowHandle, widthPx, heightPx, float32(scale))
}

// ClearSurface releases the GPU surface (call when the host surface is destroyed).
func ClearSurface() { bridge.ClearSurface() }

// GpuActive reports whether GPU rendering is live (false on the iOS Simulator —
// present with the CPU Snapshot path there). See design/mobile-gpu-bringup.md.
func GpuActive() bool { return bridge.GPUActive() }

// RenderFrame renders one frame on the GPU to the surface set by SetSurface.
func RenderFrame(dtSeconds float64) { bridge.RenderFrame(dtSeconds) }

// Snapshot renders one frame offscreen and returns RGBA8888 pixels
// (FrameWidth×FrameHeight) — for the CPU present path and screenshots.
func Snapshot(dtSeconds float64) []byte { return bridge.Snapshot(dtSeconds) }

// FrameWidth / FrameHeight are the last surface size in physical pixels.
func FrameWidth() int  { return bridge.FrameWidth() }
func FrameHeight() int { return bridge.FrameHeight() }
