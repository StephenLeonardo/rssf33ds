package content_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/aelhadii/rssf33ds/internal/article"
	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

var updateGolden = flag.Bool("update", false, "update content pipeline golden Markdown files")

type goldenFixture struct {
	Title      string `json:"title"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	HTML       string `json:"html"`
	PageStatus int    `json:"page_status,omitempty"`
}

type goldenTransport struct {
	body   string
	status int
}

func (g goldenTransport) RoundTrip(*http.Request) (*http.Response, error) {
	// A zero status means ordinary successful page content for fixtures that
	// don't need to exercise an HTTP failure.
	if g.status == 0 {
		g.status = http.StatusOK
	}
	return &http.Response{
		StatusCode: g.status,
		Status:     http.StatusText(g.status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(g.body)),
	}, nil
}

func TestGoldenResolve(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate golden fixtures")
	}
	dir := filepath.Join(filepath.Dir(source), "testdata", "golden")
	inputs, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"full-text", "readability", "code-heavy", "tables-heavy", "figures-with-captions", "sponsor-sections"} {
		want := filepath.Join(dir, category+".json")
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing fixture %s", want)
		}
	}
	for _, input := range inputs {
		name := strings.TrimSuffix(filepath.Base(input), ".json")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			var fixture goldenFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}

			// Use a stable synthetic URL so the test never makes a real request.
			link := "https://golden.example/" + name
			feedHTML := fixture.HTML
			wantOrigin := article.Origin(-1)
			client := fetch.New("")
			client.BaseDelay = time.Millisecond
			switch fixture.Kind {
			case "article":
				// Article fixtures model a short feed teaser and a separate fetched page.
				feedHTML = "<p>Short fixture teaser for " + name + ".</p>"
				client.HTTP.Transport = goldenTransport{body: fixture.HTML, status: fixture.PageStatus}
				wantOrigin = article.FromPage
			case "feed":
				feedRunes := utf8.RuneCountInString(content.PlainText(fixture.HTML))
				if feedRunes >= 1500 {
					wantOrigin = article.FromFeed
				} else {
					// A short feed uses its own HTML as the simulated page; with no
					// additional text available, Resolve should fall back to the excerpt.
					client.HTTP.Transport = goldenTransport{body: fixture.HTML, status: fixture.PageStatus}
					wantOrigin = article.Excerpt
				}
			case "excerpt":
				// Status 503 forces the page-fetch failure path while preserving a teaser.
				feedHTML = "<p>Short fixture teaser for " + name + ".</p>"
				client.MaxRetries = 0
				client.HTTP.Transport = goldenTransport{body: fixture.HTML, status: fixture.PageStatus}
				wantOrigin = article.Excerpt
			default:
				t.Fatalf("unknown fixture kind %q", fixture.Kind)
			}

			got := article.Resolve(context.Background(), client, feed.Article{
				Title: fixture.Title,
				Link:  link,
				HTML:  feedHTML,
			})
			if got.Origin != wantOrigin {
				t.Fatalf("Resolve origin = %v, want %v; markdown:\n%s", got.Origin, wantOrigin, got.Markdown)
			}

			golden := filepath.Join(dir, name+".md")
			if *updateGolden {
				// Keep this explicit so golden files change only when requested.
				if err := os.WriteFile(golden, []byte(got.Markdown+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run go test ./internal/content -run Golden -update): %v", err)
			}
			if got.Markdown+"\n" != string(want) {
				t.Errorf("Markdown differs from %s\n--- got ---\n%s\n--- want ---\n%s", golden, got.Markdown, want)
			}
		})
	}
}
