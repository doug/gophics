package widget

import (
	"github.com/doug/gophics/shell"

	"errors"
	"testing"
	"time"
)

// deferredPost is a fake UI-goroutine scheduler: it queues posted funcs and
// runs them only when drained, so a test can assert a callback did NOT fire
// inline and DID fire on the "UI goroutine".
type deferredPost struct{ q []func() }

func (d *deferredPost) post(fn func()) { d.q = append(d.q, fn) }
func (d *deferredPost) drain() {
	for len(d.q) > 0 {
		fn := d.q[0]
		d.q = d.q[1:]
		fn()
	}
}

// fakePicker invokes its callback synchronously from Open — the worst case a
// platform implementation is now allowed to be.
type fakePicker struct{}

func (fakePicker) Open(_ shell.OpenOptions, done func([]shell.PickedFile, error)) {
	done([]shell.PickedFile{{Name: "a.epub", Data: []byte("x")}}, nil)
}
func (fakePicker) Save(_ shell.SaveOptions, _ []byte, done func(error)) {
	if done != nil {
		done(errors.New("boom"))
	}
}

func TestPostedFilePickerMarshalsCallbacks(t *testing.T) {
	d := &deferredPost{}
	fp := postedFilePickerOf(fakePicker{}, d.post)

	fired := false
	fp.Open(shell.OpenOptions{}, func(files []shell.PickedFile, err error) {
		fired = true
		if len(files) != 1 || files[0].Name != "a.epub" {
			t.Errorf("files = %v", files)
		}
	})
	if fired {
		t.Fatal("Open callback fired inline; must be deferred through post")
	}
	if len(d.q) != 1 {
		t.Fatalf("expected 1 queued post, got %d", len(d.q))
	}
	d.drain()
	if !fired {
		t.Fatal("Open callback never delivered")
	}

	// A nil done must pass through as nil (Save's contract allows nil).
	fp.Save(shell.SaveOptions{}, nil, nil)
	if len(d.q) != 0 {
		t.Fatal("nil callback should not enqueue anything")
	}
}

// fakeSocket fires OnMessage/OnClose synchronously from Dial — standing in for
// a native client that would fire them from a background read goroutine. The
// callbacks live in a struct (shell.SocketHandlers), which the generated PostedSocket
// must still marshal through post (the struct-of-callbacks case).
type fakeSocket struct{}

func (fakeSocket) Dial(_ string, h shell.SocketHandlers) {
	if h.OnMessage != nil {
		h.OnMessage([]byte("hi"))
	}
	if h.OnClose != nil {
		h.OnClose(nil)
	}
}

func TestPostedSocketMarshalsStructCallbacks(t *testing.T) {
	d := &deferredPost{}
	s := postedSocketOf(fakeSocket{}, d.post)

	var got []byte
	closed := false
	s.Dial("ws://x", shell.SocketHandlers{
		OnMessage: func(b []byte) { got = b },
		OnClose:   func(error) { closed = true },
	})
	if got != nil || closed {
		t.Fatal("socket callbacks fired inline; struct-of-callbacks not marshaled through post")
	}
	if len(d.q) != 2 {
		t.Fatalf("expected 2 queued posts (message + close), got %d", len(d.q))
	}
	d.drain()
	if string(got) != "hi" || !closed {
		t.Fatalf("callbacks not delivered: got=%q closed=%v", got, closed)
	}
}

func TestPostedNilSafety(t *testing.T) {
	if postedFilePickerOf(nil, func(fn func()) {}) != nil {
		t.Error("nil inner must stay nil")
	}
	inner := fakePicker{}
	if got := postedFilePickerOf(inner, nil); got != shell.FilePicker(inner) {
		t.Error("nil post must return inner unwrapped")
	}
}

// fakeMic delivers a shell.Recorder synchronously; the shell.Recorder's Stop callback is
// also synchronous. The posted wrapper must marshal both hops.
type fakeMic struct{}

func (fakeMic) Authorize(cb func(shell.Permission)) { cb(shell.PermissionGranted) }

// Listen is unused here; Record is what this test wraps.
func (fakeMic) Listen(cb func(shell.Monitor, error)) { cb(nil, nil) }
func (fakeMic) Record(_ shell.RecordOptions, cb func(shell.Recorder, error)) {
	cb(fakeRecorder{}, nil)
}
func (fakeMic) Play(_ shell.Clip, cb func(shell.Playback, error)) { cb(nil, errors.New("no audio")) }

type fakeRecorder struct{}

func (fakeRecorder) Level() float32         { return 0.5 }
func (fakeRecorder) Elapsed() time.Duration { return time.Second }
func (fakeRecorder) Stop(cb func(shell.Clip, error)) {
	cb(shell.Clip{Mime: "audio/wav"}, nil)
}
func (fakeRecorder) Cancel() {}

func TestPostedMicrophoneWrapsRecorderRecursively(t *testing.T) {
	d := &deferredPost{}
	a := postedMicrophoneOf(fakeMic{}, d.post)

	var rec shell.Recorder
	a.Record(shell.RecordOptions{}, func(r shell.Recorder, err error) { rec = r })
	if rec != nil {
		t.Fatal("Record callback fired inline")
	}
	d.drain()
	if rec == nil {
		t.Fatal("Record callback never delivered")
	}
	// Synchronous pass-throughs still work on the wrapped shell.Recorder.
	if rec.Level() != 0.5 {
		t.Errorf("Level = %v", rec.Level())
	}
	// The recursively-wrapped shell.Recorder must post its Stop callback too.
	var clip shell.Clip
	rec.Stop(func(c shell.Clip, err error) { clip = c })
	if clip.Mime != "" {
		t.Fatal("Stop callback fired inline; recursive wrapping missing")
	}
	d.drain()
	if clip.Mime != "audio/wav" {
		t.Fatalf("Stop clip = %+v", clip)
	}
}
