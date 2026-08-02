package main

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/doug/gossamer/chart"
)

// dataset is the sample finance data the dashboard visualizes. It is generated
// deterministically (fixed seed) so screenshots and tests are stable; a real
// Ledger would import transactions from a local CSV/OFX file.
type dataset struct {
	byCategory []chart.Datum // spend per category (this month)
	balance    []chart.Datum // daily account balance
	thisWeek   []chart.Datum // spend per weekday
	budget     float64       // monthly budget target
	latest     float64       // most recent balance
	spent      float64       // total spent this month
	base       time.Time     // day 0 of the balance series
}

func sampleData() dataset {
	r := rand.New(rand.NewSource(42))

	cats := []struct {
		name string
		amt  float64
	}{
		{"Rent", 1850}, {"Food", 620}, {"Transit", 180},
		{"Fun", 240}, {"Utilities", 165}, {"Health", 95}, {"Shopping", 310},
	}
	var byCat []chart.Datum
	var spent float64
	for i, c := range cats {
		byCat = append(byCat, chart.Datum{X: float64(i), Y: c.amt, Label: c.name})
		spent += c.amt
	}

	bal := 4200.0
	var balance []chart.Datum
	for d := 0; d < 30; d++ {
		balance = append(balance, chart.Datum{X: float64(d), Y: bal})
		bal += r.Float64()*300 - 130 // gentle drift with a downward bias
	}

	var week []chart.Datum
	for i, day := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		week = append(week, chart.Datum{X: float64(i), Y: 20 + r.Float64()*130, Label: day})
	}

	return dataset{
		byCategory: byCat,
		balance:    balance,
		thisWeek:   week,
		budget:     3800,
		latest:     bal,
		spent:      spent,
		base:       time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
	}
}

// dayLabel formats a balance-series x index (0..29) as a calendar date.
func (d dataset) dayLabel(v float64) string {
	return d.base.AddDate(0, 0, int(v+0.5)).Format("Jan 2")
}

// money formats a dollar amount with thousands separators, e.g. "$4,120".
func money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(int64(v+0.5), 10)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return sign + "$" + string(out)
}
