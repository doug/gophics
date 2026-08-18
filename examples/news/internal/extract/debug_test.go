//go:build debugx

// Scorer diagnostics. Point it at a saved page to see which containers the
// heuristic ranked and why:
//
//	PAGE=/tmp/page.html go test -tags=debugx -run TestDebugScore -v ./internal/extract/
package extract

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestDebugScore(t *testing.T) {
	path := os.Getenv("PAGE")
	if path == "" {
		t.Skip("set PAGE=/path/to/page.html")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	body := first(doc, "body")
	if body == nil {
		t.Fatal("no body")
	}

	fmt.Printf("BEFORE stripNoise: bodyText=%d p=%d article=%d main=%d\n",
		textLen(body), len(collect(body, "p")), len(collect(body, "article")), len(collect(body, "main")))

	onDrop = func(n *html.Node, reason string) {
		if t := textLen(n); t > 300 {
			cid := classID(n)
			if len(cid) > 70 {
				cid = cid[:70]
			}
			fmt.Printf("  DROP %-22s text=%6d <%s> %s\n", reason, t, tag(n), cid)
		}
	}
	defer func() { onDrop = nil }()

	stripNoise(body)

	ps := collect(body, "p")
	fmt.Printf("AFTER  stripNoise: bodyText=%d p=%d\n", textLen(body), len(ps))
	var seeded int
	for _, p := range ps {
		if len(textContent(p)) >= 25 {
			seeded++
		}
	}
	fmt.Printf("paragraphs long enough to seed scoring: %d\n", seeded)

	// Re-run the scoring pass with reporting.
	scores := map[*html.Node]float64{}
	var order []*html.Node
	addScore := func(n *html.Node, v float64) {
		if n == nil || tag(n) == "" {
			return
		}
		if _, seen := scores[n]; !seen {
			scores[n] = classWeight(n) + tagBonus(n)
			order = append(order, n)
		}
		scores[n] += v
	}
	for _, p := range collect(body, "p", "pre", "td", "blockquote", "article", "section", "div") {
		if n := tag(p); n == "div" || n == "section" || n == "article" {
			if hasBlockChildren(p) {
				continue
			}
		}
		text := textContent(p)
		if len(text) < 25 {
			continue
		}
		score := 1.0 + float64(strings.Count(text, ","))
		if b := len(text) / 100; b > 0 {
			if b > 3 {
				b = 3
			}
			score += float64(b)
		}
		addScore(p.Parent, score)
		if p.Parent != nil {
			addScore(p.Parent.Parent, score/2)
			if p.Parent.Parent != nil {
				addScore(p.Parent.Parent.Parent, score/3)
			}
		}
	}

	type row struct {
		n     *html.Node
		final float64
	}
	rows := make([]row, 0, len(order))
	for _, n := range order {
		rows = append(rows, row{n, scores[n] * (1 - linkDensity(n))})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].final > rows[j].final })

	fmt.Println("top candidates:")
	for i, r := range rows {
		if i >= 12 {
			break
		}
		cid := classID(r.n)
		if len(cid) > 60 {
			cid = cid[:60]
		}
		fmt.Printf("  %6.1f  raw=%6.1f  ld=%.2f  text=%6d  <%s> %s\n",
			r.final, scores[r.n], linkDensity(r.n), textLen(r.n), tag(r.n), cid)
	}

	art, err := FromHTML(raw, "https://example.test/", DefaultOptions())
	if err != nil {
		fmt.Printf("FromHTML error: %v\n", err)
		return
	}
	fmt.Printf("RESULT words=%d title=%q\n", art.WordCount, art.Title)
}
