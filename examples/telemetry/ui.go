package main

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/doug/gophics/chart"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// rebuildEvery bounds how often the whole window is re-filtered while data is
// arriving. Every rebuild is a full pass over 100,000 spans, so this is the knob
// that trades freshness against the frame budget: at four a second the scan
// costs a small fraction of one core, and the table still reads as a live tail.
// Filter and sort changes bypass it — those must feel instant.
const rebuildEvery = 250 * time.Millisecond

type App struct{ Store *Store }

func (App) CreateState() widget.State { return &dash{} }

type dash struct {
	widget.StateBase[App]
	ctx   widget.Ctx
	store *Store

	snap []Span // reused across rebuilds so a snapshot allocates nothing
	res  Result
	q    Query

	live     bool
	dirty    bool // the query changed; rebuild on the next tick regardless
	since    time.Duration
	selected int
	loadErr  string

	// Test hooks. lastCols records the column set the last build chose, and
	// cellCount (when non-nil) counts cells built — which is how the
	// virtualization claim is tested rather than asserted.
	lastCols  []int
	cellCount *int

	// Ingest rate, sampled from the store's running total.
	lastTotal uint64
	lastAt    time.Time
	ingest    float64
}

// stateHook, if set, receives the state on mount — for tests to drive input.
var stateHook func(*dash)

func (s *dash) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.store = s.W().Store
	s.q = Query{Svc: -1, SortCol: colTime, Desc: true}
	s.live = true
	s.selected = -1
	s.lastAt = time.Now()
	s.lastTotal = s.store.Total()
	s.rebuild()
	ctx.AddTicker(s)
	if stateHook != nil {
		stateHook(s)
	}
}

// Tick decides whether this frame needs a new view. It always asks for a
// rebuild, not just a repaint: the Age column is derived from the clock rather
// than from the data, and it is produced in cell() during Build, so a repaint
// alone redraws the same strings.
//
// That distinction was a visible bug. Invalidate asks for a frame; only
// SetState marks the tree for rebuilding. With a repaint alone the ages sat
// still until something else dirtied a row — and hovering does dirty exactly
// one row, so the row under the pointer would advance its age while every row
// around it stayed where it was.
//
// Rebuilding every frame is what the design already assumes: cell() runs only
// for the rows the viewport shows, twenty-odd of them, however many rows the
// table holds.
func (s *dash) Tick(dt float64) bool {
	s.since += time.Duration(dt * float64(time.Second))
	if s.dirty || (s.live && s.since >= rebuildEvery) {
		s.rebuild()
	}
	if now := time.Now(); now.Sub(s.lastAt) >= time.Second {
		total := s.store.Total()
		s.ingest = float64(total-s.lastTotal) / now.Sub(s.lastAt).Seconds()
		s.lastTotal, s.lastAt = total, now
	}
	s.SetState(nil) // rebuild: Age is computed in cell(), not at paint time
	return true
}

func (s *dash) rebuild() {
	snap, now := s.store.Snapshot(s.snap)
	s.snap = snap
	s.res = Run(snap, now, s.q)
	s.since, s.dirty = 0, false
}

// setQuery applies a filter or sort change and forces an immediate rebuild.
func (s *dash) setQuery(f func(*Query)) {
	s.SetState(func() {
		f(&s.q)
		s.dirty = true
		s.selected = -1
	})
}

// --- Build -------------------------------------------------------------------

func (s *dash) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 18, Child: widget.LayoutBuilder{
			Build: func(cs layout.Constraints) widget.Widget {
				narrow := cs.BoundedW() && cs.Max.W < 900
				return widget.Flex{
					Axis:       layout.Vertical,
					CrossAlign: layout.CrossStretch,
					Children: []widget.Widget{
						s.header(th),
						widget.Sized{H: 14},
						s.tiles(th, narrow),
						widget.Sized{H: 14},
						widget.Sized{H: 196, Child: s.charts(th, narrow)},
						widget.Sized{H: 14},
						s.filters(th, narrow),
						widget.Sized{H: 12},
						widget.Expand(theme.Card{Pad: 10, Child: s.table(th, narrow)}),
						s.detail(th),
					},
				}
			},
		}},
	}}
}

