//go:build debugx

// Fixture capture. Saves the newest article page from each of a set of feeds so
// the extractor can be evaluated repeatedly against identical bytes:
//
//	OUT=/tmp/pages go test -tags=debugx -run TestCaptureFixtures -v ./internal/extract/
package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/doug/gophics/examples/news/internal/feed"
	"github.com/doug/gophics/examples/news/internal/fetch"
)

func TestCaptureFixtures(t *testing.T) {
	out := os.Getenv("OUT")
	if out == "" {
		t.Skip("set OUT=/path/to/dir")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	feeds := map[string]string{
		"quanta":          "https://api.quantamagazine.org/feed/",
		"bbc":             "https://feeds.bbci.co.uk/news/world/rss.xml",
		"npr":             "https://feeds.npr.org/1001/rss.xml",
		"guardian":        "https://www.theguardian.com/world/rss",
		"sectionhiker":    "https://sectionhiker.com/feed/",
		"concord":         "https://www.concordmonitor.com/feed/",
		"nhbulletin":      "https://newhampshirebulletin.com/feed/",
		"lwn":             "https://lwn.net/headlines/newrss",
		"aeon":            "https://aeon.co/feed.rss",
		"nature":          "https://www.nature.com/nature.rss",
		"lostartpress":    "https://blog.lostartpress.com/feed/",
		"finewoodworking": "https://www.finewoodworking.com/feed",
		"arstechnica":     "https://feeds.arstechnica.com/arstechnica/index",
		"ieee":            "https://spectrum.ieee.org/feeds/feed.rss",
		"aljazeera":       "https://www.aljazeera.com/xml/rss/all.xml",
		"nytimes":         "https://rss.nytimes.com/services/xml/rss/nyt/HomePage.xml",
		"csmonitor":       "https://rss.csmonitor.com/feeds/all",
		"vtdigger":        "https://vtdigger.org/feed/",
		"yankee":          "https://newengland.com/feed/",
		"phys-org":        "https://phys.org/rss-feed/",
		"newyorker":       "https://www.newyorker.com/feed/everything",
		"lrb":             "https://www.lrb.co.uk/feeds/rss",
		"eos":             "https://eos.org/feed",
		"popwood":         "https://www.popularwoodworking.com/feed/",
		"thetrek":         "https://thetrek.co/feed/",
		"unionleader":     "https://www.unionleader.com/search/?f=rss&t=article&c=news&l=25&s=start_time&sd=desc",
		"valleynews":      "https://vnews.com/feed/",
		"simonwillison":   "https://simonwillison.net/atom/everything/",
	}

	client := fetch.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	manifest := map[string]string{}
	for name, furl := range feeds {
		resp, err := client.Do(ctx, fetch.Request{URL: furl})
		if err != nil {
			t.Logf("%-16s feed error: %v", name, err)
			continue
		}
		f, err := feed.Parse(resp.Body)
		if err != nil || len(f.Items) == 0 {
			t.Logf("%-16s no items (%v)", name, err)
			continue
		}
		var link string
		for _, it := range f.Items {
			if it.Link != "" {
				link = it.Link
				break
			}
		}
		if link == "" {
			t.Logf("%-16s no item link", name)
			continue
		}
		page, err := client.Do(ctx, fetch.Request{URL: link})
		if err != nil {
			t.Logf("%-16s page error: %v", name, err)
			continue
		}
		path := filepath.Join(out, name+".html")
		if err := os.WriteFile(path, page.Body, 0o644); err != nil {
			t.Fatal(err)
		}
		manifest[name] = link
		fmt.Printf("%-16s %8d bytes  %s\n", name, len(page.Body), link)
	}

	b, _ := json.MarshalIndent(manifest, "", " ")
	os.WriteFile(filepath.Join(out, "manifest.json"), b, 0o644)
	fmt.Printf("captured %d fixtures\n", len(manifest))
}

// TestScoreFixtures reports what this extractor gets from each saved fixture.
//
//	OUT=/tmp/pages go test -tags=debugx -run TestScoreFixtures -v ./internal/extract/
func TestScoreFixtures(t *testing.T) {
	out := os.Getenv("OUT")
	if out == "" {
		t.Skip("set OUT=/path/to/dir")
	}
	var manifest map[string]string
	b, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(b, &manifest)

	names := make([]string, 0, len(manifest))
	for n := range manifest {
		names = append(names, n)
	}
	sortStrings(names)

	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(out, name+".html"))
		if err != nil {
			continue
		}
		art, err := FromHTML(raw, manifest[name], DefaultOptions())
		if err != nil {
			fmt.Printf("%-16s FAIL  %v\n", name, err)
			continue
		}
		fmt.Printf("%-16s %5d words  %.60s\n", name, art.WordCount, art.Title)
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
