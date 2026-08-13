package ui

import (
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"

	"github.com/dougfritz/tally/book"
)

// addForm is the "record a transaction" panel: a date, who it was with, the two
// accounts money moved between, and how much.
//
// Two postings rather than a general split editor, because that covers almost
// every hand-entered transaction (a purchase, a transfer, a paycheck) and keeps
// the form small enough to fill in without thinking. The engine handles splits;
// the form can grow into them when something needs it.
type addForm struct {
	open bool

	date      string
	payee     string
	narration string
	from      string
	to        string
	amount    string

	// err is the validation or write failure to show; result is the confirmation.
	err    string
	result string
	// warnings names balance assertions this edit invalidated — a consequence the
	// user did not ask for and must not discover later.
	warnings []string
}

// reset prepares a blank form dated today, keeping the accounts so entering a run
// of similar transactions doesn't mean retyping them.
func (f *addForm) reset(now time.Time) {
	f.date = now.Format("2006-01-02")
	f.payee, f.narration, f.amount = "", "", ""
	f.err, f.result, f.warnings = "", "", nil
}

// addPanel renders the form, or nothing when it's closed.
// addPanel renders the form. fullScreen is the phone arrangement: scrollable,
// with the keyboard padding inside the scroll so the buttons can be brought
// above the keys.
func (s *state) addPanel(th theme.Theme, fullScreen bool) widget.Widget {
	f := &s.form
	if !f.open {
		return widget.Sized{}
	}

	accounts := s.book.AccountNames()
	field := func(value, placeholder string, set func(string)) widget.Widget {
		return theme.Field{
			Value: value, Placeholder: placeholder,
			OnChange: func(v string) { s.SetState(func() { set(v) }) },
		}
	}
	// A field paired with its label, sized so a wide row's columns line up.
	col := func(w float32, name string, child widget.Widget) widget.Widget {
		return widget.Sized{W: w, Child: labelled(th, name, child)}
	}

	// A phone cannot hold three fields across; stack every field instead. The
	// order is the order you think in: when, who, what, from, to, how much.
	stack := func() widget.Widget {
		c := widget.Column(
			labelled(th, "Date", field(f.date, "YYYY-MM-DD", func(v string) { f.date = v })),
			widget.Sized{H: 10},
			labelled(th, "Payee", field(f.payee, "Who?", func(v string) { f.payee = v })),
			widget.Sized{H: 10},
			labelled(th, "Description", field(f.narration, "What for?", func(v string) { f.narration = v })),
			widget.Sized{H: 10},
			accountPicker(th, "From (money leaves)", f.from, accounts,
				func(v string) { s.SetState(func() { f.from = v }) }),
			widget.Sized{H: 10},
			accountPicker(th, "To (money goes)", f.to, accounts,
				func(v string) { s.SetState(func() { f.to = v }) }),
			widget.Sized{H: 10},
			labelled(th, "Amount", field(f.amount, "0.00", func(v string) { f.amount = v })),
		)
		c.CrossAlign = layout.CrossStretch
		return c
	}

	spread := func() widget.Widget {
		c := widget.Column(
			widget.Row(
				col(130, "Date", field(f.date, "YYYY-MM-DD", func(v string) { f.date = v })),
				widget.Sized{W: 12},
				col(220, "Payee", field(f.payee, "Who?", func(v string) { f.payee = v })),
				widget.Sized{W: 12},
				widget.Expand(labelled(th, "Description",
					field(f.narration, "What for?", func(v string) { f.narration = v }))),
			),
			widget.Sized{H: 12},
			widget.Row(
				widget.Expand(accountPicker(th, "From (money leaves)", f.from, accounts,
					func(v string) { s.SetState(func() { f.from = v }) })),
				widget.Sized{W: 12},
				widget.Expand(accountPicker(th, "To (money goes)", f.to, accounts,
					func(v string) { s.SetState(func() { f.to = v }) })),
				widget.Sized{W: 12},
				col(150, "Amount", field(f.amount, "0.00", func(v string) { f.amount = v })),
			),
		)
		c.CrossAlign = layout.CrossStretch
		return c
	}

	actionRow := []widget.Widget{
		theme.Button{Label: saveLabel(s.book), Primary: true, OnTap: func() { s.submitForm() }},
	}
	if !fullScreen {
		// The full-screen sheet already has Cancel in its own title bar.
		actionRow = append(actionRow, widget.Sized{W: 10},
			theme.Button{Label: "Cancel", OnTap: func() { s.SetState(func() { f.open = false }) }})
	}
	actionRow = append(actionRow, widget.Sized{W: 16}, widget.Expand(s.formStatus(th)))
	actions := widget.Row(actionRow...)

	body := widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		fields := stack()
		if !fullScreen && cs.Max.W >= wideWidth {
			fields = spread()
		}
		c := widget.Column(fields, widget.Sized{H: 14}, actions)
		c.CrossAlign = layout.CrossStretch
		return c
	}}

	rows := widget.Column(body)
	rows.CrossAlign = layout.CrossStretch

	if fullScreen {
		// The keyboard padding goes *inside* the scroll: that makes the content
		// taller than the viewport, so the fields and buttons can be scrolled up
		// clear of the keys. Padding outside a scroll would just move the whole
		// form further down, which is the opposite of the point.
		return widget.Scroll{Child: widget.KeyboardAvoiding{
			Extra: 16,
			Child: widget.Padding{
				Insets: geom.Insets{Left: 16, Right: 16, Top: 14, Bottom: 20},
				Child:  rows,
			},
		}}
	}
	return widget.Padding{
		Insets: geom.Insets{Left: 16, Right: 16, Top: 4, Bottom: 14},
		Child: widget.Decorated{
			Color: th.Surface, Radius: 10, BorderColor: th.Border, BorderWidth: 1,
			Child: widget.Padding{All: 16, Child: rows},
		},
	}
}