func (s *dash) header(th theme.Theme) widget.Widget {
	sub := fmt.Sprintf("%s spans · %s", commas(int64(len(s.res.Rows))), s.store.Source())
	if s.ingest > 0 {
		sub += fmt.Sprintf(" · %s/s arriving", commas(int64(s.ingest+0.5)))
	}
	if s.loadErr != "" {
		sub = s.loadErr
	}
	return widget.Row(
		widget.Expand(widget.Flex{
			Axis:       layout.Vertical,
			CrossAlign: layout.CrossStart,
			Children: []widget.Widget{
				widget.Text{Value: "Fleet", Font: theme.FontBold, Size: th.Type.Title, Color: th.Text},
				widget.Sized{H: 2},
				widget.Text{Value: sub, Size: th.Type.Caption, Color: th.Muted},
			},
		}),
		s.loadButtons(),
		widget.Text{Value: "Live", Size: th.Type.Label, Color: th.Muted},
		widget.Sized{W: 8},
		theme.Switch{On: s.live, Label: "Live tail",
			OnChange: func(v bool) { s.SetState(func() { s.live = v; s.dirty = v }) }},
	)
}

// tiles is the headline row. Every number on it comes out of the same pass that
// built the table's row order, so the dashboard and the grid can never disagree.
func (s *dash) tiles(th theme.Theme, narrow bool) widget.Widget {
	r := s.res
	errRate := 0.0
	if r.Matching > 0 {
		errRate = float64(r.Server) / float64(r.Matching) * 100
	}
	errTone := th.Success
	if errRate > 1 {
		errTone = th.Warning
	}
	if errRate > 4 {
		errTone = th.Danger
	}

	cells := []widget.Widget{
		tile(th, "Matching", commas(int64(r.Matching)), th.Text),
		tile(th, "p50", fmtDur(r.P50), th.Text),
		tile(th, "p95", fmtDur(r.P95), th.Text),
		tile(th, "p99", fmtDur(r.P99), th.Text),
		tile(th, "5xx", fmt.Sprintf("%.2f%%", errRate), errTone),
		tile(th, "Rebuild", fmt.Sprintf("%.1f ms", float64(r.Elapsed.Microseconds())/1000), th.Muted),
	}
	if narrow { // four across reads better than six squeezed
		cells = cells[:4]
	}
	kids := make([]widget.Widget, 0, len(cells)*2)
	for i, c := range cells {
		if i > 0 {
			kids = append(kids, widget.Sized{W: 10})
		}
		kids = append(kids, widget.Expand(c))
	}
	row := widget.Row(kids...)
	row.CrossAlign = layout.CrossStretch
	return row
}

func tile(th theme.Theme, label, value string, tone paint.Color) widget.Widget {
	return theme.Card{Pad: 10, Child: widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStart,
		Children: []widget.Widget{
			widget.Text{Value: label, Size: th.Type.Caption, Color: th.Muted},
			widget.Sized{H: 3},
			widget.Text{Value: value, Font: "mono", Size: th.Type.Heading, Color: tone},
		},
	}}
}

func (s *dash) charts(th theme.Theme, narrow bool) widget.Widget {
	if narrow {
		return chartCard(th, "Throughput", s.throughput(th))
	}
	order, capped := s.svcOrder()
	row := widget.Row(
		widget.Flexible{Flex: 3, Child: chartCard(th, "Throughput · spans/s", s.throughput(th))},
		widget.Sized{W: 12},
		widget.Flexible{Flex: 3, Child: chartCard(th, "Latency distribution", s.latency(th))},
		widget.Sized{W: 12},
		widget.Flexible{Flex: 3, Child: chartCard(th, svcTitle(capped), s.byService(th, order))},
	)
	row.CrossAlign = layout.CrossStretch
	return row
}

func chartCard(th theme.Theme, title string, c widget.Widget) widget.Widget {
	return theme.Card{Pad: 12, Child: widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{
			widget.Text{Value: title, Size: th.Type.Label, Color: th.Muted},
			widget.Sized{H: 8},
			widget.Expand(c),
		},
	}}
}

// throughput is the last minute of arrivals, one point a second. It is drawn
// from the filtered set, so narrowing to one service narrows the chart with it.
func (s *dash) throughput(th theme.Theme) widget.Widget {
	data := make([]chart.Datum, 0, ThroughputSecs)
	// The newest second is still filling, so it always reads low; drop it.
	for i := range ThroughputSecs - 1 {
		data = append(data, chart.Datum{X: float64(i - (ThroughputSecs - 1)), Y: float64(s.res.PerSec[i])})
	}
	col := th.ChartAt(0)
	return chart.Chart{
		Marks: []chart.Mark{
			chart.AreaMark{Data: data, Color: col, Alpha: 0.18},
			chart.LineMark{Data: data, Color: col, Width: 2},
		},
		XAxis:      chart.Axis{Ticks: 4, Format: func(v float64) string { return fmt.Sprintf("%.0fs", v) }},
		YAxis:      chart.Axis{Ticks: 3},
		LabelColor: th.Text, AxisColor: th.Muted, GridColor: th.Border,
	}
}

