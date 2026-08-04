//go:build darwin

package codegen

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func verifyMSLWithXcrun(t *testing.T, source string) {
	t.Helper()

	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("xcrun not found; skipping MSL compile check")
	}
	if err := exec.Command("xcrun", "--find", "metal").Run(); err != nil {
		t.Skip("xcrun metal tool not found; skipping MSL compile check")
	}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "shader.metal")
	outPath := filepath.Join(dir, "shader.air")
	if err := os.WriteFile(srcPath, []byte(source), 0o600); err != nil {
		t.Fatalf("write MSL temp file: %v", err)
	}

	cmd := exec.Command("xcrun", "-sdk", "macosx", "metal", "-c", srcPath, "-o", outPath) //nolint:gosec // G204: args are temp paths in tests
	out, err := cmd.CombinedOutput()
	if err != nil {
		// The `metal` driver can be present (xcrun --find succeeds) while its
		// compiler component — the downloadable Metal Toolchain — is not
		// installed. That's an environment gap, not a codegen bug, so skip
		// rather than fail. A genuine MSL error (toolchain present) still fails.
		if bytes.Contains(out, []byte("missing Metal Toolchain")) ||
			bytes.Contains(out, []byte("cannot execute tool 'metal'")) {
			t.Skip("Metal Toolchain not installed (xcodebuild -downloadComponent MetalToolchain); skipping MSL compile check")
		}
		t.Fatalf("xcrun metal failed: %v\n%s\nMSL:\n%s", err, out, source)
	}
}
