package cli

import (
	"fmt"
	"os"
	"runtime"
)

func cmdDoctor(_ []string) error {
	fmt.Print("gophics doctor — toolchain check\n\n")
	ok := true
	ok = check("Go toolchain", have("go"), firstNonEmpty(goEnv("GOVERSION"), runtime.Version())) && ok
	ok = check("web (GOOS=js): wasm_exec.js", wasmExecJS() != "", altText(wasmExecJS(), "not found in GOROOT — is this Go ≥ 1.21?")) && ok
	ok = check("desktop/terminal", have("go"), "uses the host toolchain") && ok
	gomobileOK := check("ios/android: gomobile", have("gomobile"), "optional — go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init")
	// `gophics run -p ios` builds the host with xcodegen; -p android uses the
	// scaffolded Gradle wrapper (a system gradle is not required).
	xcodegenOK := check("ios project: xcodegen", have("xcodegen"), "optional — brew install xcodegen")
	check("android: gradle", have("gradle"), "optional — the scaffolded ./gradlew is used if present")

	fmt.Println()
	if ok {
		fmt.Println("✓ ready to build desktop, web, and terminal targets.")
		switch {
		case gomobileOK && xcodegenOK:
			fmt.Println("✓ ready for ios + android (gomobile + xcodegen present).")
		case gomobileOK:
			fmt.Println("✓ ready for android; for ios also install xcodegen.")
		default:
			fmt.Println("• for ios/android, install gomobile (and xcodegen for ios).")
		}
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
