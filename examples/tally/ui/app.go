// Command tally is a native, local-first personal-finance app. Your data is a
// plain-text beancount file; the beango engine does the accounting; gophics draws
// the UI — one Go codebase for desktop, web, and mobile, testable with `go test`.
//
// This is the P1b slice: a balance tree you can drill into, and a per-account
// register rendered with the Tufte-styled theme.Table. Editing, charts, and
// native file open follow.
package main

import (
	_ "embed"
	"log"
	"strings"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/shopspring/decimal"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/intl"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"

	"github.com/dougfritz/tally/bean"
	"github.com/dougfritz/tally/book"
)

// demoLedger is a realistic ~6k-line beancount file, embedded so the app runs
// self-contained on first launch (and in the browser, where there's no filesystem).
//
//go:embed testdata/example.beancount
var demoLedger []byte

// Tally is the root widget.
type Tally struct{}

func (Tally) CreateState() widget.State { return &state{} }

type state struct {
	widget.StateBase[Tally]
	book *book.Book
	tree *bean.Tree
	err  error

	loaded       bool
	prefsChecked bool

	// view selects the top-level screen; a non-empty account overrides it with
	// the register drill-down.
	view view

	// Dashboard series, computed once per loaded ledger (see ensureSeries).
	seriesReady  bool
	baseCurrency string
	netWorth     []book.Point
	income       []book.Point
	expenses     []book.Point
	categories   []book.Category
	unpriced     []string

	// form is the add-transaction panel (see addform.go).
	form addForm

	// account is the drilled-into account ("" → the balances overview).
	account  string
	currency string
	entries  []book.Entry
	selected int
	filter   string
}

// view is the selected top-level screen.
type view int

const (
	viewOverview view = iota
	viewBalances
)

// stateHook lets tests observe the mounted state (see render_test.go).
var stateHook func(*state)

func (s *state) Init(widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.selected = -1
}

// prefKeyLedger is where the path of the last opened ledger is remembered.
const prefKeyLedger = "ledger.path"

// ensureLoaded opens a ledger, upgrading to the remembered one once the shell has
// wired its capabilities.
//
// The ordering matters: the widget tree mounts — Init and the first Build — before
// a window exists, so ctx.Preferences() is nil on that first pass and only becomes
// available on the frame after wiring (which triggers a rebuild). A one-shot
// "load on first Build" would therefore always miss it and show the demo forever.
// So the demo loads immediately (something is on screen from frame one) and the
// remembered ledger replaces it as soon as Preferences appears.
func (s *state) ensureLoaded(ctx widget.Ctx) { s.loadWith(ctx.Preferences()) }

// loadWith holds the logic, taking the capability as a parameter so it can be
// unit-tested against a fake store (including a nil one).
func (s *state) loadWith(p shell.Preferences) {
	if p != nil && !s.prefsChecked {
		s.prefsChecked = true
		if path, ok := p.Get(prefKeyLedger); ok && path != "" {
			if b, err := book.Open(path); err == nil {
				if tr, err := b.Tree(); err == nil {
					s.book, s.tree, s.err = b, tr, nil
					s.loaded, s.seriesReady = true, false
					return
				}
			}
			// The remembered file is unreadable now (moved, deleted, renamed).
			// Drop the stale entry rather than nagging about it every launch.
			_ = p.Delete(prefKeyLedger)
		}
	}
	if s.loaded {
		return
	}
	s.loaded = true
	s.book, s.err = book.OpenBytes("example.beancount", demoLedger)
	if s.err == nil {
		s.tree, s.err = s.book.Tree()
	}
}

// open drills into an account's register; only accounts with their own postings
// have one (a parent like Assets:US:ETrade aggregates children and has none).
func (s *state) open(account string) {
	curs := s.book.Currencies(account)
	if len(curs) == 0 {
		return
	}
	entries, err := s.book.Register(account, curs[0])
	s.SetState(func() {
		s.account, s.currency, s.entries, s.selected, s.filter = account, curs[0], entries, -1, ""
		if err != nil {
			s.err = err
		}
	})
}

func (s *state) back() {
	s.SetState(func() { s.account, s.entries, s.selected, s.filter = "", nil, -1, "" })
}

func (s *state) Build(ctx widget.Ctx) widget.Widget {
	s.ensureLoaded(ctx)
	th := theme.Auto(ctx)
	var body widget.Widget
	switch {
	case s.err != nil:
		body = widget.Padding{All: 24, Child: widget.Text{
			S: "Couldn't open the ledger: " + s.err.Error(), Color: th.Danger, Wrap: true,
		}}
	case s.account != "":
		body = s.registerView(th)
	case s.view == viewOverview:
		s.ensureSeries()
		body = s.overviewView(th)
	default:
		body = widget.Expand(widget.Scroll{Child: widget.Padding{
			Insets: geom.Insets{Left: 24, Right: 24, Top: 8, Bottom: 24},
			Child:  s.balancesView(th),
		}})
	}
	root := widget.Column(s.header(th, ctx), body)
	root.CrossAlign = layout.CrossStretch
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg, Child: root}}
}

