package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// broadcaster fans a reload signal out to every connected browser (each dev
// tab is one subscriber).
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan struct{}]struct{})}
}

func (b *broadcaster) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *broadcaster) publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default: // a reload is already pending for this client
		}
	}
}

func sseHandler(b *broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch, unsub := b.subscribe()
		defer unsub()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				fmt.Fprint(w, "data: reload\n\n")
				fl.Flush()
			}
		}
	}
}

// reloadClient reconnects and reloads the page on each event; EventSource
// auto-reconnects, so the tab recovers across dev-server restarts too.
const reloadClient = `<script>new EventSource("/_gossamer/reload").onmessage=function(){location.reload()};</script>`

// serveIndexWithReload serves dir/index.html with the reload client injected
// just before </body> (or appended if there's no body tag).
func serveIndexWithReload(w http.ResponseWriter, dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	html := string(b)
	if i := strings.LastIndex(html, "</body>"); i >= 0 {
		html = html[:i] + reloadClient + html[i:]
	} else {
		html += reloadClient
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}
