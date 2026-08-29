// Separate module so Ebiten's CGo stays out of gophics core, which is
// zero-CGo. The same reason examples/ssh is its own module.
module gophics-embed-ebiten

go 1.27.0

require (
	github.com/doug/gophics v0.0.0
	github.com/hajimehoshi/ebiten/v2 v2.8.6
	golang.org/x/image v0.44.0
)

require (
	github.com/ebitengine/gomobile v0.0.0-20240911145611-4856209ac325 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/purego v0.8.0 // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.6.1 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/doug/gophics => ../..
