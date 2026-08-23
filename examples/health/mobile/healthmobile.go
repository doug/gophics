// Package healthmobile is the gomobile-bind surface for the health app.
//
// It holds only what is health's own: building the tree, and the calls a
// native host uses to feed real HealthKit / Health Connect samples into the
// shared Go UI (package healthui). One widget tree, real device data. See
// design/health-native-providers.md for the iOS/Android host wiring.
//
// Everything generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
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

// dev is the provider the host pushes samples into.
var dev *healthui.DeviceProvider

// Start builds the app with a device-backed provider and must be called once
// from the host before any other call. storeName labels the source in the UI
// ("Apple Health" on iOS, "Health Connect" on Android).
//
// On failure it returns a nil bridge and the error to show — two results
// because the second is an error, which is the one shape gomobile allows.
func Start(storeName string) (*mobile.Bridge, error) {
	dev = healthui.NewDeviceProvider(storeName)
	h, err := app.NewHandler(healthui.App{Provider: dev}, app.Config{
		Font:       goregular.TTF,
		Background: healthui.BG,
	})
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}

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
