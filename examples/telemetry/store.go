package main

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// This file is the data side of the demo: a fixed-size window of request spans
// that a background goroutine keeps filling while the UI reads it. It is the
// example's argument for Go's concurrency story — the producer is an ordinary
// goroutine on an ordinary ticker, the UI never blocks on it, and the only
// coordination is one mutex held for the length of a copy.

const (
	// Window is how many spans the store keeps. Older ones fall out the back,
	// so memory is bounded no matter how long the demo runs: at 24 bytes a span
	// the whole window is about 2.4 MB, small enough to snapshot wholesale
	// several times a second.
	Window = 100_000

	// Rate is how many spans a second the fleet "serves".
	Rate = 1400
)

// Span is one served request. Every field is a number: the service, route, and
// host names live in dictionaries (see below) and are referenced by index, so a
// span is 24 bytes and a filter scan over the whole window never touches a
// string. That is the difference between a 100,000-row filter that is
// imperceptible and one that is not.
type Span struct {
	At    int32  // ms since the store's epoch
	Dur   int32  // microseconds
	Bytes int32  // response size
	Trace uint32 // rendered as hex; not otherwise interpreted
	Host  uint16
	Code  uint16 // HTTP status
	Svc   uint8
	Route uint8
}

// OK reports whether the request succeeded (2xx/3xx).
func (s Span) OK() bool { return s.Code < 400 }

// Failed reports whether the server itself failed (5xx) — the class that means
// the fleet is broken rather than the caller.
func (s Span) Failed() bool { return s.Code >= 500 }

var (
	// Fixed-size arrays, not slices, so len() is a constant the aggregate
	// buffers in query.go can be sized by.
	services = [...]string{"checkout", "catalog", "search", "identity", "payments", "inventory", "shipping", "reviews"}
	routes   = [...]string{
		"GET /v1/items", "GET /v1/items/{id}", "POST /v1/cart", "POST /v1/checkout",
		"GET /v1/search", "POST /v1/auth/token", "GET /v1/me", "POST /v1/payments",
		"GET /v1/stock/{sku}", "PUT /v1/stock/{sku}", "GET /v1/shipments/{id}",
		"POST /v1/reviews", "GET /v1/reviews", "DELETE /v1/cart/{id}",
	}
	regions = []string{"iad", "ord", "sfo", "fra", "sin"}
)

// hosts is a dictionary of instance names ("ord-4f"), built once.
var hosts = buildHosts()

func buildHosts() []string {
	out := make([]string, 0, len(regions)*8)
	for _, r := range regions {
		for i := 0; i < 8; i++ {
			out = append(out, r+"-"+string(rune('a'+i))+string(rune('0'+i%4)))
		}
	}
	return out
}

// baseLatency is each route's healthy median, in microseconds. Search and
// checkout are the slow ones, which is what gives the latency histogram a shape
// instead of a single spike.
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

	rng      *rand.Rand
	degraded int     // the service currently having a bad time
	degrade  float64 // 0..1, how bad
}

func NewStore(seed int64) *Store {
	return &Store{
		buf:      make([]Span, Window),
		epoch:    time.Now(),
		rng:      rand.New(rand.NewSource(seed)),
		degraded: 3,
	}
}

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
// writing. The lock is held for one memmove of about 2.4 MB — tens of
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

// Fill writes n spans immediately, spread backwards over the given duration —
// the cold start, so the demo opens on a full window and populated charts
// rather than filling in over a minute and a half.
func (s *Store) Fill(n int, over time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := -float64(over.Milliseconds())
	for i := 0; i < n; i++ {
		at := int32(start * (1 - float64(i)/float64(n)))
		s.write(s.gen(at))
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
			s.drift()
			now := s.now()
			for i := 0; i < batch; i++ {
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
		s.degraded = s.rng.Intn(len(services))
	}
}

// gen synthesizes one plausible span at time `at`. Latency is lognormal around
// the route's median — the distribution real request latency actually follows,
// and the reason the histogram needs log-spaced buckets to read at all.
func (s *Store) gen(at int32) Span {
	route := s.rng.Intn(len(routes))
	svc := route % len(services)
	if s.rng.Intn(4) == 0 { // routes aren't perfectly partitioned across services
		svc = s.rng.Intn(len(services))
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
		code = []uint16{500, 502, 503}[s.rng.Intn(3)]
	case r < 0.02+sick*0.05:
		code = []uint16{400, 401, 404, 429}[s.rng.Intn(4)]
	case r < 0.08:
		code = []uint16{201, 204, 304}[s.rng.Intn(3)]
	}

	return Span{
		At:    at,
		Dur:   int32(dur),
		Bytes: int32(180 + s.rng.ExpFloat64()*4200),
		Trace: s.rng.Uint32(),
		Host:  uint16(s.rng.Intn(len(hosts))),
		Code:  code,
		Svc:   uint8(svc),
		Route: uint8(route),
	}
}
