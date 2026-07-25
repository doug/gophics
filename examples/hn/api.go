package main

import (
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
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
	TopStories() ([]int, error)
	Item(id int) (Item, error)
}

type liveAPI struct{ client http.Client }

func newLiveAPI() *liveAPI {
	return &liveAPI{client: http.Client{Timeout: 15 * time.Second}}
}

func (a *liveAPI) get(url string, v any) error {
	resp, err := a.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

func (a *liveAPI) TopStories() ([]int, error) {
	var ids []int
	err := a.get("https://hacker-news.firebaseio.com/v0/topstories.json", &ids)
	return ids, err
}

func (a *liveAPI) Item(id int) (Item, error) {
	var it Item
	err := a.get(fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id), &it)
	return it, err
}

// Comment is a flattened comment with its nesting depth.
type Comment struct {
	Item
	Depth int
}

// loadComments fetches the comment tree depth-first up to limit comments.
func loadComments(api API, story Item, limit int) []Comment {
	var out []Comment
	var walk func(ids []int, depth int)
	walk = func(ids []int, depth int) {
		for _, id := range ids {
			if len(out) >= limit {
				return
			}
			it, err := api.Item(id)
			if err != nil || it.Text == "" {
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
				if k := strings.IndexByte(rest, '"'); k >= 0 {
					href = html.UnescapeString(rest[:k])
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