// balancesView is the scrolling account balance tree.
func (s *state) balancesView(th theme.Theme) widget.Widget {
	rows := make([]widget.Widget, 0, 128)
	for i, root := range s.tree.Roots {
		if i > 0 {
			rows = append(rows, widget.Sized{H: 18})
		}
		s.appendNode(th, root, &rows)
	}
	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch
	return col
}

// appendNode emits one row per balance-tree node, indented by depth, with the
// account name on the left and its balances right-aligned in tabular figures.
// Rows for accounts that have their own postings are tappable — they drill into
// the register.
func (s *state) appendNode(th theme.Theme, n *bean.Node, out *[]widget.Widget) {
	isRoot := n.Depth == 0
	name := n.Name
	if !isRoot {
		name = lastSegment(string(n.Account))
	}

	nameText := widget.Text{S: name, Size: th.Type.Body, Color: th.Text}
	if isRoot {
		nameText.Font, nameText.Size = theme.FontBold, th.Type.Heading
	}

	var row widget.Widget = widget.Padding{
		Insets: geom.Insets{Left: float32(n.Depth) * 18, Top: 5, Bottom: 5},
		Child: widget.Row(
			nameText,
			widget.Expand(widget.Sized{W: 16}),
			s.amountText(th, n),
		),
	}
	if !isRoot && len(s.book.Currencies(string(n.Account))) > 0 {
		acct := string(n.Account)
		row = theme.Tappable{OnTap: func() { s.open(acct) }, Radius: 6, Child: row}
	}
	*out = append(*out, row)
	if isRoot {
		*out = append(*out, divider(th))
	}
	for _, c := range n.Children {
		s.appendNode(th, c, out)
	}
}

// amountText renders a node's non-zero balances (usually one currency) as
// right-aligned monospace figures, so columns of numbers line up.
func (s *state) amountText(th theme.Theme, n *bean.Node) widget.Widget {
	if n.Balance == nil {
		return widget.Sized{}
	}
	parts := make([]string, 0, 2)
	for _, cur := range s.tree.Currencies {
		amt := n.Balance.Get(cur)
		if amt.IsZero() {
			continue
		}
		parts = append(parts, fmtMoney(amt)+" "+cur)
	}
	if len(parts) == 0 {
		return widget.Text{S: "—", Font: "mono", Size: th.Type.Body, Color: th.Muted}
	}
	return widget.Text{S: strings.Join(parts, "   "), Font: "mono", Size: th.Type.Body, Color: th.Text}
}

