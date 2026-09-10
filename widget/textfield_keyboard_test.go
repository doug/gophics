package widget

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// The field's options reach the platform keyboard: a password field asks for
// secure entry with autocorrect off, a typed field asks for its layout, and a
// read-only field asks for nothing at all.
func TestFieldOptionsReachTheSoftKeyboard(t *testing.T) {
	mount := func(f TextField) *fakeTextInput {
		ti := &fakeTextInput{}
		o := newOwner()
		o.textInput = ti
		o.SetRoot(f)
		o.FlushBuilds()
		return ti
	}

	ti := mount(TextField{Value: "pw", Obscure: true})
	if ti.shown != 1 || !ti.opts.Secure || ti.opts.Autocorrect {
		t.Errorf("password field asked for %+v (shown %d); want Secure and no autocorrect", ti.opts, ti.shown)
	}

	ti = mount(TextField{Value: "a@b", Keyboard: shell.TextInputEmail})
	if ti.opts.Type != shell.TextInputEmail || !ti.opts.Autocorrect {
		t.Errorf("email field asked for %+v; want the email layout with autocorrect on", ti.opts)
	}

	ti = mount(TextField{Value: "x", NoAutocorrect: true})
	if ti.opts.Autocorrect {
		t.Error("NoAutocorrect field still asked for autocorrect")
	}

	ti = mount(TextField{Value: "fixed", ReadOnly: true})
	if ti.shown != 0 {
		t.Errorf("read-only field raised the keyboard %d times; a native one raises none", ti.shown)
	}
}
