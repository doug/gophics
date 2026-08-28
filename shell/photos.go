package shell

// Writing an image to the user's photo library.
//
// This is deliberately separate from FilePicker, which reads. Saving a picture
// is not "save a file somewhere": the photo library is a specific destination
// with its own permission, its own album semantics, and a place in the user's
// mental model that a document picker does not occupy. An app that has just
// composed, edited or captured an image wants it *in the camera roll*, and
// offering a file browser instead is the wrong answer to the question.
//
// It is write-only on purpose. Reading the library is what FilePicker's photo
// path already does through the system picker, which shows the user exactly what
// they are handing over and needs no library-wide permission — so there is no
// Read here to tempt an app into asking for one.
//
// A Window opts in by implementing PhotosWindow; widgets reach it via
// ctx.Photos(), nil where unsupported.

// PhotosWindow is implemented by a Window that can write to the photo library.
type PhotosWindow interface {
	Photos() Photos
}

// Photos writes images to the platform photo library.
type Photos interface {
	// Authorize requests add-only access to the photo library.
	//
	// Add-only rather than full access where the platform distinguishes them
	// (iOS does): an app that saves a picture has no business enumerating the
	// user's library, and asking for less is both more likely to be granted and
	// the honest request.
	Authorize(func(Permission))
	// Save writes an encoded image (PNG or JPEG bytes) to the library.
	//
	// album is an optional album name to file it under; "" saves to the default
	// camera roll. Platforms that have no album concept ignore it.
	//
	// The callback reports completion. A denied permission is an error, not a
	// silent no-op, because the app usually wants to say so.
	Save(data []byte, album string, done func(err error))
}
