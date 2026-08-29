package main

import (
	"math"
	"math/rand"
	"slices"
	"sync"
	"time"
)

// This file is the data side of the demo: a fixed-size window of request spans
// that a background goroutine keeps filling while the UI reads it. It is the
// example's argument for Go's concurrency story — the producer is an ordinary
// goroutine on an ordinary ticker, the UI never blocks on it, and the only
// coordination is one mutex held for the length of a copy.
//
// Spans come from one of two places, through the same door: the synthetic fleet
// below, or a real OpenTelemetry capture (see otlp.go).

const (
	// Window is how many spans the store keeps. Older ones fall out the back,
	// so memory is bounded no matter how long the demo runs: at 36 bytes a span
	// the whole window is about 3.6 MB, small enough to snapshot wholesale
	// several times a second.
	Window = 100_000

	// Rate is how many spans a second the synthetic fleet "serves".
	Rate = 1400
)

// Span is one served request. The service, route, and host are dictionary
// indices rather than strings (see dict.go), so a filter scan over the whole
// window never touches a string — the difference between a 100,000-row filter
// that is imperceptible and one that is not.
//
// Trace is the full 16-byte OpenTelemetry trace ID, kept whole rather than
// truncated to something table-sized: it is the one field you copy out of a
// viewer and paste into another tool, so a shortened one would be useless.
type Span struct {
	At    int32 // ms since the store's epoch
	Dur   int32 // microseconds
	Bytes int32 // response size; -1 when the capture didn't say
	Trace [16]byte
	Svc   uint16
	Route uint16
	Host  uint16
	Code  uint16 // HTTP status
}

// OK reports whether the request succeeded (2xx/3xx).
func (s Span) OK() bool { return s.Code < 400 }

// Failed reports whether the server itself failed (5xx) — the class that means
// the fleet is broken rather than the caller.
func (s Span) Failed() bool { return s.Code >= 500 }

// TraceHex renders the trace ID the way every tracing UI does.
func (s Span) TraceHex() string {
	const hex = "0123456789abcdef"
	out := make([]byte, 32)
	for i, b := range s.Trace {
		out[i*2], out[i*2+1] = hex[b>>4], hex[b&15]
	}
	return string(out)
}

// The synthetic fleet's vocabulary. It is interned up front so the producer
// goroutine only ever emits indices that already exist.
var (
	genServices = []string{"checkout", "catalog", "search", "identity", "payments", "inventory", "shipping", "reviews"}
	genRoutes   = []string{
		"GET /v1/items", "GET /v1/items/{id}", "POST /v1/cart", "POST /v1/checkout",
		"GET /v1/search", "POST /v1/auth/token", "GET /v1/me", "POST /v1/payments",
		"GET /v1/stock/{sku}", "PUT /v1/stock/{sku}", "GET /v1/shipments/{id}",
		"POST /v1/reviews", "GET /v1/reviews", "DELETE /v1/cart/{id}",
	}
	genRegions = []string{"iad", "ord", "sfo", "fra", "sin"}
)

// baseLatency is each generated route's healthy median, in microseconds.
// Checkout, search, and payments are the slow ones, which is what gives the
// latency histogram a shape instead of a single spike.
var baseLatency = [...]float64{
	1800, 2400, 5200, 42000, 68000, 9500, 2100, 51000,
	3300, 7400, 4100, 6800, 2900, 3600,
}

// Store is the rolling window. It is a ring: writes overwrite the oldest span,
// so the buffer is allocated once at startup and never grows.
type Store struct {
	mu    sync.Mutex
	buf   []Span
	n     uint64 // total spans ever written
	epoch time.Time

	// Source names where the data came from, for the header line.
	source string
	// paused stops the synthetic producer. Loading a capture sets it: a window
	// that mixed a fixed capture with a live generator would be neither, and
	// the generator's span indices refer to a vocabulary the load replaced.
	paused bool

	rng      *rand.Rand
	svcIDs   []uint16
	routeIDs []uint16
	hostIDs  []uint16
	degraded int     // the service currently having a bad time
	degrade  float64 // 0..1, how bad
}

