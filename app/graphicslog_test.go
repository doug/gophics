package app

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
)

// restoreGraphicsLog puts the stack back to silent after a test, so one test's
// logger does not leak into another's output.
func restoreGraphicsLog(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { gg.SetLogger(nil) })
}

// A logger on the Config has to actually reach the rendering stack. gg defaults
// to a handler that discards every record, so the whole GPU stack — adapter
// selection, pipeline compilation, and the warnings it emits when it skips work
// — is silent unless something wires this up. Nothing did, which is why a blank
// text bug on Android had to be diagnosed by adding the wiring by hand first.
func TestConfigGraphicsLogReachesTheStack(t *testing.T) {
	restoreGraphicsLog(t)

	var buf bytes.Buffer
	cfg := Config{
		GraphicsLog: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}
	if _, err := newCore(nil, cfg); err != nil {
		t.Fatalf("newCore: %v", err)
	}

	// Emit through gg's own logger, which is what every layer below it shares.
	gg.Logger().Info("probe", "from", "test")

	if !strings.Contains(buf.String(), "probe") {
		t.Errorf("a record logged through gg did not reach Config.GraphicsLog; "+
			"got %q", buf.String())
	}
}

// The default must stay silent: starting an app must not turn logging on by
// itself. A UI library that writes to a program's output uninvited is worse
// than one that is hard to debug.
func TestGraphicsLogSilentByDefault(t *testing.T) {
	restoreGraphicsLog(t)
	gg.SetLogger(nil) // the state a fresh process is in
	t.Setenv("GOPHICS_GPU_LOG", "")

	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	if _, err := newCore(nil, Config{}); err != nil {
		t.Fatalf("newCore: %v", err)
	}
	gg.Logger().Error("must not appear")

	if buf.Len() != 0 {
		t.Errorf("starting an app enabled graphics logging on its own: %q", buf.String())
	}
}

// A logger someone set directly must survive an app starting. Clearing it would
// mean the second app in a process silently switches off the first's
// diagnostics, and a test that configures logging before building a core would
// lose it.
func TestGraphicsLogDoesNotClobberAnExplicitLogger(t *testing.T) {
	restoreGraphicsLog(t)
	t.Setenv("GOPHICS_GPU_LOG", "")

	var buf bytes.Buffer
	gg.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	if _, err := newCore(nil, Config{}); err != nil {
		t.Fatalf("newCore: %v", err)
	}
	gg.Logger().Info("probe")

	if !strings.Contains(buf.String(), "probe") {
		t.Errorf("starting an app cleared a logger that was set directly; got %q", buf.String())
	}
}

// GOPHICS_GPU_LOG is the way in on platforms where setting a field is awkward —
// a gomobile bind surface is started by the host, not by main.
func TestGraphicsLogFromEnv(t *testing.T) {
	restoreGraphicsLog(t)

	for _, tc := range []struct {
		env   string
		emit  func()
		want  bool
		label string
	}{
		{"debug", func() { gg.Logger().Debug("dbg") }, true, "debug passes debug"},
		{"warn", func() { gg.Logger().Debug("dbg") }, false, "warn filters debug"},
		{"warn", func() { gg.Logger().Warn("dbg") }, true, "warn passes warn"},
		{"", func() { gg.Logger().Error("dbg") }, false, "unset stays silent"},
		{"nonsense", func() { gg.Logger().Error("dbg") }, false, "unrecognised stays silent"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			restoreGraphicsLog(t)
			t.Setenv("GOPHICS_GPU_LOG", tc.env)

			// The env path routes through the standard logger, which is what
			// reaches logcat on Android; capture that rather than stderr.
			var buf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(prev) })

			if _, err := newCore(nil, Config{}); err != nil {
				t.Fatalf("newCore: %v", err)
			}
			tc.emit()

			if got := strings.Contains(buf.String(), "dbg"); got != tc.want {
				t.Errorf("GOPHICS_GPU_LOG=%q: logged=%v want=%v (output %q)",
					tc.env, got, tc.want, buf.String())
			}
		})
	}
}

// An explicit logger beats the environment, or a test or embedding cannot get
// deterministic output on a machine that happens to export the variable.
func TestConfigGraphicsLogBeatsEnv(t *testing.T) {
	restoreGraphicsLog(t)
	t.Setenv("GOPHICS_GPU_LOG", "debug")

	var stdlog, explicit bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&stdlog)
	t.Cleanup(func() { log.SetOutput(prev) })

	cfg := Config{GraphicsLog: slog.New(slog.NewTextHandler(&explicit, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))}
	if _, err := newCore(nil, cfg); err != nil {
		t.Fatalf("newCore: %v", err)
	}
	gg.Logger().Info("probe")

	if !strings.Contains(explicit.String(), "probe") {
		t.Error("the explicit logger did not receive the record")
	}
	if strings.Contains(stdlog.String(), "probe") {
		t.Error("the record also went to the environment's logger; the explicit " +
			"one must win outright")
	}
}
