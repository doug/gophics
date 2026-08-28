package shell

// Biometric authentication — Face ID, Touch ID, Android BiometricPrompt.
//
// This authenticates the *person holding the device*, which is a different
// question from the ones SecureStorage and Permissions answer. It pairs with
// SecureStorage rather than replacing it: the usual shape is a token in the
// keychain and a biometric check before using it, so an unlocked phone left on
// a table does not also unlock the app.
//
// It deliberately does not return a secret, a key, or a token. Some platforms
// can gate a keychain item on biometry directly, but exposing that through this
// interface would make every implementation that cannot do it look the same as
// one that can, while being materially less safe. What this reports is the one
// thing every platform agrees on: whether the check passed.
//
// A Window opts in by implementing BiometricWindow; widgets reach it via
// ctx.Biometric(), nil where unsupported.

// BiometricWindow is implemented by a Window that can run a biometric check.
type BiometricWindow interface {
	Biometric() Biometric
}

// BiometricKind describes what the device can ask for, so an app can label its
// own button correctly rather than guessing.
type BiometricKind uint8

const (
	// BiometricNone means the device has no enrolled biometry. It may still have
	// a passcode, which Authenticate can fall back to.
	BiometricNone BiometricKind = iota
	BiometricFingerprint
	BiometricFace
	// BiometricOther is a check the platform offers that is neither of the
	// above (iris, an unnamed sensor). Label it generically.
	BiometricOther
)

// Biometric runs a platform identity check.
type Biometric interface {
	// Available reports what the device can ask for right now — which is not a
	// static property: biometry can be enrolled, removed, or locked out after
	// repeated failures. Call it when you draw the affordance, not once at
	// startup.
	Available() BiometricKind
	// Authenticate presents the platform prompt.
	//
	// reason is shown to the user and must say why ("Unlock your vault"), because
	// both platforms display it verbatim in a system dialog the app does not
	// control.
	//
	// allowFallback permits the device passcode when biometry fails or is not
	// enrolled. Prefer true: a user whose fingerprint is not read on a cold
	// morning still needs to get in, and a passcode is what the device already
	// trusts.
	//
	// ok is false for every negative outcome — failed, cancelled, locked out —
	// with err carrying the detail. Treat the distinction as advisory: an app
	// should not vary what it *grants* by why the check failed.
	Authenticate(reason string, allowFallback bool, done func(ok bool, err error))
}
