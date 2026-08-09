package widget

import "testing"

// ctxSpy is a stateless widget that hands its build context to the test.
type ctxSpy struct {
	OnBuild func(Ctx)
}

func (c ctxSpy) Build(ctx Ctx) Widget {
	if c.OnBuild != nil {
		c.OnBuild(ctx)
	}
	return Sized{W: 1, H: 1}
}

func TestProvideNearestWins(t *testing.T) {
	var got int
	var ok bool
	o := newOwner()
	o.SetRoot(Provide[int]{Value: 1, Child: Provide[int]{Value: 2, Child: ctxSpy{
		OnBuild: func(ctx Ctx) { got, ok = Of[int](ctx) },
	}}})
	if !ok || got != 2 {
		t.Fatalf("Of[int] = %d,%v — nearest Provide must shadow the outer one", got, ok)
	}
}

func TestProvideDifferentTypesCoexist(t *testing.T) {
	var gotInt int
	var gotStr string
	o := newOwner()
	o.SetRoot(Provide[int]{Value: 7, Child: Provide[string]{Value: "s", Child: ctxSpy{
		OnBuild: func(ctx Ctx) {
			gotInt, _ = Of[int](ctx)
			gotStr, _ = Of[string](ctx)
		},
	}}})
	if gotInt != 7 || gotStr != "s" {
		t.Fatalf("got %d/%q, want 7/\"s\"", gotInt, gotStr)
	}
}

// A Provide is scoped to its subtree: a sibling must not see the value.
func TestProvideScopedToSubtree(t *testing.T) {
	var inVal string
	var inOK, outOK bool
	o := newOwner()
	o.SetRoot(Row(
		Provide[string]{Value: "scoped", Child: ctxSpy{
			OnBuild: func(ctx Ctx) { inVal, inOK = Of[string](ctx) },
		}},
		ctxSpy{
			OnBuild: func(ctx Ctx) { _, outOK = Of[string](ctx) },
		},
	))
	if !inOK || inVal != "scoped" {
		t.Fatalf("in-scope lookup = %q,%v", inVal, inOK)
	}
	if outOK {
		t.Fatal("sibling outside the Provide subtree saw the value")
	}
}

// Reconciling to a new provided value reaches descendants on their rebuild.
func TestProvideValueUpdateReachesDescendants(t *testing.T) {
	var got int
	tree := func(v int) Widget {
		return Provide[int]{Value: v, Child: ctxSpy{
			OnBuild: func(ctx Ctx) { got, _ = Of[int](ctx) },
		}}
	}
	o := newOwner()
	o.SetRoot(tree(1))
	if got != 1 {
		t.Fatalf("initial value = %d, want 1", got)
	}
	o.SetRoot(tree(9))
	if got != 9 {
		t.Fatalf("updated value = %d, want 9 (eager rebuild must re-deliver)", got)
	}
}

func TestOfAbsentReturnsFalse(t *testing.T) {
	var ok bool
	o := newOwner()
	o.SetRoot(ctxSpy{OnBuild: func(ctx Ctx) { _, ok = Of[float64](ctx) }})
	if ok {
		t.Fatal("Of reported a value with no Provide in scope")
	}
}

func TestMustOfPanicsWhenAbsent(t *testing.T) {
	var recovered any
	o := newOwner()
	o.SetRoot(ctxSpy{OnBuild: func(ctx Ctx) {
		defer func() { recovered = recover() }()
		MustOf[float64](ctx)
	}})
	if recovered == nil {
		t.Fatal("MustOf did not panic with no Provide in scope")
	}
}
