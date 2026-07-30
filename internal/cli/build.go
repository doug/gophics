package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	var o buildOpts
	var platName string
	addBuildFlags(fs, &o, &platName)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.resolve(fs, platName); err != nil {
		return err
	}
	out, err := build(o)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gossamer: built %s → %s\n", o.platform.name, out)
	return nil
}

// build compiles the app for the platform and returns the output path: a
// binary for native/terminal, a directory for web, a bind artifact for mobile.
func build(o buildOpts) (string, error) {
	switch o.platform.name {
	case "web":
		return buildWeb(o)
	case "ios", "android":
		return buildMobile(o)
	default:
		return buildNative(o)
	}
}

func outDir(o buildOpts) string {
	if o.out != "" {
		return o.out
	}
	return filepath.Join("build", o.platform.name)
}

func buildNative(o buildOpts) (string, error) {
	dir := outDir(o)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "app")
	args := []string{"build", "-o", bin}
	if t := tagList(o.platform, o.gpu, o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	args = append(args, o.pkg)
	if err := run("", nil, "go", args...); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	return bin, nil
}

// buildWeb compiles the wasm, copies the toolchain-matched wasm_exec.js, and
// writes a default index.html if one isn't already present.
func buildWeb(o buildOpts) (string, error) {
	dir := outDir(o)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	args := []string{"build", "-o", filepath.Join(dir, "app.wasm")}
	if t := tagList(o.platform, o.gpu, o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	args = append(args, o.pkg)
	if err := run("", []string{"GOOS=js", "GOARCH=wasm"}, "go", args...); err != nil {
		return "", fmt.Errorf("go build (wasm): %w", err)
	}
	src := wasmExecJS()
	if src == "" {
		return "", fmt.Errorf("could not find wasm_exec.js in GOROOT (%s)", goEnv("GOROOT"))
	}
	if err := copyFile(src, filepath.Join(dir, "wasm_exec.js")); err != nil {
		return "", fmt.Errorf("copy wasm_exec.js: %w", err)
	}
	idx := filepath.Join(dir, "index.html")
	if _, err := os.Stat(idx); os.IsNotExist(err) {
		if err := os.WriteFile(idx, []byte(indexHTML), 0o644); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// indexHTML is the default web harness. Users may edit build/web/index.html;
// buildWeb won't overwrite an existing one.
const indexHTML = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no">
  <title>gossamer app</title>
  <style>html,body{margin:0;height:100%;background:#fff}</style>
  <script src="wasm_exec.js"></script>
  <script>
    const go = new Go();
    WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject)
      .then(r => go.run(r.instance));
  </script>
</head>
<body></body>
</html>
`