func NewStore(seed int64) *Store {
	s := &Store{
		buf:      make([]Span, Window),
		epoch:    time.Now(),
		source:   "synthetic fleet",
		rng:      rand.New(rand.NewSource(seed)),
		degraded: 3,
		// Open mid-incident: a dashboard whose charts are all flat says nothing
		// about whether it would show you anything if they weren't.
		degrade: 0.5,
	}
	v := newVocab()
	s.registerVocabulary(v)
	useVocab(v)
	return s
}

// registerVocabulary interns every name the generator can emit, so that
// producing a span is pure arithmetic and the dictionaries are immutable while
// the producer runs.
func (s *Store) registerVocabulary(v *vocab) {
	for _, n := range genServices {
		s.svcIDs = append(s.svcIDs, v.svc.intern(n))
	}
	for _, n := range genRoutes {
		s.routeIDs = append(s.routeIDs, v.route.intern(n))
	}
	for _, r := range genRegions {
		for i := range 8 {
			s.hostIDs = append(s.hostIDs, v.host.intern(r+"-"+string(rune('a'+i))+string(rune('0'+i%4))))
		}
	}
}

// Source describes where the spans came from.
func (s *Store) Source() string { return s.source }

// Len is how many spans the window currently holds.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.len()
}

func (s *Store) len() int {
	if s.n < Window {
		return int(s.n)
	}
	return Window
}

// Snapshot copies the window, oldest first, into dst (reusing its capacity) and
// returns it along with the store's clock reading at the moment of the copy.
//
// Copying looks wasteful next to reading the ring in place under a read lock,
// and it is the point: the UI gets a slice nobody else will touch, so it can
// filter, sort, and re-read it across many frames while the producer keeps
// writing. The lock is held for one memmove of a few megabytes — tens of
// microseconds — instead of for the milliseconds a sort would take.
func (s *Store) Snapshot(dst []Span) ([]Span, int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.len()
	if cap(dst) < n {
		dst = make([]Span, n)
	}
	dst = dst[:n]
	if s.n < Window {
		copy(dst, s.buf[:n])
	} else {
		// The ring's oldest entry is wherever the next write will land.
		head := int(s.n % Window)
		copy(dst, s.buf[head:])
		copy(dst[Window-head:], s.buf[:head])
	}
	return dst, s.now()
}

func (s *Store) now() int32 { return int32(time.Since(s.epoch).Milliseconds()) }

// Now is the store's clock, in the same units as Span.At. The UI reads it every
// frame for the live Age column — which is the cheap half of "live": ages
// advance on every frame without the view being rebuilt at all.
func (s *Store) Now() int32 { return s.now() }

// Total is how many spans have ever been written, including those the ring has
// since evicted. Sampling it over time gives the observed ingest rate.
func (s *Store) Total() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

// Wall converts a span's timestamp to a wall clock time.
func (s *Store) Wall(at int32) time.Time { return s.epoch.Add(time.Duration(at) * time.Millisecond) }

// Replace swaps the window's contents for a decoded capture, keeping the newest
// Window spans and rebasing the clock so the newest span reads as "now" — a
// capture from last Tuesday should still show a sensible Age column.
//
// Spans are sorted by time first. An OTLP file is grouped by service, not
// ordered by clock, and the ring being chronological is not a cosmetic detail:
// it is the invariant the query engine's default view relies on to skip sorting
// entirely (see Run). Loading an unsorted capture without this shows the last
// service in the file as though it were the most recent traffic.
func (s *Store) Replace(spans []Span, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slices.SortFunc(spans, func(a, b Span) int { return int(a.At - b.At) })
	if len(spans) > Window {
		spans = spans[len(spans)-Window:]
	}
	newest := int32(0)
	if len(spans) > 0 {
		newest = spans[len(spans)-1].At
	}
	s.n = 0
	s.epoch = time.Now()
	for _, sp := range spans {
		sp.At -= newest // newest lands at 0, everything else is negative
		s.write(sp)
	}
	s.source = source
	s.paused = true
}

