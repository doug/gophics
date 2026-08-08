package widget

// Platform capabilities (Camera, Haptic, FilePicker, …) are generated from the
// shell.<X>Window interfaces into capabilities_gen.go (here) and
// app/capabilities_gen.go. Add a capability by declaring its interface in
// shell/, then rerun. See docs/design-capabilities.md.
//
//go:generate go run github.com/doug/gophics/internal/capgen
