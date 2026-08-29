package shell

// The platform's locale.
//
// intl.Auto() reads LC_ALL, LC_NUMERIC and LANG — environment variables that do
// not exist on Android, iOS or the web. Localization therefore cannot work on
// three of the five targets without a platform source, and the failure is
// silent: intl.Auto falls back to its default and the app formats dates and
// money as en-US on a device set to German, with nothing to indicate the
// setting was never read.
//
// That is not hypothetical. examples/tally calls intl.Auto() and ships on
// Android and iOS.
//
// A Window opts in by implementing LocaleWindow; widgets reach it through
// ctx.Locale(), nil where unsupported — desktop and terminal have the
// environment variables, so they leave it nil and intl.Auto is correct there.

// LocaleWindow is implemented by a Window that can report the platform locale.
type LocaleWindow interface {
	Locale() Locale
}

// Locale reports the user's language and formatting preferences.
//
// It returns a BCP-47 tag rather than a resolved format description, because
// the resolution belongs in intl where the data lives — a shell should say what
// the platform is set to, not decide what that means for a decimal separator.
// Callers pass Tag to intl.Lookup.
type Locale interface {
	// Tag is the platform's current locale as BCP-47, e.g. "de-DE" or "ar-EG".
	// Empty when the platform declines to say, which callers treat as "use the
	// default" rather than as an error.
	Tag() string
	// OnChange registers fn for locale changes. A user can change the system
	// language while an app is running — on both mobile platforms without
	// restarting it — and a UI that read the locale once at startup then
	// disagrees with every other app on the device.
	//
	// Called on the UI goroutine. A nil fn clears the subscription.
	OnChange(fn func(tag string))
}
