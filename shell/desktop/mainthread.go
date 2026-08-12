//go:build !js

package desktop

// Main-thread dispatch for the desktop shell.
//
// Platform UI APIs are thread-bound: on macOS, AppKit refuses to so much as
// construct an NSWindow (which an NSOpenPanel is) anywhere but the main thread,
// and it enforces that with an uncatchable Objective-C exception rather than an
// error. gogpu splits its callbacks across two threads — window/input events and
// OnUpdate run on the main thread, while OnDraw runs on a dedicated render thread
// — and gophics drives the widget tree (Build, layout, paint, tickers) from
// OnDraw. So framework code is *not* on the main thread by default, and a
// capability that touched AppKit from Build would abort the process.
//
// runOnMain closes that gap: work is run inline when the caller already is the
// main thread (an input handler is), and otherwise queued for the next OnUpdate,
// which is. Capabilities can then be written without caring who calls them.

// queueMain enqueues fn to run on the main thread at the next frame and asks for
// one, so a queued task can't sit until some unrelated event wakes the loop.
func (w *window) queueMain(fn func()) {
	if fn == nil {
		return
	}
	w.mainMu.Lock()
	w.mainQ = append(w.mainQ, fn)
	w.mainMu.Unlock()
	// Guarded because work can be queued before the shell has a window to wake
	// (and in tests that exercise the queue alone); once the loop is up, the
	// redraw request is what guarantees the next OnUpdate drains this task.
	if w.app != nil {
		w.Invalidate()
	}
}

// drainMain runs everything queued for the main thread. It must only be called
// from the main thread (the shell calls it from gogpu's OnUpdate).
//
// The queue is swapped out under the lock and then run unlocked, so a task that
// queues more work — or calls back into the shell — can't deadlock.
func (w *window) drainMain() {
	w.mainMu.Lock()
	q := w.mainQ
	w.mainQ = nil
	w.mainMu.Unlock()
	for _, fn := range q {
		fn()
	}
}
