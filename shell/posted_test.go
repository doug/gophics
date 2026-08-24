package shell

import (
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

func (fakePicker) Open(_ OpenOptions, done func([]PickedFile, error)) {
	done([]PickedFile{{Name: "a.epub", Data: []byte("x")}}, nil)
}
func (fakePicker) Save(_ SaveOptions, _ []byte, done func(error)) {
	if done != nil {
		done(errors.New("boom"))
	}
}

func TestPostedFilePickerMarshalsCallbacks(t *testing.T) {
	d := &deferredPost{}
	fp := PostedFilePicker(fakePicker{}, d.post)

	fired := false
	fp.Open(OpenOptions{}, func(files []PickedFile, err error) {
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
	fp.Save(SaveOptions{}, nil, nil)
	if len(d.q) != 0 {
		t.Fatal("nil callback should not enqueue anything")
	}
}

// fakeSocket fires OnMessage/OnClose synchronously from Dial — standing in for
// a native client that would fire them from a background read goroutine. The
// callbacks live in a struct (SocketHandlers), which the generated PostedSocket
// must still marshal through post (the struct-of-callbacks case).
type fakeSocket struct{}

func (fakeSocket) Dial(_ string, h SocketHandlers) {
	if h.OnMessage != nil {
		h.OnMessage([]byte("hi"))
	}
	if h.OnClose != nil {
		h.OnClose(nil)
	}
}

func TestPostedSocketMarshalsStructCallbacks(t *testing.T) {
	d := &deferredPost{}
	s := PostedSocket(fakeSocket{}, d.post)

	var got []byte
	closed := false
	s.Dial("ws://x", SocketHandlers{
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
	if PostedFilePicker(nil, func(fn func()) {}) != nil {
		t.Error("nil inner must stay nil")
	}
	inner := fakePicker{}
	if got := PostedFilePicker(inner, nil); got != FilePicker(inner) {
		t.Error("nil post must return inner unwrapped")
	}
}

// fakeMic delivers a Recorder synchronously; the Recorder's Stop callback is
// also synchronous. The posted wrapper must marshal both hops.
type fakeMic struct{}

func (fakeMic) Authorize(cb func(Permission)) { cb(PermissionGranted) }

// Listen is unused here; Record is what this test wraps.
func (fakeMic) Listen(cb func(Monitor, error)) { cb(nil, nil) }
func (fakeMic) Record(_ RecordOptions, cb func(Recorder, error)) {
	cb(fakeRecorder{}, nil)
}
func (fakeMic) Play(_ Clip, cb func(Playback, error)) { cb(nil, errors.New("no audio")) }

type fakeRecorder struct{}

func (fakeRecorder) Level() float32         { return 0.5 }
func (fakeRecorder) Elapsed() time.Duration { return time.Second }
func (fakeRecorder) Stop(cb func(Clip, error)) {
	cb(Clip{Mime: "audio/wav"}, nil)
}
func (fakeRecorder) Cancel() {}

func TestPostedMicrophoneWrapsRecorderRecursively(t *testing.T) {
	d := &deferredPost{}
	a := PostedMicrophone(fakeMic{}, d.post)

	var rec Recorder
	a.Record(RecordOptions{}, func(r Recorder, err error) { rec = r })
	if rec != nil {
		t.Fatal("Record callback fired inline")
	}
	d.drain()
	if rec == nil {
		t.Fatal("Record callback never delivered")
	}
	// Synchronous pass-throughs still work on the wrapped Recorder.
	if rec.Level() != 0.5 {
		t.Errorf("Level = %v", rec.Level())
	}
	// The recursively-wrapped Recorder must post its Stop callback too.
	var clip Clip
	rec.Stop(func(c Clip, err error) { clip = c })
	if clip.Mime != "" {
		t.Fatal("Stop callback fired inline; recursive wrapping missing")
	}
	d.drain()
	if clip.Mime != "audio/wav" {
		t.Fatalf("Stop clip = %+v", clip)
	}
}
