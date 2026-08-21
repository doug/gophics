package main

import "sync"

// dict interns names so a Span can refer to a service, route, or host by a
// two-byte index instead of carrying a string. That is what keeps a span at 36
// bytes and, more importantly, what lets the text filter be resolved against a
// few dozen names once per query instead of against a hundred thousand rows.
//
// Dictionaries are built during setup — the synthetic fleet registers its whole
// vocabulary up front, and an OTLP load interns as it decodes — and are only
// read afterwards. The producer goroutine never interns, so the UI can read
// names without a lock. Add returns an error rather than growing past the index
// width so that invariant can't be broken quietly by a pathological input.
type dict struct {
	mu    sync.Mutex
	names []string
	idx   map[string]int
}

const maxDictEntries = 1 << 16

// intern returns name's index, adding it if new. Names past the index width
// collapse onto the last entry rather than corrupting the mapping; a capture
// with 65,536 distinct routes is a broken capture, not a use case.
func (d *dict) intern(name string) uint16 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx == nil {
		d.idx = make(map[string]int)
	}
	if i, ok := d.idx[name]; ok {
		return uint16(i)
	}
	if len(d.names) >= maxDictEntries {
		return uint16(len(d.names) - 1)
	}
	d.idx[name] = len(d.names)
	d.names = append(d.names, name)
	return uint16(len(d.names) - 1)
}

// Name returns the i-th name, or "?" if the index is out of range.
func (d *dict) Name(i uint16) string {
	if int(i) >= len(d.names) {
		return "?"
	}
	return d.names[i]
}

// Names is the whole vocabulary, in insertion order.
func (d *dict) Names() []string { return d.names }

// Len is how many distinct names have been interned.
func (d *dict) Len() int { return len(d.names) }

// vocab is the set of dictionaries one dataset is described by. A span's
// indices are only meaningful against the vocabulary it was created with, so
// loading a capture builds a fresh one and swaps it in wholesale — decoding into
// the live dictionaries instead would leave the previous dataset's services
// listed in the filter and its routes matchable, and a decode that failed
// halfway would leave a vocabulary that describes neither.
type vocab struct{ svc, route, host *dict }

func newVocab() *vocab { return &vocab{svc: &dict{}, route: &dict{}, host: &dict{}} }

// The vocabulary the UI and the query engine read. It is replaced only from the
// UI goroutine, and only while the producer is stopped (see Store.Replace).
var (
	svcDict   *dict
	routeDict *dict
	hostDict  *dict
)

// useVocab installs v as the active vocabulary.
func useVocab(v *vocab) { svcDict, routeDict, hostDict = v.svc, v.route, v.host }

func init() { useVocab(newVocab()) }