// Fill writes n spans immediately, spread backwards over the given duration —
// the cold start, so the demo opens on a full window and populated charts
// rather than filling in over a minute and a half.
//
// Traffic is warped rather than spread evenly: a slow swell plus a faster
// ripple, so the throughput chart opens with a shape instead of the dead flat
// line a uniform fill produces. The incident drifts through the backfill too,
// which is what puts one service's bar above the others from the first frame.
func (s *Store) Fill(n int, over time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	span := float64(over.Milliseconds())
	for i := range n {
		if i%180 == 0 {
			s.drift()
		}
		u := float64(i) / float64(n) // 0 = oldest, 1 = now
		// Warping u compresses spans into some stretches of the timeline and
		// thins them out in others, which is what a varying arrival rate looks
		// like after the fact. The amplitudes are kept small enough that dw/du
		// never approaches zero: a warp that briefly ran backwards would pile
		// thousands of spans onto one millisecond and spike the chart.
		w := u + 0.022*math.Sin(u*7.1) + 0.010*math.Sin(u*19.3+1.2)
		s.write(s.gen(int32(-span * (1 - w))))
	}
}

// Produce runs the fleet until stop is closed, writing a batch every tick. It is
// meant to be started with `go`.
func (s *Store) Produce(stop <-chan struct{}) {
	const tick = 25 * time.Millisecond
	t := time.NewTicker(tick)
	defer t.Stop()
	batch := int(Rate * tick.Seconds())
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.mu.Lock()
			if s.paused {
				s.mu.Unlock()
				continue
			}
			s.drift()
			now := s.now()
			for range batch {
				s.write(s.gen(now))
			}
			s.mu.Unlock()
		}
	}
}

// write appends one span to the ring. The caller holds the lock.
func (s *Store) write(sp Span) {
	s.buf[s.n%Window] = sp
	s.n++
}

// drift moves the incident around: the degraded service's severity wanders, and
// when it recovers another service is picked. Without it every chart settles
// into the same shape within seconds and the demo has nothing to show.
func (s *Store) drift() {
	s.degrade += (s.rng.Float64() - 0.47) * 0.02
	switch {
	case s.degrade > 1:
		s.degrade = 1
	case s.degrade < 0:
		s.degrade = 0
		s.degraded = s.rng.Intn(len(genServices))
	}
}

// gen synthesizes one plausible span at time `at`. Latency is lognormal around
// the route's median — the distribution real request latency actually follows,
// and the reason the histogram needs log-spaced buckets to read at all.
func (s *Store) gen(at int32) Span {
	route := s.rng.Intn(len(genRoutes))
	svc := route % len(genServices)
	if s.rng.Intn(4) == 0 { // routes aren't perfectly partitioned across services
		svc = s.rng.Intn(len(genServices))
	}

	sick := 0.0
	if svc == s.degraded {
		sick = s.degrade
	}
	// A lognormal draw: exp(normal) has the long right tail latency has.
	dur := baseLatency[route] * math.Exp(s.rng.NormFloat64()*0.62) * (1 + sick*6)
	if s.rng.Float64() < 0.004+sick*0.03 { // the tail: a retry, a cold cache
		dur *= 4 + s.rng.Float64()*8
	}

	code := uint16(200)
	switch r := s.rng.Float64(); {
	case r < 0.004+sick*0.14:
		code = [...]uint16{500, 502, 503}[s.rng.Intn(3)]
	case r < 0.02+sick*0.05:
		code = [...]uint16{400, 401, 404, 429}[s.rng.Intn(4)]
	case r < 0.08:
		code = [...]uint16{201, 204, 304}[s.rng.Intn(3)]
	}

	sp := Span{
		At:    at,
		Dur:   int32(dur),
		Bytes: int32(180 + s.rng.ExpFloat64()*4200),
		Svc:   s.svcIDs[svc],
		Route: s.routeIDs[route],
		Host:  s.hostIDs[s.rng.Intn(len(s.hostIDs))],
		Code:  code,
	}
	// A real trace ID is 16 random bytes; filling it from the same seeded source
	// keeps the whole generator reproducible.
	s.rng.Read(sp.Trace[:])
	return sp
}
