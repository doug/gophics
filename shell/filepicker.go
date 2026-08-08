package shell

// File-picking capability. A Window exposes it by implementing FilePickerWindow;
// callers reach it through the widget layer (ctx.FilePicker()), which returns nil
// when the running platform can't provide one. The web shell implements it with a
// hidden <input type=file>; desktop shells use the native open/save panels; the
// mobile shells use the platform document/photo picker. All callbacks fire on the
// UI goroutine.
//
// Files are carried as bytes rather than paths on purpose: the web File API and
// mobile content URIs don't expose a stable filesystem path, so bytes are the one
// portable currency across every platform.

// FilePickerWindow is implemented by a Window that can present a file chooser.
// The app runner type-asserts the Window to it and, when present, publishes
// FilePicker() to the widget tree — the same shape as MediaWindow/HapticWindow.
type FilePickerWindow interface {
	FilePicker() FilePicker
}

// PickedFile is one file returned by an open dialog: its display name and full
// contents.
type PickedFile struct {
	Name string
	Data []byte
}

// OpenOptions configures an open dialog.
type OpenOptions struct {
	// Accept limits selectable files by extension and/or MIME type, e.g.
	// {".epub", "application/epub+zip"} or {"image/*"}. Empty accepts anything.
	// It is a platform hint (the file dialog's filter), not a hard guarantee.
	Accept []string
	// Multiple allows selecting more than one file.
	Multiple bool
}

// SaveOptions configures a save dialog.
type SaveOptions struct {
	// Name is the suggested file name (e.g. "export.csv").
	Name string
	// Accept hints the file type/extension, as in OpenOptions.
	Accept []string
}

// FilePicker presents the platform file chooser. Must be invoked from a user
// gesture (a tap) — browsers only open a file dialog then.
type FilePicker interface {
	// Open presents an open dialog and reports the chosen files' bytes. On
	// cancel it reports an empty slice and a nil error.
	Open(OpenOptions, func(files []PickedFile, err error))
	// Save presents a save dialog for data. On web this triggers a download (and
	// reports a nil error once started); on desktop/mobile it writes the file the
	// user chose. Cancel reports a nil error.
	Save(SaveOptions, []byte, func(err error))
}
