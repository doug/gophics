package apptest_test

import (
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// A SelectableText must be findable by its text, exactly as a Text is. It
// used to emit no semantics node at all, so the one label on a screen that
// was worth selecting — an invite code, say — was the one a test (and a
// screen reader) could not see.
func TestSelectableTextHasLabel(t *testing.T) {
	a := apptest.New(t, widget.SelectableText{S: "INVITE-7Q2K"})
	a.AssertLabel("INVITE-7Q2K")
	if n := a.Node("INVITE-7Q2K"); n != nil && n.Role != layout.RoleText {
		t.Errorf("SelectableText reported role %v, want %v", n.Role, layout.RoleText)
	}
}
