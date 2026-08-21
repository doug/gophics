package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
)

// --- OTLP --------------------------------------------------------------------

// TestDecodeSample parses the capture that ships with the example. It is the
// guard on the whole OTLP path: if the semantic conventions this reads ever
// stop lining up with what the file contains, everything downstream is quietly
// blank rather than wrong-looking.
func TestDecodeSample(t *testing.T) {
	v := newVocab()
	spans, err := DecodeOTLP(strings.NewReader(sampleOTLP), v)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) < 50 {
		t.Fatalf("decoded %d spans, want the sample's full set", len(spans))
	}
	for _, want := range []string{"gateway", "payments", "catalog", "checkout", "inventory"} {
		if !slices.Contains(v.svc.Names(), want) {
			t.Errorf("service %q missing; got %v", want, v.svc.Names())
		}
	}
	// Routes must arrive as "METHOD /template", not as a bare path: the method
	// comes from a different attribute than the route and has to be joined back.
	for _, r := range v.route.Names() {
		if !strings.Contains(r, " /") {
			t.Errorf("route %q isn't a method + template", r)
		}
	}
	var codes, durs, traces int
	for _, sp := range spans {
		if sp.Code >= 200 {
			codes++
		}
		if sp.Dur > 0 {
			durs++
		}
		if sp.Trace != [16]byte{} {
			traces++
		}
	}
	if codes != len(spans) || durs != len(spans) || traces != len(spans) {
		t.Errorf("codes=%d durations=%d traces=%d of %d spans — some field didn't decode",
			codes, durs, traces, len(spans))
	}
}

