//go:build !nogpu

package gpu

import (
	"os"
	"testing"
)

// requireGPU converts a missing-adapter skip into a failure when the
// environment says one is guaranteed.
//
// Every GPU test here skips when no adapter is available, which is right on a
// developer machine and dangerous in CI: a job that installs a driver and then
// silently skips reports success for having tested nothing. That is not
// hypothetical for this package — the compute pipeline failed to build on Metal
// for months while its tests skipped, and each silence hid the next problem.
//
// Set GOPHICS_REQUIRE_GPU=1 where an adapter is guaranteed. The message names
// the original error so a genuine driver problem is still diagnosable.
func requireGPU(tb testing.TB, err error, what string) {
	tb.Helper()
	if os.Getenv("GOPHICS_REQUIRE_GPU") != "" {
		tb.Fatalf("%s: %v (GOPHICS_REQUIRE_GPU is set, so this is a failure rather than a skip)", what, err)
	}
	tb.Skipf("%s: %v", what, err)
}
