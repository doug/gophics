package widget

import (
	"sync"

	"github.com/doug/gophics/intl"
)

// Locale resolution: the bridge from the platform capability to intl.
//
// intl has had the data and the formatters all along — Number, Money, Date,
// and a plural catalog — and nothing imported it. The missing piece was never
// the formatting; it was knowing which locale to format for. intl.Auto() reads
// LC_ALL and LANG, which exist on desktop and on none of Android, iOS or the
// web, so three of five targets had no way to answer the question at all.
//
// ctx.Locale() is the platform's answer, this turns it into an intl.Locale, and
// the widgets below format through it. A shell that leaves the capability nil —
// desktop and terminal, where the environment variables are the right source —
// falls back to intl.Auto, which is correct there.

var (
	autoOnce   sync.Once
	autoLocale intl.Locale
)

// Locale returns the formatting rules for this context.
//
// The platform capability wins where there is one; otherwise intl.Auto reads
// the environment, and its default is the last resort. An unrecognised tag also
// falls back rather than erroring: a UI that renders nothing because the device
// is set to a language the table does not carry is worse than one that renders
// in the default.
//
// Cheap enough to call from Build — the auto lookup is done once, and a
// platform tag is a map hit.
func (c Ctx) IntlLocale() intl.Locale {
	if pl := c.Locale(); pl != nil {
		if l, ok := intl.Lookup(pl.Tag()); ok {
			return l
		}
	}
	autoOnce.Do(func() { autoLocale = intl.Auto() })
	return autoLocale
}
