package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func cmdDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	var o buildOpts
	var platName string
	var port int
	addBuildFlags(fs, &o, &platName)
	fs.IntVar(&port, "port", 8080, "web dev-server port")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if err := o.resolve(fs, platName); err != nil {
		return err
	}
	switch o.platform.name {
	case "web":
		return devWeb(o, port)
	case "ios", "android":
		return fmt.Errorf("dev hot reload isn't supported for %s; iterate on web/desktop and use `build` for device tests", o.platform.name)
	default:
		return devRestart(o)
	}
}

// devWeb rebuilds the wasm on every source change and live-reloads the browser
// over SSE — the fastest iteration loop (Flutter-web-like).
func devWeb(o buildOpts, port int) error {
	if _, err := buildWeb(o); err != nil {
		return err
	}
	b := newBroadcaster()
	changes, stop := watchSource(".", 250*time.Millisecond)
	defer stop()
	go func() {
		for range changes {
			fmt.Fprintln(os.Stderr, "gossamer: change detected — rebuilding wasm…")
			if _, err := buildWeb(o); err != nil {
				fmt.Fprintf(os.Stderr, "gossamer: build error:\n%v\n", err)
				continue // leave the last good build up; fix and save again
			}
			b.publish()
		}
	}()
	fmt.Fprintln(os.Stderr, "gossamer: web dev — edit & save to live-reload (Ctrl-C to stop)")
	return serve(outDir(o), port, b)
}

// devRestart rebuilds and relaunches the native binary on every change — hot
// restart. It's fast and works on every OS. UI state is preserved across the
// restart: the exiting process snapshots serializable state to a file (keyed by
// tree location) and the relaunched process restores it, so you land back at
// the same page/scroll/field. State a widget doesn't expose (see
// widget.Snapshottable) resets; a structural edit above a widget drops its
// state. The app opts in automatically — it just calls app.Run.
func devRestart(o buildOpts) error {
	changes, stop := watchSource(".", 250*time.Millisecond)
	defer stop()

	// Hand-off file for state-preserving restart. Clear any snapshot from a
	// previous session so this one starts fresh; the app reads/writes it via
	// the GOSSAMER_DEV_STATE env var.
	statePath, _ := filepath.Abs(filepath.Join("build", "dev-state.json"))
	_ = os.MkdirAll(filepath.Dir(statePath), 0o755)
	_ = os.Remove(statePath)
	stateEnv := "GOSSAMER_DEV_STATE=" + statePath

	var proc *exec.Cmd
	kill := func() {
		if proc == nil || proc.Process == nil {
			return
		}
		// SIGTERM asks the app to snapshot and exit; wait for it, but don't let
		// a wedged child freeze the loop — force-kill after a grace period.
		_ = proc.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- proc.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = proc.Process.Kill()
			<-done
		}
		proc = nil
	}
	launch := func() {
		bin, err := buildNative(o)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gossamer: build error:\n%v\n", err)
			return
		}
		proc = exec.Command(bin)
		proc.Env = append(append(os.Environ(), stateEnv), o.rendererEnv()...)
		proc.Stdout, proc.Stderr, proc.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := proc.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gossamer: launch error: %v\n", err)
			proc = nil
		}
	}

	// Clean up the child on Ctrl-C.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	fmt.Fprintln(os.Stderr, "gossamer: native dev — edit & save to rebuild+restart, state preserved (Ctrl-C to stop)")
	launch()
	for {
		select {
		case <-sig:
			kill()
			return nil
		case <-changes:
			fmt.Fprintln(os.Stderr, "gossamer: change detected — rebuild + restart")
			kill()
			launch()
		}
	}
}
