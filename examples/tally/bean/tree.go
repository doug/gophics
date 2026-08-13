package bean

import (
	"sort"
	"strings"
)

// Node is one account in a balance tree. A node's Balance includes its own
// postings and all of its descendants', so a parent reads as the total of the
// subtree — which is what makes "Assets" a meaningful number.
type Node struct {
	Name     string // display name: the account's leaf, or the type name at depth 0
	Account  Account
	Depth    int
	Balance  Balance
	Children []*Node
}

// Tree is a hierarchical view of account balances over a period.
type Tree struct {
	Roots      []*Node
	Currencies []string
	From, To   Date
	// Period is false for a point-in-time balance (a balance sheet) and true for
	// a net change over a range (an income statement).
	Period bool
}

// Range selects which postings a tree includes.
type Range struct {
	// From and To bound the postings. A zero To means "everything up to the last
	// entry". When Period is false, From is ignored and the tree is cumulative
	// through To — the balance-sheet reading.
	From, To Date
	Period   bool
}

// AsOf returns a Range for a point-in-time balance through d.
func AsOf(d Date) Range { return Range{To: d} }

// Between returns a Range for the net change over [from, to].
func Between(from, to Date) Range { return Range{From: from, To: to, Period: true} }

// All returns a Range covering every posting, cumulatively.
func All() Range { return Range{} }

// BalanceTree aggregates accounts into a hierarchy, optionally restricted to
// account types ("Assets", "Expenses"; nil means all).
func (l *Ledger) BalanceTree(types []string, r Range) *Tree {
	want := map[string]bool{}
	for _, t := range types {
		want[t] = true
	}

	tree := &Tree{From: r.From, To: r.To, Period: r.Period}
	currencies := map[string]bool{}

	// Index nodes by account path so parents can be created on demand: a ledger
	// may post to Assets:US:BofA:Checking without ever naming Assets:US.
	nodes := map[Account]*Node{}
	var roots []*Node

	nodeFor := func(acct Account) *Node {
		if n, ok := nodes[acct]; ok {
			return n
		}
		n := &Node{Account: acct, Name: acct.Leaf(), Balance: NewBalance()}
		n.Depth = strings.Count(string(acct), ":")
		nodes[acct] = n
		if parent := acct.Parent(); parent != "" {
			p := nodes[parent]
			if p == nil {
				p = nodeForRecursive(nodes, &roots, parent)
			}
			p.Children = append(p.Children, n)
		} else {
			n.Name = string(acct)
			roots = append(roots, n)
		}
		return n
	}

	for _, a := range l.Accounts() {
		if len(want) > 0 && !want[a.Name.Type()] {
			continue
		}
		b := l.rangeBalance(a, r)
		if b.IsZero() && !l.hasPostings(a) {
			continue
		}
		n := nodeFor(a.Name)
		n.Balance.Merge(b)
		for cur := range b {
			currencies[cur] = true
		}
	}

	// Roll every node's balance up into its ancestors, then sort for stable output.
	for _, root := range roots {
		accumulate(root)
	}
	sortNodes(roots)
	for cur := range currencies {
		tree.Currencies = append(tree.Currencies, cur)
	}
	sort.Strings(tree.Currencies)
	sort.Slice(roots, func(i, j int) bool { return roots[i].Name < roots[j].Name })
	tree.Roots = roots
	return tree
}

// nodeForRecursive creates a missing intermediate node and links it upward.
func nodeForRecursive(nodes map[Account]*Node, roots *[]*Node, acct Account) *Node {
	if n, ok := nodes[acct]; ok {
		return n
	}
	n := &Node{Account: acct, Name: acct.Leaf(), Balance: NewBalance()}
	n.Depth = strings.Count(string(acct), ":")
	nodes[acct] = n
	if parent := acct.Parent(); parent != "" {
		p := nodeForRecursive(nodes, roots, parent)
		p.Children = append(p.Children, n)
	} else {
		n.Name = string(acct)
		*roots = append(*roots, n)
	}
	return n
}

// accumulate rolls child balances into their parent, depth-first.
func accumulate(n *Node) Balance {
	total := n.Balance.Clone()
	for _, c := range n.Children {
		total.Merge(accumulate(c))
	}
	n.Balance = total
	return total
}

func sortNodes(ns []*Node) {
	for _, n := range ns {
		sort.Slice(n.Children, func(i, j int) bool { return n.Children[i].Name < n.Children[j].Name })
		sortNodes(n.Children)
	}
}

// rangeBalance computes one account's balance for a range.
func (l *Ledger) rangeBalance(a *AccountInfo, r Range) Balance {
	b := NewBalance()
	for _, ref := range a.Postings {
		d := ref.Txn.When()
		switch {
		case r.Period:
			if d.Before(r.From) || (!r.To.IsZero() && d.After(r.To)) {
				continue
			}
		case !r.To.IsZero():
			if d.After(r.To) {
				continue
			}
		}
		b.AddAmount(*ref.Posting.Amount)
	}
	return b
}

func (l *Ledger) hasPostings(a *AccountInfo) bool { return len(a.Postings) > 0 }

// Total sums every root of a tree in one commodity, without conversion.
func (t *Tree) Total(currency string) Balance {
	b := NewBalance()
	for _, r := range t.Roots {
		if v, ok := r.Balance[currency]; ok {
			b.Add(currency, v)
		}
	}
	return b
}

// Flatten returns the tree's whole balance across every commodity.
func (t *Tree) Flatten() Balance {
	b := NewBalance()
	for _, r := range t.Roots {
		b.Merge(r.Balance)
	}
	return b
}

// Walk visits every node depth-first, parents before children.
func (t *Tree) Walk(fn func(*Node)) {
	var visit func(*Node)
	visit = func(n *Node) {
		fn(n)
		for _, c := range n.Children {
			visit(c)
		}
	}
	for _, r := range t.Roots {
		visit(r)
	}
}
