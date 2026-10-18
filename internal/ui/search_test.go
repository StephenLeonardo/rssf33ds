package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// typeQuery types q into the focused search input and runs the debounced
// search to completion.
func typeQuery(t *testing.T, m Model, q string) Model {
	t.Helper()
	for _, r := range q {
		m, _ = send(t, m, runes(string(r)))
	}
	m, cmd := send(t, m, searchTickMsg{seq: m.searchSeq})
	if cmd == nil {
		t.Fatal("tick for the current generation should run a search")
	}
	m, _ = send(t, m, cmd())
	return m
}

func hitTitles(m Model) []string {
	var out []string
	for _, it := range m.results.Items() {
		out = append(out, it.(hitItem).h.Doc.Article.Title)
	}
	return out
}

func TestGlobalSearchFlow(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "raft", "paxos"), loaded(testFeeds[1].URL, "raftlog", "queues"))

	m, _ = send(t, m, runes("s"))
	if m.view != searchView || !m.search.Focused() || m.searchScope != "" {
		t.Fatalf("s should open a focused global search: view=%d", m.view)
	}
	m = typeQuery(t, m, "raft")
	if got := hitTitles(m); len(got) != 2 {
		t.Fatalf("hits across feeds = %q", got)
	}
	if !strings.Contains(m.View(), "2 results") || !strings.Contains(m.View(), "Beta") {
		t.Fatalf("results view missing count or blog name:\n%s", m.View())
	}

	// q while typing is text, not navigation.
	m, _ = send(t, m, runes("q"))
	if m.view != searchView || m.search.Value() != "raftq" {
		t.Fatalf("q typed into query? view=%d value=%q", m.view, m.search.Value())
	}
	m, _ = send(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.search.Value() != "raft" {
		t.Fatalf("backspace: value=%q", m.search.Value())
	}

	// Enter moves to the results, enter again opens the reader on that feed.
	m, _ = send(t, m, enter)
	if m.search.Focused() {
		t.Fatal("enter should focus the results")
	}
	m, _ = send(t, m, runes("j"))
	want := m.results.SelectedItem().(hitItem).h.Doc
	m, cmd := send(t, m, enter)
	if m.view != readerView || cmd == nil || m.article.Title != want.Article.Title || m.opts.Feeds[m.cur].URL != want.FeedURL {
		t.Fatalf("enter on hit: view=%d title=%q cur=%d", m.view, m.article.Title, m.cur)
	}

	// Back goes reader → search → feed list.
	m, _ = send(t, m, esc)
	if m.view != searchView || m.searchQuery != "raft" {
		t.Fatalf("esc from reader should return to search: view=%d", m.view)
	}
	m, _ = send(t, m, esc)
	if m.view != feedsView {
		t.Fatalf("esc from search results should go back: view=%d", m.view)
	}
}

func TestScopedSearchFromArticles(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "raft", "paxos"), loaded(testFeeds[1].URL, "raftlog"))
	m, _ = send(t, m, enter, runes("s"))
	if m.searchScope != testFeeds[0].URL {
		t.Fatalf("scope = %q", m.searchScope)
	}
	m = typeQuery(t, m, "raft")
	if got := hitTitles(m); len(got) != 1 || got[0] != "raft" {
		t.Fatalf("scoped hits = %q", got)
	}
	if strings.Contains(m.View(), "Beta") {
		t.Fatal("scoped search leaked another blog")
	}
	m, _ = send(t, m, esc)
	if m.view != articlesView || m.cur != 0 {
		t.Fatalf("esc should return to the article list: view=%d cur=%d", m.view, m.cur)
	}
}

func TestStaleSearchResultsDropped(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "raft", "paxos"), runes("s"))
	m = typeQuery(t, m, "raft")

	old := m.searchSeq
	m, _ = send(t, m, runes("x")) // starts a newer generation
	m, cmd := send(t, m, searchTickMsg{seq: old})
	if cmd != nil {
		t.Fatal("tick from an older generation must not search")
	}
	m, _ = send(t, m, searchResultsMsg{seq: old, query: "nothing"})
	if m.searchQuery != "raft" || len(m.results.Items()) != 1 {
		t.Fatalf("stale results applied: query=%q", m.searchQuery)
	}
}

func TestSearchLoadsMissingFeedsAndRefreshes(t *testing.T) {
	m := newTestModel(t)
	m, cmd := send(t, m, runes("s"))
	if cmd == nil || !m.states[testFeeds[0].URL].loading || !m.states[testFeeds[1].URL].loading {
		t.Fatal("global search should load unloaded feeds")
	}
	m = typeQuery(t, m, "raft")
	if len(m.results.Items()) != 0 || !strings.Contains(m.View(), "indexing 2 more") {
		t.Fatalf("expected no hits while indexing:\n%s", m.View())
	}
	seq := m.searchSeq
	m, cmd = send(t, m, loaded(testFeeds[1].URL, "raft"))
	if cmd == nil || m.searchSeq == seq {
		t.Fatal("a feed loading during a search should schedule a re-search")
	}
	m, cmd = send(t, m, searchTickMsg{seq: m.searchSeq})
	m, _ = send(t, m, cmd())
	if got := hitTitles(m); len(got) != 1 {
		t.Fatalf("hits after index grew = %q", got)
	}
}
