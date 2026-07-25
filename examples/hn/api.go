package main

import (
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
// breaks preserved, tags dropped, entities unescaped. (Rich spans with
// links are the next milestone; see PLAN.md §7.1.)
func plainText(s string) string {
	s = strings.ReplaceAll(s, "<p>", "\n\n")
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
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