// latency is the log-bucketed histogram. Bars are tinted by how slow the bucket
// is, so the shape of the tail is legible without reading the axis.
func (s *dash) latency(th theme.Theme) widget.Widget {
	pairs := make([]chart.Pair, 0, len(histLabels))
	for i, l := range histLabels {
		pairs = append(pairs, chart.Pair{Label: l, Value: float64(s.res.Hist[i])})
	}
	data := chart.Pairs(pairs)
	for i := range data {
		switch {
		case i >= len(histLabels)-2:
			data[i].Color = th.Danger
		case i >= len(histLabels)-4:
			data[i].Color = th.Warning
		default:
			data[i].Color = th.ChartAt(1)
		}
	}
	return chart.Chart{
		Marks:      []chart.Mark{chart.BarMark{Data: data}},
		XAxis:      chart.Axis{Ticks: len(histLabels)},
		YAxis:      chart.Axis{Ticks: 3},
		LabelColor: th.Text, AxisColor: th.Muted, GridColor: th.Border,
	}
}

// svcOrder picks which services the p95 chart shows: the busiest, in dictionary
// order so the bars don't reshuffle between rebuilds. It reports the total when
// it had to cap, so the title can admit it.
func (s *dash) svcOrder() (order []int, capped int) {
	order = make([]int, len(s.res.SvcCount))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return s.res.SvcCount[order[a]] > s.res.SvcCount[order[b]] })
	// Services with nothing in the current filter would draw as empty slots.
	for len(order) > 0 && s.res.SvcCount[order[len(order)-1]] == 0 {
		order = order[:len(order)-1]
	}
	if len(order) > maxSvcBars {
		capped, order = len(order), order[:maxSvcBars]
	}
	sort.Ints(order)
	return order, capped
}

func svcTitle(capped int) string {
	if capped == 0 {
		return "p95 by service"
	}
	return fmt.Sprintf("p95 by service · top %d of %d", maxSvcBars, capped)
}

// byService is where a fleet-wide incident becomes obvious: one service's p95
// climbs away from the others and its bar turns.
func (s *dash) byService(th theme.Theme, order []int) widget.Widget {
	pairs := make([]chart.Pair, 0, len(order))
	for _, i := range order {
		pairs = append(pairs, chart.Pair{Label: shortName(svcDict.Name(uint16(i))), Value: float64(s.res.SvcP95[i]) / 1000})
	}
	data := chart.Pairs(pairs)
	for i := range data {
		switch {
		case data[i].Y > 400:
			data[i].Color = th.Danger
		case data[i].Y > 150:
			data[i].Color = th.Warning
		default:
			data[i].Color = th.ChartAt(2)
		}
	}
	return chart.Chart{
		Marks:      []chart.Mark{chart.BarMark{Data: data}},
		XAxis:      chart.Axis{Ticks: len(data)},
		YAxis:      chart.Axis{Ticks: 3, Format: func(v float64) string { return fmt.Sprintf("%.0fms", v) }},
		LabelColor: th.Text, AxisColor: th.Muted, GridColor: th.Border,
	}
}

var statusNames = []string{"All", "2xx", "4xx", "5xx"}

func (s *dash) filters(th theme.Theme, narrow bool) widget.Widget {
	svcOpts := append([]string{"All services"}, svcDict.Names()...)
	search := theme.Field{
		Value:       s.q.Text,
		Placeholder: "Filter by service, route, host, or trace prefix…",
		OnChange:    func(v string) { s.setQuery(func(q *Query) { q.Text = v }) },
	}
	status := theme.Segmented{Options: statusNames, Selected: s.q.Status,
		OnChange: func(i int) { s.setQuery(func(q *Query) { q.Status = i }) }}
	svc := theme.Dropdown{Options: svcOpts, Selected: s.q.Svc + 1,
		OnChange: func(i int) { s.setQuery(func(q *Query) { q.Svc = i - 1 }) }}

	if narrow {
		return widget.Flex{
			Axis:       layout.Vertical,
			CrossAlign: layout.CrossStretch,
			Children: []widget.Widget{
				search,
				widget.Sized{H: 8},
				status,
			},
		}
	}
	row := widget.Row(
		widget.Expand(search),
		widget.Sized{W: 10},
		widget.Sized{W: 230, Child: status},
		widget.Sized{W: 10},
		widget.Sized{W: 170, Child: svc},
	)
	row.CrossAlign = layout.CrossCenter
	return row
}

