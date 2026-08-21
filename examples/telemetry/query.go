package main

import (
	"bytes"
	"math/bits"
	"slices"
	"strings"
	"time"
)

// This file is the query engine: one pass over the snapshot that filters,
// aggregates, and (when the sort isn't chronological) sorts. It runs a few times
// a second on the UI goroutine, between frames, over the whole 100,000-span
// window — so everything here is written to stay well inside a frame budget.

// Sort columns. They are the table's column indices, so a header tap maps
// straight through.
const (
	colTime = iota
	colAge
	colService
	colRoute
	colHost
	colTrace
	colStatus
	colLatency
	colBytes
	numCols
)

// Status filter classes.
const (
	statusAny = iota
	statusOK
	statusClient
	statusServer
)

// Query is everything the filter bar and the table header contribute.
type Query struct {
	Text    string
	Status  int
	Svc     int // -1 for every service
	SortCol int
	Desc    bool
}

// Buckets for the latency histogram the UI draws, in microseconds. The scale is
// logarithmic because request latency is: a linear histogram of these spans is a
// single bar at the left and eleven empty ones.
var histEdges = [...]int32{1e3, 2e3, 5e3, 1e4, 2e4, 5e4, 1e5, 2e5, 5e5, 1e6, 2e6}

var histLabels = [...]string{"<1ms", "2", "5", "10", "20", "50", "100", "200", "500", "1s", "2s", "2s+"}

// ThroughputSecs is how many seconds of history the throughput chart shows.
const ThroughputSecs = 60

// Result is one query's output: the row order to display, plus every number the
// dashboard shows. They come from the same pass, because a second pass over
// 100,000 rows to compute the charts would cost as much as the first.
type Result struct {
	Rows []Span  // the snapshot the view indexes into
	View []int32 // matching rows, in display order
	Now  int32

	Matching  int
	Client    int // 4xx
	Server    int // 5xx
	P50       int32
	P95       int32
	P99       int32
	Hist      [len(histLabels)]int32
	PerSec    [ThroughputSecs]int32
	SvcP95    []int32 // per service, indexed by dictionary id
	SvcCount  []int32
	Elapsed   time.Duration
	SortedRow bool // whether this rebuild had to sort (see Run)
}

// Run filters, aggregates, and orders the snapshot.
func Run(rows []Span, now int32, q Query) Result {
	start := time.Now()
	res := Result{Rows: rows, Now: now}

	// The text filter is resolved against the dictionaries once, up front, into
	// a bitset per dictionary. After this the 100,000-row scan is integer
	// lookups — it never touches a string, which is what keeps a filter over the
	// whole window off the frame budget.
	f := compile(q.Text)

	nsvc := svcDict.Len()
	res.SvcP95 = make([]int32, nsvc)
	res.SvcCount = make([]int32, nsvc)

	view := make([]int32, 0, len(rows))
	var fine [200]int32
	svcFine := make([][200]int32, nsvc)

	for i := range rows {
		sp := rows[i]
		if q.Svc >= 0 && int(sp.Svc) != q.Svc {
			continue
		}
		switch q.Status {
		case statusOK:
			if !sp.OK() {
				continue
			}
		case statusClient:
			if sp.Code < 400 || sp.Code >= 500 {
				continue
			}
		case statusServer:
			if !sp.Failed() {
				continue
			}
		}
		if f.on && !f.match(sp) {
			continue
		}

		view = append(view, int32(i))
		b := latBucket(sp.Dur)
		fine[b]++
		svcFine[sp.Svc][b]++
		res.SvcCount[sp.Svc]++
		switch {
		case sp.Failed():
			res.Server++
		case sp.Code >= 400:
			res.Client++
		}
		if age := now - sp.At; age >= 0 && age < ThroughputSecs*1000 {
			res.PerSec[ThroughputSecs-1-age/1000]++
		}
	}

	res.View = view
	res.Matching = len(view)
	res.P50 = percentile(&fine, res.Matching, 0.50)
	res.P95 = percentile(&fine, res.Matching, 0.95)
	res.P99 = percentile(&fine, res.Matching, 0.99)
	for i := range svcFine {
		res.SvcP95[i] = percentile(&svcFine[i], int(res.SvcCount[i]), 0.95)
	}
	for b, n := range fine {
		if n > 0 {
			res.Hist[histBucket(bucketUs(b))] += n
		}
	}

	res.SortedRow = sortView(view, rows, q)
	if q.SortCol == colTime || q.SortCol == colAge {
		// Chronological order is the order the ring already holds, so the
		// default view — newest first — needs no sort at all, only a reverse.
		// It is worth the special case: it is the order the table opens in and
		// the one a live tail sits in, so the common path stays free.
		if (q.SortCol == colTime) == q.Desc {
			slices.Reverse(view)
		}
	}

	res.Elapsed = time.Since(start)
	return res
}

