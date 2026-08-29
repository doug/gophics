package mobile

import "github.com/doug/gophics/shell"

// Locale makes the Bridge a shell.LocaleWindow.
//
// This is the platform where the capability matters most: intl.Auto() reads
// LC_ALL and LANG, and neither exists on Android or iOS, so without a source
// here an app formats dates and money as en-US on a device set to German — and
// does it silently, because the fallback looks like a successful read.
//
// Pushed rather than asked, like connectivity and battery: the host knows the
// locale at startup and gets told when it changes, so a Go-side request would
// only ever be answered from what the last push said.
func (b *Bridge) Locale() shell.Locale {
	if b.localeTag == "" {
		return nil // no host reporting; intl's default is as good a guess
	}
	return mobileLocale{b}
}

type mobileLocale struct{ b *Bridge }

func (l mobileLocale) Tag() string { return l.b.localeTag }

func (l mobileLocale) OnChange(fn func(string)) { l.b.localeSub = fn }

// SetLocale delivers the platform locale as a BCP-47 tag.
//
// Hosts call it once before the first frame and again whenever the setting
// changes — which a user can do while the app runs, on both platforms, without
// restarting it. iOS reads Locale.current.identifier (converting its
// underscore form), Android Resources.getSystem().configuration.locales[0].
func (b *Bridge) SetLocale(tag string) {
	if b.localeTag == tag {
		return
	}
	first := b.localeTag == ""
	b.localeTag = tag
	if first {
		// The capability did not exist a moment ago, so the runtime has to
		// re-read what this Bridge offers or it stays nil for the window's life.
		b.capabilitiesChanged()
	}
	if b.localeSub != nil {
		b.localeSub(tag)
	}
	b.Invalidate()
}
