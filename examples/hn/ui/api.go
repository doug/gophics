package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/doug/gophics/fetch"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Item is a HackerNews item (story or comment) from the Firebase API.
type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Kids        []int  `json:"kids"`
	Time        int64  `json:"time"`
}

// API is the data source; Live hits the real Firebase endpoints, tests use
// a fixture implementation.
type API interface {
	TopStories(ctx context.Context) ([]int, error)
	Item(ctx context.Context, id int) (Item, error)
}

const apiTimeout = 15 * time.Second

type liveAPI struct{}

func newLiveAPI() *liveAPI { return &liveAPI{} }

// get fetches url and decodes the JSON into v.
//
// gophics/fetch rather than net/http directly, because this app is also built
// for the browser and net/http costs 2MB of gzipped wasm there for a socket
// path that cannot run — see that package. The timeout is the caller's
// context, so it is visible here rather than buried in a client.
func (a *liveAPI) get(ctx context.Context, url string, v any) error {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	b, err := fetch.Get(ctx, url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (a *liveAPI) TopStories(ctx context.Context) ([]int, error) {
	var ids []int
	err := a.get(ctx, "https://hacker-news.firebaseio.com/v0/topstories.json", &ids)
	return ids, err
}

func (a *liveAPI) Item(ctx context.Context, id int) (Item, error) {
	var it Item
	err := a.get(ctx, fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id), &it)
	return it, err
}

// Comment is a flattened comment with its nesting depth.
type Comment struct {
	Item
	Depth int
}

// loadComments fetches a story's comment tree, up to limit comments.
func loadComments(ctx context.Context, api API, story Item, limit int) []Comment {
	return streamComments(ctx, api, story, limit, nil)
}

// streamComments fetches breadth-first and assembles depth-first.
//
// Those are two different orders on purpose. Reading order is depth-first — a
// comment followed by its replies, indented — but *fetching* in that order
// means descending one deep reply chain before the second top-level comment is
// even requested, over 80 serial round trips. Fetching by level instead makes
// every id at one depth a single concurrent batch, so the top-level comments,
// which are what the reader actually wants first, land in one trip.
//
// onProgress, when non-nil, is called after each level with the tree as
// assembled so far, so the thread fills in from the top instead of showing a
// spinner until the last reply arrives.
func streamComments(ctx context.Context, api API, story Item, limit int, onProgress func([]Comment)) []Comment {
	loaded := map[int]Item{}
	frontier := story.Kids
	var out []Comment

	for len(frontier) > 0 && ctx.Err() == nil {
		items := fetchItems(ctx, api, frontier, nil)

		var next []int
		for _, it := range items {
			loaded[it.ID] = it
			// A comment with no text is deleted or dead; it is not shown, and
			// its replies are not pursued — matching what the depth-first walk
			// did, so a dead subtree does not cost a level of fetching.
			if it.Text != "" {
				next = append(next, it.Kids...)
			}
		}

		out = assembleComments(story, loaded, limit)
		if onProgress != nil {
			onProgress(out)
		}
		if len(out) >= limit {
			break // the tree is already longer than anything that will be shown
		}
		frontier = next
	}
	return out
}

// assembleComments walks the story depth-first over whatever has been loaded,
// which is what puts a reply directly under its parent at depth+1. Ids not yet
// fetched are simply absent, so the same walk works on a partial tree.
func assembleComments(story Item, loaded map[int]Item, limit int) []Comment {
	out := make([]Comment, 0, limit)
	var walk func(ids []int, depth int)
	walk = func(ids []int, depth int) {
		for _, id := range ids {
			if len(out) >= limit {
				return
			}
			it, ok := loaded[id]
			if !ok || it.Text == "" {
				continue
			}
			out = append(out, Comment{Item: it, Depth: depth})
			walk(it.Kids, depth+1)
		}
	}
	walk(story.Kids, 0)
	return out
}

// plainText strips HN's comment HTML down to displayable text: paragraph
// breaks preserved, tags dropped, entities unescaped.
func plainText(s string) string {
	var b strings.Builder
	for _, sp := range parseSpans(s, spanStyle{}) {
		b.WriteString(sp.Text)
	}
	return strings.TrimSpace(b.String())
}

// spanStyle carries the colors parseSpans assigns per span kind.
type spanStyle struct {
	Text, Link, Emph paint.Color
}

// parseSpans converts HN's comment HTML subset (<p>, <a href>, <i>,
// <code>/<pre>) into rich spans: links tappable, italics emphasized,
// entities unescaped, paragraph breaks preserved.
func parseSpans(s string, style spanStyle) []layout.RichSpan {
	var spans []layout.RichSpan
	var cur strings.Builder
	var href string
	emph := false

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		sp := layout.RichSpan{Text: html.UnescapeString(cur.String()), Color: style.Text}
		if href != "" {
			sp.Link, sp.Color = href, style.Link
		} else if emph {
			sp.Color = style.Emph
		}
		spans = append(spans, sp)
		cur.Reset()
	}

	for i := 0; i < len(s); {
		if s[i] != '<' {
			cur.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			break
		}
		tag := s[i+1 : i+end]
		i += end + 1
		lower := strings.ToLower(tag)
		switch {
		case lower == "p" || lower == "/p":
			flush()
			if len(spans) > 0 {
				spans = append(spans, layout.RichSpan{Text: "\n\n", Color: style.Text})
			}
		case strings.HasPrefix(lower, "a "):
			flush()
			if j := strings.Index(lower, `href="`); j >= 0 {
				rest := tag[j+6:]
				if before, _, ok := strings.Cut(rest, "\""); ok {
					href = html.UnescapeString(before)
				}
			}
		case lower == "/a":
			flush()
			href = ""
		case lower == "i" || lower == "em":
			flush()
			emph = true
		case lower == "/i" || lower == "/em":
			flush()
			emph = false
		}
	}
	flush()
	// Trim a leading paragraph break.
	if len(spans) > 0 && strings.TrimSpace(spans[0].Text) == "" {
		spans = spans[1:]
	}
	return spans
}

// domain extracts the host for the story's meta line.
func domain(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	s = strings.TrimPrefix(s, "www.")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}
