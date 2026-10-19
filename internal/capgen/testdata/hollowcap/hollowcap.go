// Package hollowcap is the same plugin as extcap, written the two wrong ways —
// the fixture the hollow check is tested against.
//
// Nothing here calls widget.NewCapability. That is deliberate: a registration
// in a package-level var would panic during this package's initialization, and
// a package whose init panics cannot be imported by a test that wants to
// *observe* the panic — the test binary would die before running. So the
// providers are exported and the parent package's test registers them itself,
// inside a recover. The shapes are the ones that matter; the var is not.
//
// Keeping them compiled rather than written inline in the test is the point:
// these are the mistakes a plugin author actually makes, and they have to still
// look reasonable. Both do.
package hollowcap

// Scanner is extcap.Scanner again, restated here so this fixture stands alone.
type Scanner interface {
	Scan(found func(code string))
	Stop()
}

// ScannerWindow is the opt-in a real platform window would implement.
type ScannerWindow interface {
	Scanner() Scanner
}

// Window is a stand-in for shell.Window, named by the providers below. It is
// an interface rather than the real thing so this fixture needs no import; the
// providers are assigned to the real signature in the test, which is where the
// compiler checks it.
type Window interface {
	Invalidate()
	SetTitle(title string)
	Close()
	ClipboardRead() (string, error)
	ClipboardWrite(text string) error
	OpenURL(url string) error
	DarkMode() bool
}

// IgnoresWindow is the first wrong way: a provider that returns a capability
// without ever asking the window whether the platform has one.
//
// It is an easy mistake to make and an easy one to defend — "the scanner is
// implemented in this package, so why consult the window?" — and it is wrong
// because the answer is then the same everywhere. An app on a laptop with no
// camera gets a non-nil Scanner, hides nothing, scans forever and finds
// nothing, and cannot tell that from "no barcode in view".
func IgnoresWindow(w Window, post func(func())) Scanner { return doNothing{} }

// doNothing is the hollow implementation: every method compiles, nothing
// happens. This is the shape a caller cannot detect at runtime, which is why
// the gate has to catch it at registration.
type doNothing struct{}

func (doNothing) Scan(found func(code string)) {}
func (doNothing) Stop()                        {}

// UnsafeWrap is the second wrong way, and the one that would survive review:
// the provider consults the window correctly, and the posted wrapper is not
// nil-safe.
//
// On a platform with no scanner the assertion fails, x.Scanner() is never
// reached — but on a platform whose window implements ScannerWindow and
// returns nil (the normal way a shell says "not on this device"), wrap hands
// back a non-nil wrapper around a nil inner. Non-nil to the app's nil check,
// and a nil-pointer panic on first use.
//
// This one gets past NewCapability's registration probe, and must: the probe
// window implements no ScannerWindow, so the assertion fails and the provider
// correctly returns nil. Only a window that implements ScannerWindow reaches
// the wrap, and the framework cannot build one — ScannerWindow is declared
// here. Catching it is what Capability.ProvidedBy exists for, and the test
// uses it.
func UnsafeWrap(w Window, post func(func())) Scanner {
	x, ok := w.(ScannerWindow)
	if !ok {
		return nil
	}
	return unsafeWrapper{inner: x.Scanner(), post: post}
}

type unsafeWrapper struct {
	inner Scanner
	post  func(func())
}

func (u unsafeWrapper) Scan(found func(code string)) { u.inner.Scan(found) }
func (u unsafeWrapper) Stop()                        { u.inner.Stop() }
