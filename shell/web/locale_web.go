//go:build js && wasm

// Web implementation of the locale capability (shell/locale.go) using
// navigator.language and the window languagechange event.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Locale reports the browser's preferred language. navigator.language is
// universally available, so this is always returned on web — unlike desktop,
// where the environment variables intl.Auto reads are the right source and this
// capability is nil.
func (w *window) Locale() shell.Locale { return &webLocale{} }

type webLocale struct {
	cb    func(string)
	wired bool // the languagechange listener is registered on first OnChange
}

func (l *webLocale) Tag() string {
	// navigator.languages[0] is the ordered preference list and language is the
	// single best guess; they agree except where a user has ranked several, and
	// the list is the more accurate answer.
	nav := js.Global().Get("navigator")
	if langs := nav.Get("languages"); langs.Truthy() && langs.Length() > 0 {
		return langs.Index(0).String()
	}
	if v := nav.Get("language"); v.Truthy() {
		return v.String()
	}
	return ""
}

func (l *webLocale) OnChange(fn func(string)) {
	l.cb = fn
	if fn == nil || l.wired {
		return
	}
	l.wired = true
	// languagechange fires when the user changes their browser language without
	// reloading, which is the case a startup-only read gets wrong.
	js.Global().Call("addEventListener", "languagechange",
		js.FuncOf(func(js.Value, []js.Value) any {
			if l.cb != nil {
				l.cb(l.Tag())
			}
			return nil
		}))
}
