//go:build live

// Live extraction checks against the real internet. These are excluded from
// normal test runs because they are slow, non-deterministic and dependent on
// third parties. Run them when tuning the scorer or auditing the catalog:
//
//	go test -tags=live -v -run TestLive ./internal/extract/
//
// Each case takes the newest entry from a real feed and extracts its article,
// so the fixtures never go stale.
package extract

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/doug/gophics/examples/news/internal/feed"
	"github.com/doug/gophics/examples/news/internal/fetch"
)

func TestLiveExtraction(t *testing.T) {
	cases := []struct {
		name     string
		feedURL  string
		minWords int
		// mayLackArticle marks sources whose newest entry is sometimes a video
		// or audio segment with no prose at all. For those, a refusal to extract
		// is the correct outcome rather than a defect.
		mayLackArticle bool
	}{
		{"quanta", "https://api.quantamagazine.org/feed/", 500, false},
		{"bbc-world", "https://feeds.bbci.co.uk/news/world/rss.xml", 150, false},
		// NPR's newest entry is frequently an audio segment whose page carries
		// class="no-transcript" and holds only a few hundred characters. There is
		// genuinely no article to extract, so the bar here is deliberately low.
		{"npr", "https://feeds.npr.org/1001/rss.xml", 40, true},
		{"guardian", "https://www.theguardian.com/world/rss", 200, false},
		{"sectionhiker", "https://sectionhiker.com/feed/", 300, false},
		{"concord-monitor", "https://www.concordmonitor.com/feed/", 150, false},
		{"nh-bulletin", "https://newhampshirebulletin.com/feed/", 300, false},
		{"lwn", "https://lwn.net/headlines/newrss", 100, false},
		{"aeon", "https://aeon.co/feed.rss", 800, false},
		{"nature", "https://www.nature.com/nature.rss", 80, false},
		{"lost-art-press", "https://blog.lostartpress.com/feed/", 150, false},
		{"fine-woodworking", "https://www.finewoodworking.com/feed", 100, false},
	}

	client := fetch.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, err := client.Do(ctx, fetch.Request{URL: c.feedURL})
			if err != nil {
				t.Skipf("feed unavailable: %v", err)
			}
			f, err := feed.Parse(resp.Body)
			if err != nil {
				t.Fatalf("parse feed: %v", err)
			}
			if len(f.Items) == 0 {
				t.Skip("feed served no items")
			}

			var link string
			for _, it := range f.Items {
				if it.Link != "" {
					link = it.Link
					break
				}
			}
			if link == "" {
				t.Skip("no item link to follow")
			}

			page, err := client.Do(ctx, fetch.Request{URL: link})
			if err != nil {
				t.Skipf("article unavailable: %v", err)
			}
			art, err := FromHTML(page.Body, link, DefaultOptions())
			if err != nil {
				if c.mayLackArticle && errors.Is(err, ErrTooShort) {
					t.Skipf("no article on %s (audio or video entry): %v", link, err)
				}
				t.Fatalf("extract %s: %v", link, err)
			}

			t.Logf("%s\n  title: %s\n  byline: %s\n  words: %d\n  head: %.180s",
				link, art.Title, art.Byline, art.WordCount, art.Text)

			if art.WordCount < c.minWords {
				t.Errorf("extracted %d words, want at least %d", art.WordCount, c.minWords)
			}
			if art.Title == "" {
				t.Error("no title extracted")
			}
		})
	}
}
