package book

import "testing"

// TestOpenExample loads the bundled realistic ledger (beango's example.beancount,
// ~6k lines) and checks the accounting engine produced a sensible balance tree.
func TestOpenExample(t *testing.T) {
	b, err := Open("../demo/example.beancount")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	tr, err := b.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tr.Roots) == 0 {
		t.Fatal("balance tree has no roots")
	}
	// A realistic personal ledger has at least Assets and Expenses populated.
	roots := map[string]bool{}
	for _, r := range tr.Roots {
		roots[r.Name] = true
	}
	for _, want := range []string{"Assets", "Expenses"} {
		if !roots[want] {
			t.Errorf("expected a %q root; got roots %v", want, roots)
		}
	}
	if len(tr.Currencies) == 0 {
		t.Error("expected at least one currency")
	}
}
