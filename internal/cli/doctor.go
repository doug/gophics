package cli

import (
	"fmt"
	"os"
	"runtime"
)

func cmdDoctor(_ []string) error {
	fmt.Print("gossamer doctor — toolchain check\n\n")
	ok := true
	ok = check("Go toolchain", have("go"), firstNonEmpty(goEnv("GOVERSION"), runtime.Version())) && ok
	ok = check("web (GOOS=js): wasm_exec.js", wasmExecJS() != "", altText(wasmExecJS(), "not found in GOROOT — is this Go ≥ 1.21?")) && ok
	ok = check("desktop/terminal", have("go"), "uses the host toolchain") && ok
	check("ios/android: gomobile", have("gomobile"), "optional — go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init")
	check("ios project: xcodegen", have("xcodegen"), "optional — brew install xcodegen")
	check("android: gradle", have("gradle"), "optional — install via Android Studio")
	hotOK := runtime.GOOS == "linux" || runtime.GOOS == "darwin"
	check("hot reload (--hot)", hotOK, altText("supported on "+runtime.GOOS, "plugin hot reload is linux/macOS only"))

	fmt.Println()
	if ok {
		fmt.Println("✓ ready to build desktop, web, and terminal targets.")
	} else {
		fmt.Println("✗ some required tools are missing (see above).")
	}
	return nil
}

func check(label string, ok bool, detail string) bool {
	mark := "✓"
	if !ok {
		mark = "✗"
	}
	if detail != "" {
		fmt.Fprintf(os.Stdout, "  %s %-32s %s\n", mark, label, detail)
	} else {
		fmt.Fprintf(os.Stdout, "  %s %s\n", mark, label)
	}
	return ok
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func altText(ok, bad string) string {
	if ok != "" {
		return ok
	}
	return bad
}
