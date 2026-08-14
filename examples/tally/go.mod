// Tally is a native, local-first personal-finance app: your data is a plain-text
// beancount file, the bundled Apache-2.0 engine (./bean) does the accounting, and
// gophics draws the UI.
//
// A separate module so an app's dependencies stay out of gophics core — and so
// this doubles as the flagship proof that gophics is "a library, not a platform":
// a real app embedding it via a plain go.mod line. Its only non-stdlib
// dependencies are gophics itself, x/image for fonts, and a decimal package for
// money; nothing GPL-licensed, so it is distributable through the app stores.
module github.com/doug/tally

go 1.26.5

require (
	github.com/doug/gophics v0.0.0-00010101000000-000000000000
	golang.org/x/image v0.45.0
)

require (
	golang.org/x/mobile v0.0.0-20260812174124-2f419b2fb945 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

require (
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.6.1 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/doug/gophics => ../..

tool golang.org/x/mobile/cmd/gobind
