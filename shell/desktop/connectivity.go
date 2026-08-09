//go:build !js

// Desktop implementation of the shell connectivity capability
// (shell/connectivity.go).
//
// This is an honest best-effort, not a fake: desktop always reports
// Online()==true. gophics is zero-CGo, and fine-grained desktop reachability
// needs OS APIs (macOS SCNetworkReachability, Linux NetworkManager over D-Bus,
// Windows INetworkListManager) we don't yet call, so we cannot observe link
// changes. OnChange therefore registers no callback (a no-op subscription)
// rather than inventing events. When those OS reachability sources are wired,
// Online() gains real link state and OnChange begins delivering flips.

package desktop

import "github.com/doug/gophics/shell"

// Connectivity makes the desktop window a shell.ConnectivityWindow.
func (w *window) Connectivity() shell.Connectivity { return desktopConnectivity{} }

type desktopConnectivity struct{}

// Online always reports true on desktop today (see the package note): a desktop
// app is overwhelmingly run with a network present, so assuming online is safer
// than falsely gating features offline.
func (desktopConnectivity) Online() bool { return true }

// OnChange is a no-op subscription: without OS reachability APIs desktop cannot
// observe link changes, so f is never called (never a fake event).
func (desktopConnectivity) OnChange(func(bool)) {}