// registerView is one account's transaction register in the Tufte data table:
// date, payee/narration, the counterpart account, the amount, and a running
// balance — with a filter box above it.
func (s *state) registerView(th theme.Theme) widget.Widget {
	rows := s.visibleEntries()

	tbl := theme.Table{
		Columns: []theme.Col{
			{Title: "Date", Width: 96},
			{Title: "Payee / narration", Flex: 3},
			{Title: "Account", Flex: 3},
			{Title: "Amount", Width: 124, Align: theme.AlignEnd},
			{Title: "Balance", Width: 132, Align: theme.AlignEnd},
		},
		Count:      len(rows),
		Selectable: true,
		Selected:   s.selected,
		OnTapRow:   func(i int) { s.SetState(func() { s.selected = i }) },
		Cell: func(r, c int) widget.Widget {
			e := rows[r]
			switch c {
			case 0:
				return widget.Text{S: fmtDate(e.Date), Font: "mono", Size: th.Type.Body, Color: th.Muted}
			case 1:
				return widget.Text{S: describe(e), Size: th.Type.Body, Color: th.Text, Ellipsis: true, MaxLines: 1}
			case 2:
				return widget.Text{S: e.Other, Size: th.Type.Body, Color: th.Muted, Ellipsis: true, MaxLines: 1}
			case 3:
				return widget.Text{S: fmtMoney(e.Amount), Font: "mono", Size: th.Type.Body, Color: amountColor(th, e.Amount)}
			default:
				return widget.Text{S: fmtMoney(e.Balance), Font: "mono", Size: th.Type.Body, Color: th.Text}
			}
		},
	}

	bar := widget.Padding{
		Insets: geom.Insets{Left: 24, Right: 24, Top: 4, Bottom: 10},
		Child: widget.Row(
			theme.Button{Label: "‹ Balances", OnTap: s.back},
			widget.Sized{W: 14},
			widget.Text{S: s.account, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text, Ellipsis: true, MaxLines: 1},
			widget.Sized{W: 8},
			widget.Text{S: s.currency, Size: th.Type.Label, Color: th.Muted},
			widget.Expand(widget.Sized{W: 16}),
			widget.Sized{W: 220, Child: theme.Field{
				Value:       s.filter,
				Placeholder: "Filter…",
				OnChange:    func(v string) { s.SetState(func() { s.filter = v; s.selected = -1 }) },
			}},
		),
	}

	footer := widget.Padding{
		Insets: geom.Insets{Left: 24, Right: 24, Top: 8, Bottom: 14},
		Child: widget.Text{
			S:     pluralize(len(rows), "transaction") + total(rows, s.currency),
			Size:  th.Type.Caption,
			Color: th.Muted,
		},
	}

	col := widget.Column(
		bar,
		widget.Expand(widget.Padding{Insets: geom.Insets{Left: 12, Right: 12}, Child: tbl}),
		footer,
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Expand(col)
}

// visibleEntries applies the filter box to the register (case-insensitive across
// payee, narration and the counterpart account).
func (s *state) visibleEntries() []book.Entry {
	q := strings.ToLower(strings.TrimSpace(s.filter))
	if q == "" {
		return s.entries
	}
	out := make([]book.Entry, 0, len(s.entries))
	for _, e := range s.entries {
		hay := strings.ToLower(e.Payee + " " + e.Narration + " " + e.Other + " " + e.Date)
		if strings.Contains(hay, q) {
			out = append(out, e)
		}
	}
	return out
}

// header is the top bar: the app name, an Open button where the platform offers a
// file picker, and the loaded ledger's name.
func (s *state) header(th theme.Theme, ctx widget.Ctx) widget.Widget {
	name := "no ledger"
	if s.book != nil {
		name = s.book.Path
	}
	row := []widget.Widget{
		widget.Text{S: "Tally", Font: theme.FontBold, Size: th.Type.Title, Color: th.Text},
		widget.Sized{W: 20},
		s.tab(th, "Overview", viewOverview),
		widget.Sized{W: 4},
		s.tab(th, "Balances", viewBalances),
		widget.Expand(widget.Sized{W: 12}),
	}
	// The picker is nil on platforms without one (and the web build carries no
	// filesystem path), so the affordance simply isn't offered there.
	if ctx.FilePicker() != nil {
		row = append(row,
			theme.Button{Label: "Open ledger…", OnTap: func() { s.pick(ctx) }},
			widget.Sized{W: 14},
		)
	}
	if s.book != nil && s.book.CanEdit() {
		row = append(row,
			theme.Button{Label: "+ New transaction", Primary: true, OnTap: func() { s.toggleForm() }},
			widget.Sized{W: 14},
		)
	}
	row = append(row, widget.Text{S: name, Size: th.Type.Label, Color: th.Muted, Ellipsis: true, MaxLines: 1})

	bar := widget.Padding{
		Insets: geom.Insets{Left: 24, Right: 24, Top: 16, Bottom: 14},
		Child:  widget.Row(row...),
	}
	head := widget.Column(bar, widget.Fill{Color: th.Border, Child: widget.Sized{H: 1}}, s.addPanel(th))
	head.CrossAlign = layout.CrossStretch
	return head
}

// tab is one top-level nav item: the selected one is emphasized, the rest are
// quiet, so the header reads as navigation rather than a row of buttons.
func (s *state) tab(th theme.Theme, label string, v view) widget.Widget {
	sel := s.view == v && s.account == ""
	col, font := th.Muted, ""
	if sel {
		col, font = th.Text, theme.FontBold
	}
	return theme.Tappable{
		Radius: 6,
		OnTap:  func() { s.SetState(func() { s.view, s.account, s.entries = v, "", nil }) },
		Child: widget.Padding{
			Insets: geom.Insets{Left: 10, Right: 10, Top: 5, Bottom: 5},
			Child:  widget.Text{S: label, Font: font, Size: th.Type.Body, Color: col},
		},
	}
}

// toggleForm opens or closes the add-transaction panel, seeding it with today's
// date and sensible default accounts on first open.
func (s *state) toggleForm() {
	s.SetState(func() {
		s.form.open = !s.form.open
		if !s.form.open {
			return
		}
		s.form.reset(time.Now())
		if s.form.from == "" || s.form.to == "" {
			from, to := defaultAccounts(s.book.AccountNames())
			s.form.from, s.form.to = from, to
		}
	})
}

// defaultAccounts guesses a starting pair — the first asset account and the first
// expense account — so the common case (spending money) is one dropdown away.
func defaultAccounts(names []string) (from, to string) {
	for _, n := range names {
		if from == "" && strings.HasPrefix(n, "Assets:") {
			from = n
		}
		if to == "" && strings.HasPrefix(n, "Expenses:") {
			to = n
		}
	}
	return from, to
}

// pick opens the platform file panel and loads the chosen beancount file.
func (s *state) pick(ctx widget.Ctx) {
	fp := ctx.FilePicker()
	if fp == nil {
		return
	}
	fp.Open(shell.OpenOptions{Accept: []string{".beancount", ".bean", "text/plain"}},
		func(files []shell.PickedFile, err error) {
			if err != nil {
				s.SetState(func() { s.err = err })
				return
			}
			if len(files) == 0 {
				return // cancelled
			}
			s.load(files[0])
			// Remember it so the next launch opens the same ledger.
			if p := ctx.Preferences(); p != nil && files[0].Path != "" {
				_ = p.Set(prefKeyLedger, files[0].Path)
			}
		})
}

// load replaces the open ledger with a picked file, preferring its real path (so
// includes resolve relative to it) and falling back to its bytes on platforms
// that don't expose one.
func (s *state) load(f shell.PickedFile) {
	var b *book.Book
	var err error
	if f.Path != "" {
		b, err = book.Open(f.Path)
	} else {
		b, err = book.OpenBytes(f.Name, f.Data)
	}
	var tr *bean.Tree
	if err == nil {
		tr, err = b.Tree()
	}
	s.SetState(func() {
		s.err = err
		if err != nil {
			return
		}
		s.book, s.tree = b, tr
		s.seriesReady = false
		s.account, s.entries, s.selected, s.filter = "", nil, -1, ""
	})
}

func divider(th theme.Theme) widget.Widget {
	return widget.Padding{Insets: geom.Insets{Top: 2, Bottom: 4}, Child: widget.Fill{Color: th.Border, Child: widget.Sized{H: 1}}}
}

// describe is the register's human line: "Payee — narration", or whichever exists.
func describe(e book.Entry) string {
	switch {
	case e.Payee != "" && e.Narration != "":
		return e.Payee + " — " + e.Narration
	case e.Payee != "":
		return e.Payee
	default:
		return e.Narration
	}
}

// amountColor tints outflows so the sign is readable at a glance without adding ink.
func amountColor(th theme.Theme, d decimal.Decimal) paint.Color {
	if d.IsNegative() {
		return th.Danger
	}
	return th.Text
}

// lastSegment returns the final colon-separated component of a beancount account
// (e.g. "Assets:US:BofA:Checking" → "Checking").
func lastSegment(account string) string {
	if i := strings.LastIndexByte(account, ':'); i >= 0 {
		return account[i+1:]
	}
	return account
}

func pluralize(n int, word string) string {
	s := itoa(n) + " " + word
	if n != 1 {
		s += "s"
	}
	return s
}

// total sums a register slice for the footer.
func total(rows []book.Entry, currency string) string {
	if len(rows) == 0 {
		return ""
	}
	sum := decimal.Zero
	for _, e := range rows {
		sum = sum.Add(e.Amount)
	}
	return "   ·   net " + fmtMoney(sum) + " " + currency
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// locale decides how numbers and dates are punctuated. Resolved once at startup
// from the environment; a settings screen can override it later.
var locale = intl.Auto()

// fmtMoney formats a decimal in the reader's locale — 1,234.56 in the US,
// 1.234,56 in Germany. It formats to two places first so a ledger's columns are
// even, then hands the digits to intl, which re-punctuates without rounding.
func fmtMoney(d decimal.Decimal) string {
	return locale.Number(d.StringFixed(2))
}

// fmtDate renders an ISO date (as the engine stores them) in the locale's style.
// An unparsable value is passed through rather than blanked: showing the raw text
// is more useful than hiding it.
func fmtDate(iso string) string {
	if len(iso) != 10 || iso[4] != '-' || iso[7] != '-' {
		return iso
	}
	y, m, d := atoiSafe(iso[0:4]), atoiSafe(iso[5:7]), atoiSafe(iso[8:10])
	if y == 0 || m == 0 || d == 0 {
		return iso
	}
	return locale.Date(y, m, d)
}

func atoiSafe(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func main() {
	if err := app.Run(Tally{}, app.Config{
		Title:      "Tally",
		AppID:      "com.dougfritz.tally",
		Size:       geom.Size{W: 1040, H: 680},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
			"mono":         gomono.TTF,
		},
	}); err != nil {
		log.Fatal(err)
	}
}
