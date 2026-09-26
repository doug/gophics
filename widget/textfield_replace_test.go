package widget

import "testing"

// An IME replacement (autocorrect, prediction) is capped by MaxLength like an
// insertion, counting the span it replaces as room.
func TestIMEReplaceCappedByMaxLength(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti
	var got []string
	o.SetRoot(TextField{Autofocus: true, Value: "abc", MaxLength: 4,
		OnChange: func(s string) { got = append(got, s) }})
	o.FlushBuilds()
	if ti.handler.OnReplace == nil {
		t.Fatal("no OnReplace handler given to the platform")
	}
	// "abc" with "bc" replaced by "xyzw" would be "axyzw": one over the cap.
	ti.handler.OnReplace(1, 3, "xyzw")
	if len(got) != 1 || got[0] != "axyz" {
		t.Errorf("replacement gave %q, want [axyz] (MaxLength 4)", got)
	}
}

// A field that turned read-only under a live IME session declines the
// replacement, as onText declines typed text.
func TestIMEReplaceDeclinedByReadOnlyField(t *testing.T) {
	ti := &fakeTextInput{}
	o := newOwner()
	o.textInput = ti
	var got []string
	o.SetRoot(TextField{Autofocus: true, Value: "abc",
		OnChange: func(s string) { got = append(got, s) }})
	o.FlushBuilds()
	o.SetRoot(TextField{Autofocus: true, Value: "abc", ReadOnly: true,
		OnChange: func(s string) { got = append(got, s) }})
	o.FlushBuilds()
	ti.handler.OnReplace(1, 3, "xy")
	if len(got) != 0 {
		t.Errorf("a read-only field accepted an IME replacement: %q", got)
	}
}
