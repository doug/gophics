package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// run executes name+args with stdio streamed to the terminal, in dir (empty =
// current), with extra environment entries appended to the parent's.
func run(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

// goEnv returns `go env <name>`, or "" on error.
func goEnv(name string) string {
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// have reports whether an executable is on PATH.
func have(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// goList returns `go list -f <format> <pkg>`.
func goList(format, pkg string) (string, error) {
	out, err := exec.Command("go", "list", "-f", format, pkg).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// writeFileAtomic writes data to path via a temp file + rename, so a concurrent
// reader never sees a half-written file.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// wasmExecJS returns the path to the active toolchain's wasm_exec.js, so the
// copy served to the browser always matches the compiler (a version mismatch
// silently breaks the wasm). Go 1.24+ keeps it under lib/wasm; older releases
// under misc/wasm.
func wasmExecJS() string {
	root := goEnv("GOROOT")
	if root == "" {
		return ""
	}
	for _, p := range []string{
		filepath.Join(root, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(root, "misc", "wasm", "wasm_exec.js"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
