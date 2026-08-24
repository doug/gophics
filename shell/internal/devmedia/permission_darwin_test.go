//go:build darwin && !ios && !js

package devmedia

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// The query itself is not tested, and cannot usefully be: on a machine where
// access is granted, a function that asks the platform and one that returns
// Granted unconditionally give the same answer. That is precisely how the
// microphone came to claim Granted without ever asking. What is testable is
// the mapping, which is where a wrong answer would actually come from.
func TestPermissionFor(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int64
		want   shell.Permission
	}{
		{"authorized", avAuthAuthorized, shell.PermissionGranted},
		{"denied", avAuthDenied, shell.PermissionDenied},
		{"restricted", avAuthRestricted, shell.PermissionDenied},
		{"not determined", 0, shell.PermissionPrompt},
		{"unknown future value", 99, shell.PermissionPrompt},
	} {
		if got := permissionFor(c.status); got != c.want {
			t.Errorf("permissionFor(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestMicPermissionAgreesWithTheMapping is the most this can check about the
// live query without a second machine: whatever the platform says, the
// capability must report the mapped answer and not a constant.
func TestMicPermissionAgreesWithTheMapping(t *testing.T) {
	got := micPermission()
	switch got {
	case shell.PermissionGranted, shell.PermissionDenied, shell.PermissionPrompt:
	default:
		t.Fatalf("micPermission() = %v, which is not a Permission", got)
	}
	t.Logf("microphone permission on this machine: %v", got)
}
