package widget

import (
	"strings"
	"testing"
)

type missingThing struct{ X int }

func TestMustOfNamesTheMissingType(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustOf did not panic for an absent Provide")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "missingThing") {
			t.Errorf("panic was %q — it must name the type, since the failing "+
				"line reads MustOf[T]() with T already erased", msg)
		}
	}()
	o := newOwner()
	o.SetRoot(Sized{})
	o.FlushBuilds()
	_ = o.root.ctx().MustOf[missingThing]()
}