// TestDecodeConventions checks both spellings of the HTTP semantic conventions
// (renamed in v1.21) and both encodings of a 64-bit number — the spec says
// quoted, and plenty of encoders emit bare.
func TestDecodeConventions(t *testing.T) {
	const modern = `{"resourceSpans":[{"resource":{"attributes":[
	 {"key":"service.name","value":{"stringValue":"api"}}]},
	 "scopeSpans":[{"spans":[{"traceId":"0102030405060708090a0b0c0d0e0f10","name":"x",
	  "startTimeUnixNano":"1000000000","endTimeUnixNano":"1002500000","attributes":[
	   {"key":"http.request.method","value":{"stringValue":"GET"}},
	   {"key":"http.route","value":{"stringValue":"/v1/a"}},
	   {"key":"http.response.status_code","value":{"intValue":"503"}},
	   {"key":"server.address","value":{"stringValue":"h1"}}]}]}]}]}`

	// The pre-1.21 names, and unquoted numbers throughout.
	const legacy = `{"resourceSpans":[{"resource":{"attributes":[
	 {"key":"service.name","value":{"stringValue":"api"}}]},
	 "scopeSpans":[{"spans":[{"traceId":"0102030405060708090a0b0c0d0e0f10","name":"x",
	  "startTimeUnixNano":1000000000,"endTimeUnixNano":1002500000,"attributes":[
	   {"key":"http.method","value":{"stringValue":"GET"}},
	   {"key":"http.target","value":{"stringValue":"/v1/a"}},
	   {"key":"http.status_code","value":{"intValue":503}},
	   {"key":"net.host.name","value":{"stringValue":"h1"}}]}]}]}]}`

	for name, doc := range map[string]string{"modern": modern, "legacy": legacy} {
		v := newVocab()
		spans, err := DecodeOTLP(strings.NewReader(doc), v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(spans) != 1 {
			t.Fatalf("%s: %d spans, want 1", name, len(spans))
		}
		sp := spans[0]
		if got := v.route.Name(sp.Route); got != "GET /v1/a" {
			t.Errorf("%s: route %q", name, got)
		}
		if sp.Code != 503 {
			t.Errorf("%s: code %d, want 503", name, sp.Code)
		}
		if sp.Dur != 2500 { // 2.5 ms, in microseconds
			t.Errorf("%s: duration %d µs, want 2500", name, sp.Dur)
		}
		if v.host.Name(sp.Host) != "h1" {
			t.Errorf("%s: host %q", name, v.host.Name(sp.Host))
		}
		if got := sp.TraceHex(); got != "0102030405060708090a0b0c0d0e0f10" {
			t.Errorf("%s: trace %q", name, got)
		}
	}
}

// TestDecodeStream checks a file holding several JSON objects back to back —
// what the Collector's file exporter writes, one per line.
func TestDecodeStream(t *testing.T) {
	one := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"a"}}]},
	 "scopeSpans":[{"spans":[{"traceId":"aa","name":"n","startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`
	v := newVocab()
	spans, err := DecodeOTLP(strings.NewReader(one+"\n"+one+"\n"+one), v)
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 3 {
		t.Errorf("got %d spans from 3 concatenated objects, want 3", len(spans))
	}
}

// TestDecodeRejectsNonOTLP checks that pointing the loader at the wrong JSON
// reports it, rather than silently emptying the dashboard.
func TestDecodeRejectsNonOTLP(t *testing.T) {
	if _, err := DecodeOTLP(strings.NewReader(`{"hello":"world"}`), newVocab()); err == nil {
		t.Error("a document with no spans decoded without complaint")
	}
}

// --- Buckets and percentiles -------------------------------------------------

// TestLatBucketMonotonic checks the integer log-bucketing is order-preserving:
// a slower request must never land in an earlier bucket, or every percentile
// off the histogram is wrong.
func TestLatBucketMonotonic(t *testing.T) {
	prev := -1
	for us := int32(1); us < 20_000_000; us = us + 1 + us/64 {
		b := latBucket(us)
		if b < prev {
			t.Fatalf("%d µs → bucket %d, below the previous %d", us, b, prev)
		}
		if got := bucketUs(b); float64(got) < float64(us)*0.85 || float64(got) > float64(us)*1.2 {
			t.Fatalf("bucket %d for %d µs reads back as %d — outside the resolution it claims", b, us, got)
		}
		prev = b
	}
}

// TestPercentileMatchesASort compares the histogram estimate against the exact
// answer from sorting the durations. The histogram exists so the query engine
// never has to take that sort; this is the test that says what it costs.
func TestPercentileMatchesASort(t *testing.T) {
	store := NewStore(3)
	store.Fill(20_000, 30*time.Second)
	snap, now := store.Snapshot(nil)
	res := Run(snap, now, Query{Svc: -1, SortCol: colTime, Desc: true})

	durs := make([]int32, len(snap))
	for i, sp := range snap {
		durs[i] = sp.Dur
	}
	slices.Sort(durs)
	exact := func(p float64) int32 { return durs[int(float64(len(durs))*p)] }

	for _, c := range []struct {
		name string
		got  int32
		want int32
	}{{"p50", res.P50, exact(0.50)}, {"p95", res.P95, exact(0.95)}, {"p99", res.P99, exact(0.99)}} {
		if ratio := float64(c.got) / float64(c.want); ratio < 0.9 || ratio > 1.1 {
			t.Errorf("%s: histogram says %d, sort says %d (%.1f%% off)", c.name, c.got, c.want, (ratio-1)*100)
		}
	}
}

// --- Filtering and sorting ---------------------------------------------------

func fixture(t *testing.T) ([]Span, int32) {
	t.Helper()
	store := NewStore(5)
	store.Fill(30_000, 40*time.Second)
	snap, now := store.Snapshot(nil)
	return snap, now
}

// TestFilterByText checks the compiled-dictionary filter agrees with the
// obvious slow version — matching the strings directly, row by row.
func TestFilterByText(t *testing.T) {
	rows, now := fixture(t)
	for _, q := range []string{"payments", "checkout", "/v1/items", "ord-", "GET"} {
		res := Run(rows, now, Query{Svc: -1, Text: q, SortCol: colTime, Desc: true})
		want := 0
		lower := strings.ToLower(q)
		for _, sp := range rows {
			if strings.Contains(strings.ToLower(svcDict.Name(sp.Svc)), lower) ||
				strings.Contains(strings.ToLower(routeDict.Name(sp.Route)), lower) ||
				strings.Contains(strings.ToLower(hostDict.Name(sp.Host)), lower) {
				want++
			}
		}
		if want == 0 {
			t.Fatalf("%q matched nothing in the fixture; the test proves nothing", q)
		}
		if res.Matching != want {
			t.Errorf("%q matched %d rows, want %d", q, res.Matching, want)
		}
	}
}

// TestFilterByTracePrefix checks searching a trace ID by its leading hex
// digits, including an odd-length query — the case that ends on a half byte.
func TestFilterByTracePrefix(t *testing.T) {
	rows, now := fixture(t)
	full := rows[len(rows)/2].TraceHex()
	for _, n := range []int{3, 4, 7, 8} {
		q := full[:n]
		res := Run(rows, now, Query{Svc: -1, Text: q, SortCol: colTime, Desc: true})
		if res.Matching == 0 {
			t.Fatalf("prefix %q (%d digits) matched nothing", q, n)
		}
		for _, i := range res.View {
			if got := rows[i].TraceHex(); !strings.HasPrefix(got, q) {
				t.Fatalf("prefix %q matched trace %s", q, got)
			}
		}
	}
}

// TestFilterByStatus checks the three status classes partition the window.
func TestFilterByStatus(t *testing.T) {
	rows, now := fixture(t)
	all := Run(rows, now, Query{Svc: -1, Status: statusAny, SortCol: colTime}).Matching
	var sum int
	for _, st := range []int{statusOK, statusClient, statusServer} {
		res := Run(rows, now, Query{Svc: -1, Status: st, SortCol: colTime})
		sum += res.Matching
		for _, i := range res.View {
			code := rows[i].Code
			ok := (st == statusOK && code < 400) ||
				(st == statusClient && code >= 400 && code < 500) ||
				(st == statusServer && code >= 500)
			if !ok {
				t.Fatalf("status class %d admitted code %d", st, code)
			}
		}
	}
	if sum != all {
		t.Errorf("the three classes cover %d rows, the window holds %d", sum, all)
	}
}

// TestSortOrdersEveryColumn checks each sortable column, both directions.
func TestSortOrdersEveryColumn(t *testing.T) {
	rows, now := fixture(t)
	key := map[int]func(Span) string{
		colService: func(s Span) string { return svcDict.Name(s.Svc) },
		colRoute:   func(s Span) string { return routeDict.Name(s.Route) },
		colHost:    func(s Span) string { return hostDict.Name(s.Host) },
		colTrace:   func(s Span) string { return s.TraceHex() },
		colStatus:  func(s Span) string { return fmt.Sprintf("%05d", s.Code) },
		colLatency: func(s Span) string { return fmt.Sprintf("%012d", s.Dur) },
		colBytes:   func(s Span) string { return fmt.Sprintf("%012d", s.Bytes) },
	}
	for col, k := range key {
		for _, desc := range []bool{false, true} {
			res := Run(rows, now, Query{Svc: -1, SortCol: col, Desc: desc})
			for i := 1; i < len(res.View); i++ {
				a, b := k(rows[res.View[i-1]]), k(rows[res.View[i]])
				if (!desc && a > b) || (desc && a < b) {
					t.Fatalf("column %d desc=%v out of order at %d: %q then %q", col, desc, i, a, b)
				}
			}
		}
	}
}

// TestChronologicalFastPathMatchesASort is the test that licenses the
// optimization: when the sort is by time, Run skips sorting entirely and just
// walks the ring. That is only safe while the ring is genuinely time-ordered,
// so this compares the fast path against an actual sort of the same data.
func TestChronologicalFastPathMatchesASort(t *testing.T) {
	rows, now := fixture(t)
	for _, desc := range []bool{false, true} {
		res := Run(rows, now, Query{Svc: -1, SortCol: colTime, Desc: desc})
		if res.SortedRow {
			t.Error("the chronological path sorted; it is supposed to be free")
		}
		want := make([]int32, len(res.View))
		copy(want, res.View)
		sort.SliceStable(want, func(a, b int) bool {
			if desc {
				return rows[want[a]].At > rows[want[b]].At
			}
			return rows[want[a]].At < rows[want[b]].At
		})
		for i := range want {
			if rows[want[i]].At != rows[res.View[i]].At {
				t.Fatalf("desc=%v: fast path diverges from a sort at row %d", desc, i)
			}
		}
	}
}

// TestAggregatesAgreeWithTheView checks the dashboard's numbers describe the
// same rows the table lists — they come from one pass, and this is what says so.
func TestAggregatesAgreeWithTheView(t *testing.T) {
	rows, now := fixture(t)
	res := Run(rows, now, Query{Svc: -1, Text: "GET", SortCol: colLatency, Desc: true})
	if res.Matching != len(res.View) {
		t.Errorf("Matching=%d but the view holds %d rows", res.Matching, len(res.View))
	}
	var server, client int
	var hist int32
	for _, i := range res.View {
		switch code := rows[i].Code; {
		case code >= 500:
			server++
		case code >= 400:
			client++
		}
	}
	for _, n := range res.Hist {
		hist += n
	}
	if server != res.Server || client != res.Client {
		t.Errorf("counted %d/%d 5xx/4xx, result says %d/%d", server, client, res.Server, res.Client)
	}
	if int(hist) != res.Matching {
		t.Errorf("histogram holds %d rows, the view holds %d", hist, res.Matching)
	}
}

// --- Store -------------------------------------------------------------------

// TestRingWrapsOldestFirst overfills the window and checks Snapshot still hands
// back the newest Window spans, oldest first — the ordering everything else
// assumes.
func TestRingWrapsOldestFirst(t *testing.T) {
	store := NewStore(1)
	store.Fill(Window+5_000, 90*time.Second)
	if got := store.Len(); got != Window {
		t.Fatalf("window holds %d spans, want the cap of %d", got, Window)
	}
	if got := store.Total(); got != Window+5_000 {
		t.Errorf("total written %d, want %d", got, Window+5_000)
	}
	snap, _ := store.Snapshot(nil)
	for i := 1; i < len(snap); i++ {
		if snap[i].At < snap[i-1].At {
			t.Fatalf("snapshot out of time order at %d", i)
		}
	}
}

// TestSnapshotReusesItsBuffer checks the per-rebuild copy doesn't allocate — the
// reason a full snapshot several times a second is affordable at all.
func TestSnapshotReusesItsBuffer(t *testing.T) {
	store := NewStore(1)
	store.Fill(Window, 70*time.Second)
	buf, _ := store.Snapshot(nil)
	allocs := testing.AllocsPerRun(5, func() { buf, _ = store.Snapshot(buf) })
	if allocs > 0 {
		t.Errorf("Snapshot allocated %.0f times into a buffer that already fits", allocs)
	}
}

// TestReplaceSortsAndStopsTheProducer covers what loading a capture has to do:
// order it by time (an OTLP file is grouped by service, not by clock) and stop
// the synthetic fleet, whose spans index a vocabulary the load replaced.
func TestReplaceSortsAndStopsTheProducer(t *testing.T) {
	store := NewStore(1)
	v := newVocab()
	spans, err := DecodeOTLP(strings.NewReader(sampleOTLP), v)
	if err != nil {
		t.Fatal(err)
	}
	useVocab(v)
	defer func() { useVocab(newVocab()) }()

	store.Replace(spans, "sample")
	snap, _ := store.Snapshot(nil)
	if len(snap) != len(spans) {
		t.Fatalf("stored %d spans, decoded %d", len(snap), len(spans))
	}
	for i := 1; i < len(snap); i++ {
		if snap[i].At < snap[i-1].At {
			t.Fatalf("loaded window isn't time-ordered at %d", i)
		}
	}
	if newest := snap[len(snap)-1].At; newest != 0 {
		t.Errorf("newest span is at %d ms; the clock should have been rebased to it", newest)
	}

	// The producer must be inert now, or it would write generator spans whose
	// indices mean nothing in the loaded vocabulary.
	before := store.Total()
	stop := make(chan struct{})
	go store.Produce(stop)
	time.Sleep(120 * time.Millisecond)
	close(stop)
	if after := store.Total(); after != before {
		t.Errorf("the producer wrote %d spans after a load", after-before)
	}
}

// TestGeneratorIsDeterministic guards the seeded generator, so a screenshot or a
// golden test over this example is stable.
func TestGeneratorIsDeterministic(t *testing.T) {
	a, _ := NewStore(4).snapshotAfterFill(t)
	b, _ := NewStore(4).snapshotAfterFill(t)
	if len(a) != len(b) {
		t.Fatalf("%d vs %d spans", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed 4 diverged at span %d", i)
		}
	}
}

func (s *Store) snapshotAfterFill(t *testing.T) ([]Span, int32) {
	t.Helper()
	s.Fill(2_000, 5*time.Second)
	return s.Snapshot(nil)
}

// --- The app -----------------------------------------------------------------

// newApp mounts the real app headless. The cell counter is installed before the
// first build, because the widget tree is retained: attaching it afterwards
// would count nothing, since a second render reuses the rows already mounted.
func newApp(t *testing.T, w, h float32) (*app.Headless, *dash, *int) {
	t.Helper()
	store := NewStore(9)
	store.Fill(Window, time.Duration(Window)*time.Second/Rate)

	var st *dash
	cells := new(int)
	stateHook = func(d *dash) { st = d; d.cellCount = cells }
	defer func() { stateHook = nil }()

	hl, err := app.NewHeadless(App{Store: store}, app.Config{
		Size: geom.Size{W: w, H: h}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	hl.Resize(geom.Size{W: w, H: h})
	hl.Render()
	if st == nil {
		t.Fatal("state never mounted")
	}
	return hl, st, cells
}

// TestTableIsVirtualized is the headline claim, tested rather than asserted: a
// table over a hundred thousand rows builds only the cells the viewport shows.
func TestTableIsVirtualized(t *testing.T) {
	_, s, cells := newApp(t, 1360, 860)
	if len(s.res.View) != Window {
		t.Fatalf("view holds %d rows, want the full %d", len(s.res.View), Window)
	}
	built := *cells
	if built == 0 {
		t.Fatal("no cells were built at all")
	}
	// Nine columns over the rows that fit an 860px window — a few hundred, not
	// the ~900,000 an unvirtualized table would need.
	if built > 2_000 {
		t.Errorf("built %d cells for %d rows; the list isn't windowing", built, Window)
	}
}

// TestFilteringIsFastEnough keeps the rebuild inside a frame. It is generous
// (a frame is 16 ms; the scan should be a small fraction of one) so it fails on
// a regression in kind rather than on a busy machine.
func TestFilteringIsFastEnough(t *testing.T) {
	rows, now := fixture(t)
	start := time.Now()
	const runs = 5
	for i := 0; i < runs; i++ {
		Run(rows, now, Query{Svc: -1, Text: "payments", SortCol: colTime, Desc: true})
	}
	if per := time.Since(start) / runs; per > 25*time.Millisecond {
		t.Errorf("a filter over %d rows took %v", len(rows), per)
	}
}

// TestLayoutFollowsTheWindow checks the table drops to its narrow column set on
// a phone rather than squeezing nine columns into 430 points.
func TestLayoutFollowsTheWindow(t *testing.T) {
	h, s, _ := newApp(t, 1360, 860)
	if got := len(s.lastCols); got != len(fullCols) {
		t.Errorf("desktop showed %d columns, want %d", got, len(fullCols))
	}
	h.Resize(geom.Size{W: 430, H: 900})
	h.Render()
	if got := len(s.lastCols); got != len(narrowCols) {
		t.Errorf("phone width showed %d columns, want %d", got, len(narrowCols))
	}
}

// TestRunsWithoutPanic steps the real app — clock, rebuilds, charts, table.
func TestRunsWithoutPanic(t *testing.T) {
	h, s, _ := newApp(t, 1360, 860)
	for i := 0; i < 90; i++ {
		h.Step(1.0 / 60)
	}
	if s.res.Elapsed <= 0 {
		t.Error("no rebuild was ever timed")
	}
	if img := h.Render(); img.Bounds().Dx() != 1360 {
		t.Fatalf("rendered %v", img.Bounds())
	}
}
