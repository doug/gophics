// Package newsmobile is the gomobile-bind surface for the news reader: a flat,
// bind-friendly API (ints, floats, strings, []byte) over shell/mobile.
//
// Two calls here have no counterpart in the other examples, and both exist
// because a reader needs things a drawing demo does not:
//
// SetDataDir must be called before Start. The reader keeps a store of articles,
// a ranking model, subscriptions and a picture cache on disk, and only the host
// knows where that is allowed to live — Context.getFilesDir on Android, the
// Application Support directory inside the sandbox on iOS.
//
// PendingLoginDomain / SetCookies are the sign-in handshake for paid sources.
// The framework's WebView capability is implemented for the web shell only and
// exposes no cookie access, so logging in to a publisher happens in the host's
// own WebView or WKWebView. Go raises a request, the host polls for it on the
// frame loop it already runs, presents the login, and hands the session back.
//
// Build the Android library (requires the Android NDK):
//
//	go install golang.org/x/mobile/cmd/gomobile@latest
//	gomobile init
//	gomobile bind -target=android -androidapi 24 -o examples/news/android/app/libs/newsmobile.aar ./examples/news/mobile
//
// then open examples/news/android with Gradle (see its README).
package newsmobile

import (
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/examples/news/ui"
	"github.com/doug/gophics/shell/mobile"
)

// SetDataDir tells the reader where it may keep its store, ranking model,
// subscriptions and picture cache. Call it before Start.
//
// Android must pass Context.getFilesDir().getAbsolutePath(); iOS must pass the
// Application Support directory inside the app sandbox. Without it the reader
// falls back to the platform config directory, which on Android is not
// somewhere an app may write.
func SetDataDir(dir string) { library.SetDataDir(dir) }

// Start builds the app and must be called once from the host, after SetDataDir
// and before any other call. The scene is the reader, or the GPU bring-up check
// when built with -tags gophics_verify (see scene.go).
// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
	root, bg := scene()
	h, err := app.NewHandler(root, app.Config{
		Font:         goregular.TTF,
		FontFamilies: ui.Fonts(),
		Background:   bg,
	})
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