// fullCols is the desktop column set. narrowCols keeps only what survives a
// phone-width table; both index the same colX constants, so a header tap sorts
// the same way in either layout.
var (
	fullCols   = []int{colTime, colAge, colService, colRoute, colHost, colTrace, colStatus, colLatency, colBytes}
	narrowCols = []int{colTime, colService, colStatus, colLatency}
)

// Fixed widths are sized to their widest real content plus the table's column
// gap — a monospaced "11:23:54.728" is about 94 points at the label size, and a
// column narrower than that lets the timestamp run into the next one.
var colSpec = map[int]theme.Col{
	colTime:    {Title: "Time", Width: 112},
	colAge:     {Title: "Age", Width: 62, Align: theme.AlignEnd},
	colService: {Title: "Service", Flex: 1},
	colRoute:   {Title: "Route", Flex: 3},
	colHost:    {Title: "Host", Width: 96},
	colTrace:   {Title: "Trace", Width: 84},
	colStatus:  {Title: "Status", Width: 66, Align: theme.AlignCenter},
	colLatency: {Title: "Latency", Width: 86, Align: theme.AlignEnd},
	colBytes:   {Title: "Bytes", Width: 82, Align: theme.AlignEnd},
}

func (s *dash) table(th theme.Theme, narrow bool) widget.Widget {
	set := fullCols
	if narrow {
		set = narrowCols
	}
	s.lastCols = set
	cols := make([]theme.Col, len(set))
	for i, c := range set {
		cols[i] = colSpec[c]
	}
	// Sorting is reported against the logical column, not the visible one, so
	// the indicator lands on the right header after a layout change.
	sortAt := -1
	for i, c := range set {
		if c == s.q.SortCol {
			sortAt = i
		}
	}
	return theme.Table{
		Columns:    cols,
		Count:      len(s.res.View),
		RowHeight:  28,
		Selectable: true,
		Selected:   s.selected,
		OnTapRow:   func(i int) { s.SetState(func() { s.selected = i }) },
		Sortable:   true,
		SortCol:    sortAt,
		SortDesc:   s.q.Desc,
		OnSort: func(i int, desc bool) {
			s.setQuery(func(q *Query) { q.SortCol, q.Desc = set[i], desc })
		},
		Cell: func(row, col int) widget.Widget { return s.cell(th, row, set[col]) },
	}
}

// cell builds one visible cell. It is called only for rows the viewport shows —
// twenty-odd of them — which is what makes a column like Age, recomputed from
// the clock on every frame, cost nothing across a hundred thousand rows.
func (s *dash) cell(th theme.Theme, row, col int) widget.Widget {
	if row < 0 || row >= len(s.res.View) {
		return nil
	}
	if s.cellCount != nil {
		*s.cellCount++
	}
	sp := s.res.Rows[s.res.View[row]]
	mono := func(txt string, c paint.Color) widget.Widget {
		return widget.Text{Value: txt, Font: "mono", Size: th.Type.Label, Color: c}
	}
	switch col {
	case colTime:
		return mono(s.store.Wall(sp.At).Format("15:04:05.000"), th.Muted)
	case colAge:
		return mono(fmtAge(s.store.Now()-sp.At), th.Muted)
	case colService:
		return widget.Text{Value: svcDict.Name(sp.Svc), Size: th.Type.Label, Color: th.Text, Ellipsis: true, MaxLines: 1}
	case colRoute:
		return widget.Text{Value: routeDict.Name(sp.Route), Size: th.Type.Label, Color: th.Text, Ellipsis: true, MaxLines: 1}
	case colHost:
		return mono(hostDict.Name(sp.Host), th.Muted)
	case colTrace:
		// The column shows the leading digits every tracing UI shows; the full
		// 32 are in the detail strip, where they can be read and copied.
		return mono(sp.TraceHex()[:8], th.Muted)
	case colStatus:
		return mono(fmt.Sprint(sp.Code), statusColor(th, sp.Code))
	case colLatency:
		return mono(fmtDur(sp.Dur), latencyColor(th, sp.Dur))
	default:
		if sp.Bytes < 0 { // the capture didn't record a response size
			return mono("—", th.Muted)
		}
		return mono(commas(int64(sp.Bytes)), th.Muted)
	}
}

