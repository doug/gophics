package shell

// Share capability: the platform "share sheet" (iOS/Android share UI, the web
// Share API, the desktop sharing services picker). A Window opts in by
// implementing ShareWindow; widgets reach it via ctx.Share(), nil where
// unsupported. The callback fires on the UI goroutine.

// ShareWindow is implemented by a Window that can present a share sheet. The app
// runner type-asserts the Window to it and publishes Share() to the widget tree.
type ShareWindow interface {
	Share() Share
}

// ShareItem is the content handed to the share sheet. Set whichever fields apply;
// most platforms accept any subset (text, a URL, and/or a single file).
type ShareItem struct {
	Title    string // a short subject/title (used by email, etc.)
	Text     string // the message body
	URL      string // a link to share
	FileName string // optional file to attach, with…
	FileData []byte // …its bytes
}

// Share presents the platform share sheet.
type Share interface {
	// Share opens the share sheet for item. The callback reports completion; a
	// user dismissal reports a nil error (there is no reliable "shared" signal on
	// most platforms).
	Share(ShareItem, func(err error))
}
