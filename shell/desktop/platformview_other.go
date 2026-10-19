//go:build !js && !darwin

package desktop

import "github.com/doug/gophics/shell"

// PlatformViews is nil on Windows and Linux, and this is the honest answer
// rather than a placeholder: an app reads the nil and shows whatever
// widget.PlatformView's Placeholder is, which is what it would have to do
// anyway on a build where the view fails to appear.
//
// The handle is no longer the obstacle. gogpu.Window.NativeHandles returns
// exactly the right parent on all three of these backends — (hinstance, HWND)
// on Windows, (Display*, Window) on X11, (wl_display, wl_surface) on Wayland —
// so parenting a child window or a subsurface is a solved problem here in a
// way it is not on macOS, where the same call answers with a CALayer that no
// view owns.
//
// What is missing is the thing inside the child window. The capability's
// contract is that a listed kind works, and the only kind gophics names that
// a desktop could plausibly offer is "webview":
//
//   - Windows needs WebView2, which is a COM server reached through
//     CreateCoreWebView2EnvironmentWithOptions and then driven over
//     ICoreWebView2 vtables with syscall.SyscallN. Zero-CGo-compatible and
//     entirely mechanical, but it is a few hundred lines of vtable plumbing
//     plus an asynchronous environment/controller handshake, and the runtime
//     it needs is a separate redistributable that an app has to detect rather
//     than assume. Nothing here is hard; it is simply not small.
//   - Linux has no system web view. WebKitGTK is the practical answer and it
//     brings a GTK main loop with it, which has to be driven alongside gogpu's
//     own loop; that integration, not the handle, is the real work. A build
//     without webkit2gtk installed must report the kind as absent, so this is
//     also the one platform where Kinds has to be answered at runtime from a
//     dlopen probe rather than stated.
//
// Accessibility would then be the larger half again, exactly as on macOS: UI
// Automation on Windows and AT-SPI on Linux both publish a virtual tree
// hanging off one opaque window on the premise that gophics draws every pixel,
// so a real child window has to be spliced into that tree inside the vendored
// fork. See the package comment in platformview_darwin.go.
func (w *window) PlatformViews() shell.PlatformViews { return nil }

// WebView follows PlatformViews, because a web view is a kind of platform view
// rather than a capability of its own. Derived rather than written out so that
// implementing the registry here lights both up at once, and so the two can
// never disagree about whether this build can embed a browser.
func (w *window) WebView() shell.WebView { return shell.WebViewOf(w.PlatformViews()) }