// saveLabel tells the truth about where the entry will go: a ledger with no file
// behind it (the bundled demo) can be added to, but nothing is written.
// labelled stacks a caption above a control.
func labelled(th theme.Theme, name string, child widget.Widget) widget.Widget {
	c := widget.Column(
		widget.Text{S: name, Size: th.Type.Label, Color: th.Muted},
		widget.Sized{H: 4},
		child,
	)
	c.CrossAlign = layout.CrossStretch
	return c
}

func saveLabel(b *book.Book) string {
	if b != nil && b.Writable() {
		return "Save to ledger"
	}
	return "Add (not saved — demo ledger)"
}

// formStatus shows the outcome: an error to fix, or a confirmation plus any
// assertions the edit invalidated.
func (s *state) formStatus(th theme.Theme) widget.Widget {
	f := &s.form
	switch {
	case f.err != "":
		return widget.Text{S: f.err, Size: th.Type.Label, Color: th.Danger, Wrap: true}
	case f.result != "" && len(f.warnings) > 0:
		c := widget.Column(
			widget.Text{S: f.result, Size: th.Type.Label, Color: th.Success},
			widget.Sized{H: 3},
			widget.Text{
				S: pluralize(len(f.warnings), "balance check") + " no longer match after this entry — " +
					"the amounts they assert were recorded before it existed.",
				Size: th.Type.Caption, Color: th.Warning, Wrap: true,
			},
		)
		c.CrossAlign = layout.CrossStart
		return c
	case f.result != "":
		return widget.Text{S: f.result, Size: th.Type.Label, Color: th.Success}
	}
	return widget.Sized{}
}

// accountPicker is a dropdown over the ledger's accounts. Choosing from what the
// ledger already declares avoids typos silently creating a new account — the most
// common way a hand-entered transaction goes wrong.
func accountPicker(th theme.Theme, name, selected string, accounts []string, set func(string)) widget.Widget {
	idx := -1
	for i, a := range accounts {
		if a == selected {
			idx = i
			break
		}
	}
	c := widget.Column(
		widget.Text{S: name, Size: th.Type.Label, Color: th.Muted},
		widget.Sized{H: 4},
		theme.Dropdown{
			Options: accounts, Selected: idx, Placeholder: "Choose an account",
			OnChange: func(i int) {
				if i >= 0 && i < len(accounts) {
					set(accounts[i])
				}
			},
		},
	)
	c.CrossAlign = layout.CrossStretch
	return c
}

// submitForm validates and applies the entry.
func (s *state) submitForm() {
	f := &s.form
	date, err := time.Parse("2006-01-02", strings.TrimSpace(f.date))
	if err != nil {
		s.SetState(func() { f.err, f.result = "Date must look like 2026-01-31.", "" })
		return
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(f.amount))
	if err != nil {
		s.SetState(func() { f.err, f.result = "Amount must be a number, like 12.34.", "" })
		return
	}

	res, err := s.book.Add(book.NewEntry{
		Date: date, Payee: f.payee, Narration: f.narration,
		From: f.from, To: f.to, Amount: amount, Currency: s.baseCurrency,
	})
	if err != nil {
		s.SetState(func() { f.err, f.result = capitalize(err.Error())+".", "" })
		return
	}

	// The ledger changed underneath every view, so drop the derived state and let
	// it recompute.
	s.SetState(func() {
		f.err = ""
		f.result = "Added" + savedSuffix(res.Saved)
		f.warnings = res.Invalidated
		f.payee, f.narration, f.amount = "", "", ""
		s.seriesReady = false
		s.refreshAfterEdit()
	})
}

func savedSuffix(saved bool) string {
	if saved {
		return " and saved."
	}
	return " (in memory only)."
}

// refreshAfterEdit rebuilds the views that read from the ledger.
func (s *state) refreshAfterEdit() {
	if tr, err := s.book.Tree(); err == nil {
		s.tree = tr
	}
	if s.account != "" {
		if entries, err := s.book.Register(s.account, s.currency); err == nil {
			s.entries = entries
		}
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
