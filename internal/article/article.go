// Package article resolves the readable body of a feed entry.
package article

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

const (
	// PageTTL is how long a fetched article page is served from cache.
	PageTTL = 7 * 24 * time.Hour

	// A feed body with at least this much text is treated as the full
	// article; shorter bodies are excerpts and the page itself is fetched.
	fullTextRunes = 1500
)

// Origin says where the Markdown came from.
type Origin int

const (
	FromFeed Origin = iota // the feed carried the full text
	FromPage               // readability extraction of the article page
	Excerpt                // the page failed; the feed excerpt is shown
	Empty                  // nothing readable anywhere
)

// Result is a resolved article body.
type Result struct {
	Markdown string
	Note     string // user-facing explanation when Origin is Excerpt or Empty
	Origin   Origin
}

// Resolve returns Markdown for a: the feed's own body when it carries the
// full text, otherwise the readability-extracted web page, falling back to
// the feed excerpt when the page cannot be fetched or has less text.
func Resolve(ctx context.Context, c *fetch.Client, a feed.Article) Result {
	feedMD, _ := content.ToMarkdown(a.HTML, a.Link)
	feedLen := utf8.RuneCountInString(content.PlainText(a.HTML))
	if feedLen >= fullTextRunes || a.Link == "" {
		return finish(Result{Markdown: feedMD, Origin: FromFeed}, a)
	}

	res, err := c.Get(ctx, a.Link, PageTTL)
	if err == nil {
		var body string
		body, err = content.Extract(res.Body, a.Link)
		if err == nil {
			if utf8.RuneCountInString(content.PlainText(body)) > feedLen {
				if md, cerr := content.ToMarkdown(body, a.Link); cerr == nil && md != "" {
					return finish(Result{Markdown: md, Origin: FromPage}, a)
				}
			}
			err = errors.New("page had no more text than the feed")
		}
	}

	reason := content.Sanitize(err.Error())
	r := Result{Markdown: feedMD, Origin: Excerpt}
	if feedMD != "" {
		r.Note = "Showing the feed excerpt — full article unavailable (" + reason + "). Press o to open it in a browser."
	} else {
		r.Note = "Could not load the article (" + reason + "). Press o to open it in a browser."
	}
	return finish(r, a)
}

func finish(r Result, a feed.Article) Result {
	r.Markdown = dropTitle(r.Markdown, a.Title)
	if strings.TrimSpace(r.Markdown) == "" {
		r.Markdown = "_This entry has no readable content._"
		r.Origin = Empty
		if a.Link != "" && r.Note == "" {
			r.Note = "Press o to open it in a browser."
		}
	}
	return r
}

// dropTitle removes a leading heading that repeats the article title, which
// the reader already shows in its header.
func dropTitle(md, title string) string {
	rest := strings.TrimLeft(md, "\n")
	line, after, _ := strings.Cut(rest, "\n")
	if !strings.HasPrefix(line, "#") {
		return md
	}
	text := strings.TrimSpace(strings.TrimLeft(line, "#"))
	if !strings.EqualFold(text, strings.TrimSpace(title)) {
		return md
	}
	return strings.TrimLeft(after, "\n")
}
