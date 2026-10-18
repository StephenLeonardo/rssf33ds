// Package search is a small in-memory full-text index over feed articles.
// Every query term must match (AND), at the start of a word, in any of the
// article's title, blog name, author, summary or body; hits are ranked by
// where the terms matched, then by date.
package search

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
)

// bodyRunes caps how much of each article body is indexed, bounding memory
// for feeds that ship hundreds of full-text entries.
const bodyRunes = 8000

// Field weights. A term in the title says far more about an article than
// one somewhere in its body.
const (
	wTitle      = 10
	wFeed       = 4
	wAuthor     = 3
	wSummary    = 3
	wBodyMax    = 5 // one point per body occurrence, up to this many
	wAllInTitle = 5
)

// Doc is one indexed article. Docs are immutable once built, so a slice of
// them can be searched from any goroutine.
type Doc struct {
	FeedURL  string
	FeedName string
	Article  feed.Article

	body                                    string // plain text, for snippets
	lTitle, lFeed, lAuthor, lSummary, lBody string // lower-cased fields
}

// NewDoc indexes a. It parses the article HTML, so call it off the UI loop.
func NewDoc(feedURL, feedName string, a feed.Article) *Doc {
	body := content.PlainText(a.HTML)
	if body == "" {
		body = a.Summary
	}
	if utf8.RuneCountInString(body) > bodyRunes {
		body = string([]rune(body)[:bodyRunes])
	}
	return &Doc{
		FeedURL:  feedURL,
		FeedName: feedName,
		Article:  a,
		body:     body,
		lTitle:   strings.ToLower(a.Title),
		lFeed:    strings.ToLower(feedName),
		lAuthor:  strings.ToLower(a.Author),
		lSummary: strings.ToLower(a.Summary),
		lBody:    strings.ToLower(body),
	}
}

// Index builds docs for every article of f.
func Index(feedURL, feedName string, f *feed.Feed) []*Doc {
	docs := make([]*Doc, len(f.Articles))
	for i, a := range f.Articles {
		docs[i] = NewDoc(feedURL, feedName, a)
	}
	return docs
}

// Hit is a matching document.
type Hit struct {
	Doc     *Doc
	Score   int
	Snippet string // body text around the first match, or the summary
}

// Terms splits a query into lower-cased terms. Double-quoted phrases stay
// one term; an unterminated quote runs to the end of the query.
func Terms(q string) []string {
	var terms []string
	q = strings.ToLower(q)
	for q != "" {
		q = strings.TrimLeftFunc(q, unicode.IsSpace)
		if q == "" {
			break
		}
		var t string
		if q[0] == '"' {
			end := strings.IndexByte(q[1:], '"')
			if end < 0 {
				t, q = q[1:], ""
			} else {
				t, q = q[1:end+1], q[end+2:]
			}
			t = strings.Join(strings.Fields(t), " ")
		} else {
			end := strings.IndexFunc(q, unicode.IsSpace)
			if end < 0 {
				end = len(q)
			}
			t, q = q[:end], q[end:]
		}
		if t != "" {
			terms = append(terms, t)
		}
	}
	return terms
}

// Search returns up to limit hits for query, best first. An empty query
// matches nothing.
func Search(docs []*Doc, query string, limit int) []Hit {
	terms := Terms(query)
	if len(terms) == 0 {
		return nil
	}
	var hits []Hit
	for _, d := range docs {
		if score, ok := d.score(terms); ok {
			hits = append(hits, Hit{Doc: d, Score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Doc.Article.Published.After(hits[j].Doc.Article.Published)
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	for i := range hits {
		hits[i].Snippet = hits[i].Doc.snippet(terms)
	}
	return hits
}

func (d *Doc) score(terms []string) (int, bool) {
	total, inTitle := 0, 0
	for _, t := range terms {
		s := 0
		if count(d.lTitle, t, 1) > 0 {
			s += wTitle
			inTitle++
		}
		if count(d.lFeed, t, 1) > 0 {
			s += wFeed
		}
		if count(d.lAuthor, t, 1) > 0 {
			s += wAuthor
		}
		if count(d.lSummary, t, 1) > 0 {
			s += wSummary
		}
		s += count(d.lBody, t, wBodyMax)
		if s == 0 {
			return 0, false // every term must match somewhere
		}
		total += s
	}
	if inTitle == len(terms) {
		total += wAllInTitle
	}
	return total, true
}

// count returns how many times term starts a word in hay, stopping at max.
func count(hay, term string, max int) int {
	n := 0
	for i := 0; n < max; {
		j := strings.Index(hay[i:], term)
		if j < 0 {
			break
		}
		pos := i + j
		if wordStart(hay, pos) {
			n++
		}
		i = pos + len(term)
	}
	return n
}

// first returns the byte offset of the first word-start match, or -1.
func first(hay, term string) int {
	for i := 0; ; {
		j := strings.Index(hay[i:], term)
		if j < 0 {
			return -1
		}
		if pos := i + j; wordStart(hay, pos) {
			return pos
		}
		i += j + len(term)
	}
}

func wordStart(s string, pos int) bool {
	if pos == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

const (
	snippetBefore = 60
	snippetAfter  = 160
)

func (d *Doc) snippet(terms []string) string {
	// Offsets in the lower-cased body only line up with the original when
	// lower-casing kept every byte length, which holds for almost all text.
	if len(d.lBody) == len(d.body) {
		pos := -1
		for _, t := range terms {
			if p := first(d.lBody, t); p >= 0 && (pos < 0 || p < pos) {
				pos = p
			}
		}
		if pos >= 0 {
			return window(d.body, pos)
		}
	}
	return d.Article.Summary
}

// window cuts s around byte offset pos, on word boundaries.
func window(s string, pos int) string {
	start := max(0, pos-snippetBefore)
	end := min(len(s), pos+snippetAfter)
	for start > 0 && !utf8.RuneStart(s[start]) {
		start--
	}
	for end < len(s) && !utf8.RuneStart(s[end]) {
		end++
	}
	out := s[start:end]
	if start > 0 {
		if i := strings.IndexByte(out, ' '); i >= 0 && i < pos-start {
			out = out[i+1:]
		}
		out = "…" + out
	}
	if end < len(s) {
		if i := strings.LastIndexByte(out, ' '); i > 0 {
			out = out[:i]
		}
		out += "…"
	}
	return out
}
