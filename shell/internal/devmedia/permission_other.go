//go:build !js && !(darwin && !ios)

package devmedia

import "github.com/doug/gophics/shell"

// Microphone permission off macOS.
//
// Neither Linux nor Windows has a per-application microphone permission that
// can be queried: both gate at the OS settings level, and a refusal surfaces
// as a device that will not open. Granted is the honest answer to "may you",
// and Listen is what discovers the rest.
func micPermission() shell.Permission { return shell.PermissionGranted }
