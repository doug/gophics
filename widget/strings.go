package widget

import (
	"sync"

	"github.com/doug/gophics/intl"
)

// The framework's own user-visible strings.
//
// gophics ships a handful: the edit menu's Cut, Copy, Paste and Select All, and
// the accessibility role names a screen reader announces. They are baked into
// the widget layer, so an app cannot translate them however carefully it
// translates its own — a German user long-pressing a text field gets an English
// menu from a framework that has a message catalog sitting unused.
//
// This is deliberately small. It is a catalog for strings gophics itself emits,
// not an i18n framework for apps: an app with its own strings builds its own
// intl.Catalog, and this does not get in the way.
//
// Adding a language means adding a catalog here. Coverage is partial on
// purpose — an unlisted language falls through to English, which is what the
// framework said before, rather than to a blank.

// Message keys. Constants rather than raw strings at the call site, so a typo
// is a compile error instead of a silently untranslated menu item.
const (
	MsgCut       = "cut"
	MsgCopy      = "copy"
	MsgPaste     = "paste"
	MsgSelectAll = "selectAll"
)

var (
	catalogsOnce sync.Once
	catalogs     map[string]*intl.Catalog
)

func buildCatalogs() {
	catalogs = map[string]*intl.Catalog{}
	add := func(lang string, msgs map[string]string) {
		c := intl.NewCatalog(lang)
		for k, v := range msgs {
			c.Set(k, v)
		}
		catalogs[c.Lang()] = c
	}
	add("en", map[string]string{
		MsgCut: "Cut", MsgCopy: "Copy", MsgPaste: "Paste", MsgSelectAll: "Select All",
	})
	add("de", map[string]string{
		MsgCut: "Ausschneiden", MsgCopy: "Kopieren", MsgPaste: "Einfügen",
		MsgSelectAll: "Alles auswählen",
	})
	add("fr", map[string]string{
		MsgCut: "Couper", MsgCopy: "Copier", MsgPaste: "Coller",
		MsgSelectAll: "Tout sélectionner",
	})
	add("es", map[string]string{
		MsgCut: "Cortar", MsgCopy: "Copiar", MsgPaste: "Pegar",
		MsgSelectAll: "Seleccionar todo",
	})
	add("ja", map[string]string{
		MsgCut: "カット", MsgCopy: "コピー", MsgPaste: "ペースト",
		MsgSelectAll: "すべて選択",
	})
}

// Message returns a built-in string in the context's language.
//
// Falls through to English for a language with no catalog, and returns the key
// itself only if English is somehow missing it — a visible placeholder beats an
// empty menu item, because one is a bug report and the other is a mystery.
func (c Ctx) Message(key string) string {
	catalogsOnce.Do(buildCatalogs)

	lang := intl.NewCatalog(c.IntlLocale().Tag).Lang()
	if cat, ok := catalogs[lang]; ok {
		if s := cat.Get(key); s != "" {
			return s
		}
	}
	if s := catalogs["en"].Get(key); s != "" {
		return s
	}
	return key
}
