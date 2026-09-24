//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

package desktop

import (
	"errors"
	"slices"
	"testing"

	"github.com/doug/gophics/shell"
)

// recorder stands in for notify-send: it remembers each argv and answers with
// a canned id or error.
type recorder struct {
	calls [][]string
	ids   []string // returned in order, one per call
	errs  []error  // likewise; nil entries succeed
}

func (r *recorder) run(args ...string) ([]byte, error) {
	i := len(r.calls)
	r.calls = append(r.calls, slices.Clone(args))
	var err error
	if i < len(r.errs) {
		err = r.errs[i]
	}
	id := ""
	if i < len(r.ids) {
		id = r.ids[i]
	}
	return []byte(id + "\n"), err
}

// A tagged notification asks for its id (-p) and replaces the previous one
// with the same tag (-r id); an untagged one asks for nothing. This is the
// whole point of Tag and it ran only against a live daemon.
func TestNotifyTagsReplaceThroughPrintAndReplace(t *testing.T) {
	rec := &recorder{ids: []string{"41", "42"}}
	n := &unixNotifier{ids: map[string]string{}, run: rec.run}

	n.post(shell.Notification{Title: "Build", Body: "started", Tag: "build"})
	n.post(shell.Notification{Title: "Build", Body: "done", Tag: "build"})
	n.post(shell.Notification{Title: "Other", Body: "thing"})

	want := [][]string{
		{"-p", "Build", "started"},
		{"-p", "-r", "41", "Build", "done"},
		{"Other", "thing"},
	}
	if len(rec.calls) != len(want) {
		t.Fatalf("notify-send ran %d times: %q; want %q", len(rec.calls), rec.calls, want)
	}
	for i := range want {
		if !slices.Equal(rec.calls[i], want[i]) {
			t.Errorf("call %d = %q, want %q", i, rec.calls[i], want[i])
		}
	}
	if got := n.ids["build"]; got != "42" {
		t.Errorf("remembered id for the tag = %q, want the latest, 42", got)
	}
}

// A notify-send too old for -p/-r fails the tagged call; the notification
// must still appear, untagged, rather than vanish.
func TestNotifyFallsBackWhenTaggingIsUnsupported(t *testing.T) {
	rec := &recorder{errs: []error{errors.New("unknown option -p")}}
	n := &unixNotifier{ids: map[string]string{}, run: rec.run}

	n.post(shell.Notification{Title: "T", Body: "B", Tag: "x"})

	if len(rec.calls) != 2 || !slices.Equal(rec.calls[1], []string{"T", "B"}) {
		t.Fatalf("calls = %q, want the tagged attempt then a plain retry", rec.calls)
	}
	if _, remembered := n.ids["x"]; remembered {
		t.Error("an id was remembered for a call that failed")
	}
}
