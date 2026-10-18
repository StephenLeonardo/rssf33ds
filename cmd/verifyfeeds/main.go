// Command verifyfeeds re-checks every catalog feed (and the rejected
// candidates) live, compares the result against the prior findings in the
// graph, and appends the new findings with supersedes edges.
//
//	go run ./cmd/verifyfeeds            # feeds only
//	go run ./cmd/verifyfeeds -content   # also resolve each feed's newest article
//
// It exits 1 if any catalog feed is invalid.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/aelhadii/rssf33ds/internal/article"
	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

// candidates are feeds that were searched for but failed verification. They
// are re-probed each run so a revived feed is noticed.
var candidates = []catalog.Feed{
	{Name: "Uber Engineering", URL: "https://www.uber.com/blog/engineering/rss/"},
	{Name: "Uber Engineering (legacy)", URL: "https://eng.uber.com/feed/"},
	{Name: "Uber Blog (us/en)", URL: "https://www.uber.com/us/en/blog/feed/"},
	{Name: "LinkedIn Engineering", URL: "https://engineering.linkedin.com/blog.rss.html"},
	{Name: "DoorDash Engineering", URL: "https://careersatdoordash.com/feed/"},
	{Name: "Twitter/X Engineering", URL: "https://blog.twitter.com/engineering/en_us/blog.rss"},
	{Name: "CockroachDB Blog", URL: "https://www.cockroachlabs.com/blog/index.xml"},
	// Valid feed, but Go's TLS verification rejects stripe.com's chain on
	// some macOS trust stores; stripe.dev/blog/feed is in the catalog instead.
	{Name: "Stripe Blog (stripe.com)", URL: "https://stripe.com/blog/feed.rss"},
}

// A feed whose newest entry is older than this is flagged stale.
const staleAfter = 365 * 24 * time.Hour

func main() {
	storePath := flag.String("store", ".staged-agents/findings.json", "findings graph")
	withContent := flag.Bool("content", false, "also resolve the newest article of each feed")
	flag.Parse()

	store, err := loadStore(*storePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verifyfeeds: load store:", err)
		os.Exit(2)
	}
	prior := store.latest() // query the graph before running

	run := Run{ID: newID(), Stage: "graph", Started: time.Now().UTC(), Tool: "cmd/verifyfeeds"}
	client := fetch.New("") // no cache: every claim must come from a live fetch

	type job struct {
		f    catalog.Feed
		role string
	}
	var jobs []job
	for _, f := range catalog.Feeds {
		jobs = append(jobs, job{f, "catalog"})
	}
	for _, f := range candidates {
		jobs = append(jobs, job{f, "candidate"})
	}

	results := make([]Finding, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = probe(client, j.f, j.role, *withContent)
		}()
	}
	wg.Wait()

	failed := 0
	for i := range results {
		f := &results[i]
		f.ID, f.RunID, f.At = newID(), run.ID, time.Now().UTC()
		p, seen := prior[f.Entity]
		if seen {
			f.Supersedes = p.ID
		}
		mark := "  "
		switch {
		case !seen:
			mark = "+ "
		case p.Status != f.Status:
			mark = "! "
		}
		fmt.Printf("%s%-9s %-8s %-40s %s\n", mark, f.Role, f.Status, f.Name, f.Claim)
		if seen && p.Status != f.Status {
			fmt.Printf("    changed: %s → %s (was: %s, %s)\n", p.Status, f.Status, p.Claim, p.At.Format("2006-01-02"))
			if f.Role == "candidate" && f.Status == "valid" {
				fmt.Println("    → candidate feed is live again; consider adding it to the catalog")
			}
		}
		if f.Role == "catalog" && f.Status == "invalid" {
			failed++
		}
	}

	store.Runs = append(store.Runs, run)
	store.Findings = append(store.Findings, results...)
	if err := store.save(*storePath); err != nil {
		fmt.Fprintln(os.Stderr, "verifyfeeds: save store:", err)
		os.Exit(2)
	}

	valid := 0
	for _, f := range results {
		if f.Role == "catalog" && f.Status != "invalid" {
			valid++
		}
	}
	fmt.Printf("\nrun %s: %d/%d catalog feeds valid; %d findings appended to %s\n",
		run.ID, valid, len(catalog.Feeds), len(results), *storePath)
	if failed > 0 {
		os.Exit(1)
	}
}

func probe(c *fetch.Client, f catalog.Feed, role string, withContent bool) Finding {
	out := Finding{Entity: f.URL, Name: f.Name, Role: role, Source: Source{Kind: "http+parse"}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res, err := c.Get(ctx, f.URL, 0)
	if err != nil {
		return invalid(out, err)
	}
	parsed, err := feed.Parse(res.Body, f.URL)
	if err != nil {
		return invalid(out, fmt.Errorf("parse: %w", err))
	}
	if len(parsed.Articles) == 0 {
		return invalid(out, fmt.Errorf("feed has no entries"))
	}

	arts := parsed.Articles // newest first
	out.Source.Items = len(arts)
	out.Status = "valid"
	newest := "undated"
	if t := arts[0].Published; !t.IsZero() {
		out.Source.Newest = &t
		newest = t.Format("2006-01-02")
		if time.Since(t) > staleAfter {
			out.Status = "stale"
		}
	}
	out.Claim = fmt.Sprintf("%d entries, newest %s", len(arts), newest)

	if withContent {
		r := article.Resolve(ctx, c, arts[0])
		out.Source.ContentRun = utf8.RuneCountInString(content.Sanitize(r.Markdown))
		out.Source.ContentFrom = [...]string{"feed", "page", "excerpt", "empty"}[r.Origin]
		out.Claim += fmt.Sprintf("; newest article %d runes from %s", out.Source.ContentRun, out.Source.ContentFrom)
	}
	return out
}

func invalid(f Finding, err error) Finding {
	f.Status = "invalid"
	f.Source.Error = err.Error()
	f.Claim = err.Error()
	return f
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
