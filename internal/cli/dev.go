package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func cmdDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	var o buildOpts
	var platName string
	var port int
	var hot bool
	addBuildFlags(fs, &o, &platName)
	fs.IntVar(&port, "port", 8080, "web dev-server port")
	fs.BoolVar(&hot, "hot", false, "(retired) accepted for compatibility; falls back to hot-restart — see note")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if err := o.resolve(fs, platName); err != nil {
		return err
	}
	if hot {
		// Retired: Go's plugin loader refuses to load a second plugin that
		// contains a different version of an already-loaded package
		// ("plugin was built with a different version of package X"), which is
		// exactly what an edited UI package is. There is no plugin-based path to
		// state-preserving in-place reload. Steer to the loops that do work.
		fmt.Fprintln(os.Stderr, "gossamer: --hot (plugin reload) is retired — Go plugins can't reload an edited package in place.")
		fmt.Fprintln(os.Stderr, "gossamer: using desktop hot-restart instead. For the fastest loop use `gossamer dev -p web`.")
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
// restart (state is lost, but it's fast and works on every OS).
func devRestart(o buildOpts) error {
	changes, stop := watchSource(".", 250*time.Millisecond)
	defer stop()

	var proc *exec.Cmd
	kill := func() {
		if proc != nil && proc.Process != nil {
			_ = proc.Process.Signal(syscall.SIGTERM)
			_ = proc.Wait()
			proc = nil
		}
	}
	launch := func() {
		bin, err := buildNative(o)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gossamer: build error:\n%v\n", err)
			return
		}
		proc = exec.Command(bin)
		proc.Stdout, proc.Stderr, proc.Stdin = os.Stdout, os.Stderr, os.Stdin
		if err := proc.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gossamer: launch error: %v\n", err)
			proc = nil
		}
	}

	// Clean up the child on Ctrl-C.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	fmt.Fprintln(os.Stderr, "gossamer: native dev — edit & save to rebuild+restart (Ctrl-C to stop)")
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
