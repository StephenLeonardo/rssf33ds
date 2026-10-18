package search

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aelhadii/rssf33ds/internal/feed"
)

func doc(feedName, title, body string, day int) *Doc {
	return NewDoc("https://"+strings.ToLower(feedName)+".test/feed", feedName, feed.Article{
		Title:     title,
		Link:      "https://x.test/" + title,
		Published: time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC),
		HTML:      "<p>" + body + "</p>",
	})
}

func titles(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Doc.Article.Title)
	}
	return out
}

func TestTerms(t *testing.T) {
	got := Terms(`  Kafka "Exactly   Once" delivery "unterminated phrase`)
	want := []string{"kafka", "exactly once", "delivery", "unterminated phrase"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Terms = %q, want %q", got, want)
	}
	if Terms("   ") != nil {
		t.Fatal("blank query should have no terms")
	}
}

func TestAllTermsMustMatch(t *testing.T) {
	docs := []*Doc{
		doc("Netflix", "Kafka at scale", "We run consumers on Flink.", 1),
		doc("Uber", "Kafka tiered storage", "Brokers offload segments.", 2),
	}
	if got := titles(Search(docs, "kafka flink", 0)); !reflect.DeepEqual(got, []string{"Kafka at scale"}) {
		t.Fatalf("got %q", got)
	}
}

func TestMatchesWordStartsOnly(t *testing.T) {
	docs := []*Doc{
		doc("A", "Ten years ago", "An algorithm for cargo.", 1),
		doc("B", "Goroutine leaks", "Debugging Go services.", 2),
	}
	if got := titles(Search(docs, "go", 0)); !reflect.DeepEqual(got, []string{"Goroutine leaks"}) {
		t.Fatalf("got %q", got)
	}
}

func TestRankingTitleBeforeBodyThenRecency(t *testing.T) {
	docs := []*Doc{
		doc("A", "Notes on storage", "Raft keeps a replicated log.", 9),
		doc("B", "Raft in practice", "Leader election details.", 1),
		doc("C", "Raft revisited", "Membership changes.", 5),
	}
	want := []string{"Raft revisited", "Raft in practice", "Notes on storage"}
	if got := titles(Search(docs, "raft", 0)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBlogNameNarrowsResults(t *testing.T) {
	docs := []*Doc{
		doc("Cloudflare", "Faster resolvers", "Our DNS edge.", 1),
		doc("Netflix", "Service discovery", "Internal DNS records.", 2),
	}
	if got := titles(Search(docs, "cloudflare dns", 0)); !reflect.DeepEqual(got, []string{"Faster resolvers"}) {
		t.Fatalf("got %q", got)
	}
}

func TestPhraseAndSnippet(t *testing.T) {
	body := strings.Repeat("filler words here ", 20) + "we guarantee exactly once delivery with idempotent producers " + strings.Repeat("more text ", 30)
	docs := []*Doc{
		doc("A", "Delivery semantics", body, 1),
		doc("B", "Once upon a time", "Exactly the right delivery truck.", 2),
	}
	hits := Search(docs, `"exactly once"`, 0)
	if len(hits) != 1 || hits[0].Doc.Article.Title != "Delivery semantics" {
		t.Fatalf("phrase hits = %q", titles(hits))
	}
	s := hits[0].Snippet
	if !strings.Contains(s, "exactly once delivery") || !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Fatalf("snippet = %q", s)
	}
}

func TestLimitAndEmpty(t *testing.T) {
	var docs []*Doc
	for i := 1; i <= 10; i++ {
		docs = append(docs, doc("A", "Cache post", "body", i))
	}
	if n := len(Search(docs, "cache", 3)); n != 3 {
		t.Fatalf("limit: got %d", n)
	}
	if Search(docs, "", 0) != nil {
		t.Fatal("empty query should match nothing")
	}
}

func TestBodyIsCapped(t *testing.T) {
	d := doc("A", "Long", strings.Repeat("x ", bodyRunes)+"needle", 1)
	if len(Search([]*Doc{d}, "needle", 0)) != 0 {
		t.Fatal("text past the index cap should not be searched")
	}
}