// sortView orders the view for any non-chronological column, and reports
// whether it actually had to sort.
func sortView(view []int32, rows []Span, q Query) bool {
	if q.SortCol == colTime || q.SortCol == colAge {
		return false
	}
	cmp := func(a, b int32) int {
		x, y := rows[a], rows[b]
		var d int
		switch q.SortCol {
		case colService:
			d = strings.Compare(svcDict.Name(x.Svc), svcDict.Name(y.Svc))
		case colRoute:
			d = strings.Compare(routeDict.Name(x.Route), routeDict.Name(y.Route))
		case colHost:
			d = strings.Compare(hostDict.Name(x.Host), hostDict.Name(y.Host))
		case colTrace:
			d = bytes.Compare(x.Trace[:], y.Trace[:])
		case colStatus:
			d = int(x.Code) - int(y.Code)
		case colLatency:
			d = int(x.Dur - y.Dur)
		case colBytes:
			d = int(x.Bytes - y.Bytes)
		}
		if d == 0 { // ties break by time, so the order is total and stable
			d = int(x.At - y.At)
		}
		if q.Desc {
			return -d
		}
		return d
	}
	slices.SortFunc(view, cmp)
	return true
}

// filter is a compiled search: one bitset per dictionary plus an optional trace
// prefix. Matching a hex prefix is done on the raw bytes — the query's nibbles
// against the ID's leading ones — so no span is ever formatted to be searched.
type filter struct {
	on    bool
	svc   []bool
	route []bool
	host  []bool

	tracePfx  [16]byte
	traceNibs int // how many leading hex digits of the query to honour
}

// match tests one span. It is the innermost thing in the scan, so it is nothing
// but slice indexing and, at most, a 16-byte compare.
func (f *filter) match(sp Span) bool {
	if f.svc[sp.Svc] || f.route[sp.Route] || f.host[sp.Host] {
		return true
	}
	if f.traceNibs == 0 {
		return false
	}
	whole := f.traceNibs / 2
	if !bytes.Equal(sp.Trace[:whole], f.tracePfx[:whole]) {
		return false
	}
	if f.traceNibs%2 == 1 { // an odd query length ends on a half byte
		return sp.Trace[whole]>>4 == f.tracePfx[whole]>>4
	}
	return true
}

// compile resolves the search text against the dictionaries once, up front.
// After this the 100,000-row scan is integer lookups — it never touches a
// string, which is what keeps a filter over the whole window off the frame
// budget however fast someone types.
func compile(text string) filter {
	f := filter{
		svc:   make([]bool, svcDict.Len()),
		route: make([]bool, routeDict.Len()),
		host:  make([]bool, hostDict.Len()),
	}
	q := strings.ToLower(strings.TrimSpace(text))
	if q == "" {
		return f
	}
	f.on = true
	for i, n := range svcDict.Names() {
		f.svc[i] = strings.Contains(strings.ToLower(n), q)
	}
	for i, n := range routeDict.Names() {
		f.route[i] = strings.Contains(strings.ToLower(n), q)
	}
	for i, n := range hostDict.Names() {
		f.host[i] = strings.Contains(strings.ToLower(n), q)
	}
	if len(q) <= 32 {
		if nibs, ok := parseNibbles(q, f.tracePfx[:]); ok {
			f.traceNibs = nibs
		}
	}
	return f
}

// parseNibbles writes q's hex digits into dst, high nibble first, and reports
// how many it wrote. It fails on any non-hex character, which is how a search
// for "search" is treated as a name and one for "4afb" as a trace prefix.
func parseNibbles(q string, dst []byte) (int, bool) {
	for i := 0; i < len(q); i++ {
		var v byte
		switch c := q[i]; {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		default:
			return 0, false
		}
		if i%2 == 0 {
			dst[i/2] = v << 4
		} else {
			dst[i/2] |= v
		}
	}
	return len(q), true
}

// latBucket maps microseconds to one of ~200 log-spaced buckets with integer
// operations only: the bucket's octave is the position of the leading set bit,
// and the three bits under it split that octave into eight — about 9%
// resolution, which is finer than a percentile off a live sample deserves and
// costs a shift and a mask instead of a call to math.Log.
func latBucket(us int32) int {
	u := uint32(us)
	if u < 16 {
		return int(u)
	}
	e := bits.Len32(u) // 5..32
	return 16 + (e-5)*8 + int((u>>(e-4))&7)
}

// bucketUs is latBucket's inverse: the middle of the range a bucket covers.
func bucketUs(b int) int32 {
	if b < 16 {
		return int32(b)
	}
	b -= 16
	shift := b/8 + 1
	sub := uint32(b % 8)
	return int32(((8+sub)<<shift + (9+sub)<<shift) / 2)
}

// percentile reads a quantile straight off the histogram — O(buckets), not
// O(n log n), and no copy of the matching durations to sort.
func percentile(h *[200]int32, n int, p float64) int32 {
	if n == 0 {
		return 0
	}
	want := int32(float64(n) * p)
	var cum int32
	for b, c := range h {
		if cum += c; cum > want {
			return bucketUs(b)
		}
	}
	return bucketUs(len(h) - 1)
}

// histBucket places a duration in one of the twelve display buckets.
func histBucket(us int32) int {
	for i, e := range histEdges {
		if us < e {
			return i
		}
	}
	return len(histEdges)
}
