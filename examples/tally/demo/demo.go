// Package demo carries the sample ledger Tally opens on first run.
//
// It lives in its own package so the desktop binary, the mobile bind surface and
// the tests all reference one copy of the file rather than each embedding their
// own — a realistic ~6k-line ledger is not something to duplicate.
package demo

import _ "embed"

// Ledger is a realistic beancount file: three years of a salaried person's
// finances, with investments, taxes and balance assertions.
//
//go:embed example.beancount
var Ledger []byte

// Name is the logical filename shown when the demo is loaded.
const Name = "example.beancount"
