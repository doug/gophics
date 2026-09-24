package gogpu

import "testing"

// a11yMock is a platform window with a bridge, counting what is published.
type a11yMock struct {
	mockWindow
	sets      int
	announces int
}

func (m *a11yMock) SetA11yTree([]A11yNode, func(id int)) { m.sets++ }
func (m *a11yMock) AnnounceA11y(string, bool)            { m.announces++ }

// A11ySupported is the probe a shell uses to decide whether to offer
// accessibility. It has to answer without publishing: the alternative probe,
// SetAccessibilityTree(nil, nil), removes the published tree as a side effect,
// which blanks a screen reader's view of the window if it is asked again after
// the first tree went up.
func TestA11ySupportedDoesNotPublish(t *testing.T) {
	m := &a11yMock{}
	app := &App{platWindow: m}
	if !app.A11ySupported() {
		t.Fatal("a window with a bridge reports no accessibility support")
	}
	if m.sets != 0 || m.announces != 0 {
		t.Errorf("the probe published: %d tree(s), %d announcement(s)", m.sets, m.announces)
	}

	// And the negative answers: no window, a window without the bridge.
	if (&App{}).A11ySupported() {
		t.Error("an App with no platform window reports support")
	}
	if (&App{platWindow: &mockWindow{}}).A11ySupported() {
		t.Error("a window without the a11y bridge reports support")
	}
	var none *App
	if none.A11ySupported() {
		t.Error("a nil App reports support")
	}
}