// detail is the strip under the table describing the selected row in full —
// the columns the grid had to truncate, plus the trace ID to search on.
func (s *dash) detail(th theme.Theme) widget.Widget {
	if s.selected < 0 || s.selected >= len(s.res.View) {
		return widget.Sized{H: 0}
	}
	sp := s.res.Rows[s.res.View[s.selected]]
	line := fmt.Sprintf("%s  %s  %s  on %s  →  %d in %s, %s",
		sp.TraceHex(), svcDict.Name(sp.Svc), routeDict.Name(sp.Route), hostDict.Name(sp.Host),
		sp.Code, fmtDur(sp.Dur), fmtBytes(sp.Bytes))
	return widget.Padding{Insets: geom.Insets{Top: 10},
		Child: widget.Align{X: 0, Y: 0.5,
			Child: widget.Text{Value: line, Font: "mono", Size: th.Type.Caption,
				Color: th.Text, Ellipsis: true, MaxLines: 1}}}
}

func statusColor(th theme.Theme, code uint16) paint.Color {
	switch {
	case code >= 500:
		return th.Danger
	case code >= 400:
		return th.Warning
	default:
		return th.Success
	}
}

func latencyColor(th theme.Theme, us int32) paint.Color {
	switch {
	case us > 1_000_000:
		return th.Danger
	case us > 200_000:
		return th.Warning
	default:
		return th.Text
	}
}

// --- Formatting --------------------------------------------------------------

func fmtDur(us int32) string {
	switch {
	case us <= 0:
		return "—"
	case us < 1000:
		return fmt.Sprintf("%d µs", us)
	case us < 10_000:
		return fmt.Sprintf("%.2f ms", float64(us)/1000)
	case us < 1_000_000:
		return fmt.Sprintf("%.1f ms", float64(us)/1000)
	default:
		return fmt.Sprintf("%.2f s", float64(us)/1e6)
	}
}

func fmtAge(ms int32) string {
	switch {
	case ms < 0:
		return "0.0s"
	case ms < 60_000:
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	default:
		return fmt.Sprintf("%dm", ms/60_000)
	}
}

func fmtBytes(b int32) string {
	if b < 0 {
		return "size unrecorded"
	}
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f kB", float64(b)/1024)
}

// commas groups an integer for reading: 100000 → "100,000".
func commas(n int64) string {
	s := fmt.Sprint(n)
	neg := ""
	if s[0] == '-' {
		neg, s = "-", s[1:]
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return neg + string(out)
}

// maxSvcBars is how many services the p95 chart can label legibly at the width
// it gets. A capture with more is truncated to the busiest — and says so, rather
// than quietly implying the rest are fine.
const maxSvcBars = 8

// shortName trims a service name to what fits under one bar.
func shortName(s string) string {
	if len(s) <= 7 {
		return s
	}
	return s[:6] + "…"
}

// loadButtons offers the two ways into real data: the bundled sample capture,
// and a file off the user's disk where the platform gives us a picker. On a
// platform without one ctx.FilePicker() is nil and the button simply isn't
// there — the capability layer's whole contract in two lines.
func (s *dash) loadButtons() widget.Widget {
	kids := []widget.Widget{
		theme.Button{Label: "Sample OTLP", OnTap: s.loadSample},
	}
	if s.ctx.FilePicker() != nil {
		kids = append(kids, widget.Sized{W: 8},
			theme.Button{Label: "Open OTLP…", OnTap: s.openOTLP})
	}
	kids = append(kids, widget.Sized{W: 14})
	return widget.Row(kids...)
}

func (s *dash) loadSample() { s.load(strings.NewReader(sampleOTLP), "otlp-sample.json") }

func (s *dash) openOTLP() {
	s.ctx.FilePicker().Open(shell.OpenOptions{Accept: []string{".json", "application/json"}},
		func(files []shell.PickedFile, err error) {
			if err != nil || len(files) == 0 {
				return
			}
			s.load(bytes.NewReader(files[0].Data), files[0].Name)
		})
}

// load decodes a capture into a fresh vocabulary and, only if that succeeds,
// installs both. A failed decode leaves the current dataset exactly as it was —
// which is why the decoder interns into a vocabulary it is handed rather than
// into the live one.
func (s *dash) load(r io.Reader, name string) {
	v := newVocab()
	spans, err := DecodeOTLP(r, v)
	s.SetState(func() {
		if err != nil {
			s.loadErr = err.Error()
			return
		}
		s.loadErr = ""
		s.live = false
		useVocab(v)
		s.store.Replace(spans, name) // also stops the synthetic producer
		s.q = Query{Svc: -1, SortCol: colTime, Desc: true}
		s.selected = -1
		s.dirty = true
	})
}
