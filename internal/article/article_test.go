package article

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

func fastClient() *fetch.Client {
	c := fetch.New("")
	c.BaseDelay = time.Millisecond
	return c
}

func TestResolvePrefersFullFeedBody(t *testing.T) {
	a := feed.Article{Title: "T", Link: "http://127.0.0.1:1/never-fetched", HTML: "<p>" + strings.Repeat("word ", 400) + "</p>"}
	r := Resolve(context.Background(), fastClient(), a)
	if r.Origin != FromFeed || r.Note != "" {
		t.Fatalf("got origin %d note %q", r.Origin, r.Note)
	}
}

func TestResolveFetchesPageForExcerpts(t *testing.T) {
	page := `<html><head><title>Post</title></head><body><nav>Menu</nav><article><h1>Post</h1>` +
		strings.Repeat("<p>Consensus requires a quorum of replicas to agree on each log entry before commit.</p>", 30) +
		`</article><footer>Footer links</footer></body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(page)) }))
	defer srv.Close()

	r := Resolve(context.Background(), fastClient(), feed.Article{Title: "Post", Link: srv.URL, HTML: "<p>Short teaser</p>"})
	if r.Origin != FromPage || !strings.Contains(r.Markdown, "quorum of replicas") {
		t.Fatalf("origin %d:\n%s", r.Origin, r.Markdown)
	}
	if strings.Contains(r.Markdown, "Menu") || strings.Contains(r.Markdown, "Footer links") || strings.HasPrefix(r.Markdown, "# Post") {
		t.Fatalf("chrome or duplicate title kept:\n%.300s", r.Markdown)
	}
}

func TestResolveFallsBackToExcerpt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "", 500) }))
	defer srv.Close()
	r := Resolve(context.Background(), fastClient(), feed.Article{Title: "T", Link: srv.URL, HTML: "<p>Teaser text</p>"})
	if r.Origin != Excerpt || r.Markdown != "Teaser text" || !strings.Contains(r.Note, "500") {
		t.Fatalf("got %+v", r)
	}
	r = Resolve(context.Background(), fastClient(), feed.Article{Title: "T", Link: srv.URL})
	if r.Origin != Empty || r.Note == "" {
		t.Fatalf("empty: got %+v", r)
	}
}
