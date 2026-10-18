// Package feed parses RSS, Atom and JSON Feed documents into a flat,
// sanitized list of articles.
package feed

import (
	"bytes"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/aelhadii/rssf33ds/internal/content"
)

// Article is one entry of a feed. Title, Author and Summary are sanitized
// plain text; HTML is the raw body the feed carried (may be empty).
type Article struct {
	Title     string
	Link      string
	Author    string
	Published time.Time // zero if the feed gave no parseable date
	Summary   string
	HTML      string
}

// Feed is a parsed feed.
type Feed struct {
	Title    string
	Articles []Article // newest first
}

const summaryLen = 280

// Parse parses body, resolving relative article links against feedURL.
func Parse(body []byte, feedURL string) (*Feed, error) {
	f, err := gofeed.NewParser().Parse(bytes.NewReader(scrubXML(body)))
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(feedURL)
	if f.Link != "" {
		if u, err := url.Parse(f.Link); err == nil && u.IsAbs() {
			base = u
		}
	}

	out := &Feed{Title: content.PlainText(f.Title)}
	for _, it := range f.Items {
		if it == nil {
			continue
		}
		a := Article{
			Title: content.PlainText(it.Title),
			Link:  resolve(base, firstNonEmpty(it.Link, linkFrom(it.Links), it.GUID)),
			HTML:  firstNonEmpty(it.Content, it.Description),
		}
		if it.Author != nil {
			a.Author = content.PlainText(it.Author.Name)
		} else if len(it.Authors) > 0 && it.Authors[0] != nil {
			a.Author = content.PlainText(it.Authors[0].Name)
		}
		switch {
		case it.PublishedParsed != nil:
			a.Published = *it.PublishedParsed
		case it.UpdatedParsed != nil:
			a.Published = *it.UpdatedParsed
		}
		a.Summary = truncate(content.PlainText(firstNonEmpty(it.Description, it.Content)), summaryLen)
		if a.Title == "" {
			a.Title = truncate(a.Summary, 80)
		}
		if a.Title == "" {
			a.Title = "(untitled)"
		}
		out.Articles = append(out.Articles, a)
	}
	// Stable so undated items keep feed order; zero dates sink to the bottom.
	sort.SliceStable(out.Articles, func(i, j int) bool {
		return out.Articles[i].Published.After(out.Articles[j].Published)
	})
	return out, nil
}

// illegalCharRef matches numeric character references to C0 control
// characters, which XML 1.0 forbids even when escaped.
var illegalCharRef = regexp.MustCompile(`&#(?:[xX]0*(?:[0-8bBcCeEfF]|1[0-9a-fA-F])|0*(?:[0-8]|1[124-9]|2[0-9]|3[01]));`)

// scrubXML drops control characters that XML forbids. Real feeds contain
// them (pasted terminal output, broken CMS exports) and a strict parser
// would otherwise reject the whole feed over one bad byte.
func scrubXML(b []byte) []byte {
	b = bytes.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, b)
	return illegalCharRef.ReplaceAll(b, nil)
}

func linkFrom(links []string) string {
	for _, l := range links {
		if l != "" {
			return l
		}
	}
	return ""
}

func resolve(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "" // GUIDs like "urn:uuid:..." or tag: URIs, or no host at all
	}
	return u.String()
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
