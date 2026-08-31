package widget

// Provide makes a value available to every widget below it in the tree,
// looked up by type with Of — gophics's InheritedWidget
// with generics instead of runtime type lookups at the call site:
//
//	widget.Provide[Theme]{Value: dark, Child: app}
//	...
//	theme, ok := ctx.Of[Theme]()
//
// The nearest ancestor Provide of the type wins. Values are read at build
// time; because gophics rebuilds reconciled subtrees eagerly, a changed
// value reaches descendants on their next build without explicit
// dependency registration.
type Provide[T any] struct {
	Value T
	Child Widget
}

func (p Provide[T]) Build(Ctx) Widget { return p.Child }

// provider is the type-erased lookup hook.
func (p Provide[T]) provided() any { return p.Value }

type provider interface{ provided() any }

// Of returns the nearest provided value of type T above this context,
// reporting whether one exists.
//
// A method rather than a package-level function since Go 1.27, which allows
// type parameters on methods. It reads at the call site as the lookup it is —
// ctx.Of[Theme]() — instead of a free function that happens to take a Ctx, and
// it stops two very general names, Of and MustOf, sitting in package scope.
func (c Ctx) Of[T any]() (T, bool) {
	for e := c.el; e != nil; e = e.parent {
		if p, ok := e.widget.(provider); ok {
			if v, ok := p.provided().(T); ok {
				return v, true
			}
		}
	}
	var zero T
	return zero, false
}

// MustOf returns the nearest provided T, panicking if absent (for values
// the app guarantees, like a theme or navigator).
func (c Ctx) MustOf[T any]() T {
	v, ok := c.Of[T]()
	if !ok {
		panic("widget: no Provide for requested type in scope")
	}
	return v
}
