// Tally is a native, local-first personal-finance app: your data is a plain-text
// beancount file, the beango engine does the accounting, and gophics draws the UI.
//
// A separate module so the beancount engine (beango) and its dependency tree stay
// out of gophics core — and so this doubles as the flagship proof that gophics is
// "a library, not a platform": a real app embedding it via a plain go.mod line.
// It replaces gophics and beango with the local checkouts.
module github.com/dougfritz/tally

go 1.26.5

require (
	github.com/doug/gophics v0.0.0-00010101000000-000000000000
	github.com/dougfritz/beango v0.0.0
	golang.org/x/image v0.45.0
)

require (
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/go-webgpu/goffi v0.6.1 // indirect
	github.com/shopspring/decimal v1.4.0
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace github.com/doug/gophics => ../..

replace github.com/dougfritz/beango => ../../../beango
