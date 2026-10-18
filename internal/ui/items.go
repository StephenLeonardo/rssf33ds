package ui

import (
	"fmt"
	"time"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
	"github.com/aelhadii/rssf33ds/internal/search"
)

// feedState is everything known about one feed. It is shared by pointer with
// the feed list items, so status updates show up without rebuilding the list.
type feedState struct {
	loading   bool
	feed      *feed.Feed
	err       error // last error; with feed != nil it was served from stale cache
	source    fetch.Source
	fetchedAt time.Time
	docs      []*search.Doc // search index of feed's articles
}

type feedItem struct {
	f  catalog.Feed
	st *feedState
}

func (i feedItem) Title() string       { return i.f.Name }
func (i feedItem) FilterValue() string { return i.f.Name + " " + i.f.Category }

func (i feedItem) Description() string {
	st := i.st
	switch {
	case st.loading && st.feed == nil:
		return i.f.Category + " · loading…"
	case st.feed == nil && st.err != nil:
		return i.f.Category + " · error: " + content.Sanitize(st.err.Error())
	case st.feed == nil:
		return i.f.Category
	}
	n := len(st.feed.Articles)
	desc := fmt.Sprintf("%s · %d posts", i.f.Category, n)
	if n > 0 && !st.feed.Articles[0].Published.IsZero() {
		desc += " · latest " + ago(st.feed.Articles[0].Published)
	}
	if st.err != nil {
		desc += " · offline (cached)"
	}
	return desc
}

type articleItem struct{ a feed.Article }

func (i articleItem) Title() string       { return i.a.Title }
func (i articleItem) FilterValue() string { return i.a.Title }

func (i articleItem) Description() string {
	d := "undated"
	if !i.a.Published.IsZero() {
		d = i.a.Published.Local().Format("2006-01-02")
	}
	if i.a.Summary != "" {
		d += " · " + i.a.Summary
	}
	return d
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}
