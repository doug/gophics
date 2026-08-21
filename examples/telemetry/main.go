// Command telemetry is a live request explorer over a rolling window of 100,000
// spans — the example for data at a scale that breaks naive UI code.
//
// A background goroutine serves a synthetic fleet at ~1,400 requests a second
// into a fixed-size ring; the dashboard snapshots that window several times a
// second, re-filters and re-sorts all 100,000 rows, and redraws stat tiles, three
// live charts, and a virtualized table from the same single pass. Filter by
// service, route, host, or trace prefix; sort on any column; watch the Age
// column tick every frame. Nothing is precomputed and nothing is sampled.
//
// Three things it is built to show:
//
//   - Virtualization. widget.LazyList mounts only the rows on screen, so the
//     table costs the same at a hundred rows and a hundred thousand — and a
//     column recomputed from the clock on every frame costs nothing.
//
//   - Concurrency. The producer is an ordinary goroutine. The only coordination
//     is a mutex held for the length of one copy, so the UI never waits on it.
//
//   - Honest numbers. The dashboard shows how long its own rebuild took, so the
//     claim is on screen rather than in this comment.
//
//     go run ./examples/telemetry
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
)

func main() {
	otlp := flag.String("otlp", "", "load an OpenTelemetry OTLP/JSON trace export instead of the synthetic fleet")
	flag.Parse()

	store := NewStore(9)
	if *otlp != "" {
		if err := load(store, *otlp); err != nil {
			log.Fatal(err)
		}
	} else {
		// Open on a full window rather than filling one over the minute-plus it
		// would take in real time: the charts have a minute of history at once.
		store.Fill(Window, time.Duration(Window)*time.Second/Rate)
		stop := make(chan struct{})
		defer close(stop)
		go store.Produce(stop)
	}

	if err := app.Run(App{Store: store}, app.Config{
		Title: "Fleet",
		AppID: "com.gophics.telemetry",
		Size:  geom.Size{W: 1360, H: 860},
		Font:  goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
			"mono":         gomono.TTF,
		},
	}); err != nil {
		log.Fatal(err)
	}
}

// load reads a capture off disk. Anything an OTLP/JSON exporter writes works;
// see otlp.go for what is read out of it.
func load(store *Store, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	v := newVocab()
	spans, err := DecodeOTLP(f, v)
	if err != nil {
		return err
	}
	useVocab(v)
	store.Replace(spans, filepath.Base(path))
	log.Printf("loaded %d spans from %s", len(spans), path)
	return nil
}
