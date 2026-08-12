//go:build !darwin && !js

package desktop

// runOnMain runs fn immediately: no capability on these platforms is main-thread
// bound yet. When one lands (Win32 dialogs are UI-thread bound), give it the same
// isMainThread check the darwin build uses and route through queueMain.
func (w *window) runOnMain(fn func()) {
	if fn != nil {
		fn()
	}
}
