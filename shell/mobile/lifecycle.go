package mobile

import "github.com/doug/gophics/shell"

// Lifecycle makes the Bridge a shell.LifecycleWindow. It returns nil today,
// leaving ctx.Lifecycle() nil so callers fall back to always-running behavior.
//
// TODO(android/ios lifecycle callbacks): drive run state from the host activity/
// app lifecycle over the bridge — Android onStart/onStop/onPause/onResume and
// iOS sceneWillEnterForeground/DidEnterBackground/resign/becomeActive — mapping
// foreground+focused→StateActive, foreground+resigned→StateInactive, and
// backgrounded→StateBackground. Note the existing Bridge.Focused inbound signal
// is window focus, not background, so it is intentionally not reused as a proxy
// for the background transition.
func (b *Bridge) Lifecycle() shell.Lifecycle { return nil }
