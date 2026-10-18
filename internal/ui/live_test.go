//go:build live

// End-to-end smoke test against the real feeds:
//
//	go test -tags live ./internal/ui -run Live -v
package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

// drive delivers msg, then runs the resulting commands and feeds back every
// app message they produce (spinner ticks and timers are ignored) until the
// model is quiet.
func drive(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		var cmd tea.Cmd
		var tm tea.Model
		tm, cmd = m.Update(queue[0])
		m, queue = tm.(Model), queue[1:]
		queue = append(queue, collect(cmd)...)
	}
	return m
}

func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(60 * time.Second):
		return nil
	}
	switch msg := msg.(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collect(c)...)
		}
		return out
	case feedLoadedMsg, articleLoadedMsg, renderedMsg, statusMsg, searchTickMsg, searchResultsMsg:
		return []tea.Msg{msg}
	}
	return nil
}

func frame(t *testing.T, label string, m Model) string {
	v := ansi.Strip(m.View())
	t.Logf("\n===== %s =====\n%s", label, v)
	return v
}

func TestLiveReadThrough(t *testing.T) {
	m := New(Options{Feeds: catalog.Feeds, Client: fetch.New(t.TempDir()), Style: "dark"})
	m = drive(t, m, tea.WindowSizeMsg{Width: 110, Height: 36})

	for _, name := range strings.Split(envOr("LIVE_FEEDS", "Netflix TechBlog,Martin Fowler,Tailscale Blog"), ",") {
		idx := -1
		for i, f := range catalog.Feeds {
			if f.Name == name {
				idx = i
			}
		}
		if idx < 0 {
			t.Fatalf("no feed %q", name)
		}
		m.feeds.Select(idx)
		m = drive(t, m, enter)
		v := frame(t, name+" · articles", m)
		if m.view != articlesView || len(m.articles.Items()) == 0 {
			t.Fatalf("%s: no articles", name)
		}
		if strings.Contains(v, "Error") {
			t.Errorf("%s: error shown", name)
		}

		m = drive(t, m, enter)
		v = frame(t, name+" · reader", m)
		if m.rendered == 0 || len(strings.TrimSpace(v)) < 200 {
			t.Errorf("%s: reader empty", name)
		}
		for _, line := range strings.Split(v, "\n") {
			if w := ansi.StringWidth(line); w > 110 {
				t.Errorf("%s: line overflows terminal (%d cols): %q", name, w, line)
			}
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
		frame(t, name+" · reader after pgdown", m)
		m = drive(t, m, esc)
		m = drive(t, m, esc)
	}
	frame(t, "feeds", m)
}

func TestLiveSearch(t *testing.T) {
	m := New(Options{Feeds: catalog.Feeds, Client: fetch.New(t.TempDir()), Style: "dark"})
	m = drive(t, m, tea.WindowSizeMsg{Width: 110, Height: 36})

	start := time.Now()
	m = drive(t, m, runes("s")) // loads and indexes every feed
	t.Logf("loaded+indexed %d articles from %d/%d blogs in %v",
		m.searchStats.docs, m.searchStats.indexed, m.searchStats.total, time.Since(start))
	if m.searchStats.indexed < len(catalog.Feeds)-2 {
		t.Fatalf("only %d feeds indexed", m.searchStats.indexed)
	}

	for _, q := range strings.Split(envOr("LIVE_QUERIES", `kafka,"exactly once",cloudflare dns,postgres replication`), ",") {
		m.search.SetValue("")
		for _, r := range q {
			m, _ = send(t, m, runes(string(r)))
		}
		docs := m.searchStats.docs
		start := time.Now()
		m = drive(t, m, searchTickMsg{seq: m.searchSeq})
		t.Logf("query %q: %d hits over %d docs in %v", q, len(m.searchStats.hits), docs, time.Since(start))
		v := frame(t, "search "+q, m)
		if len(m.searchStats.hits) == 0 {
			t.Errorf("no hits for %q", q)
		}
		for _, line := range strings.Split(v, "\n") {
			if w := ansi.StringWidth(line); w > 110 {
				t.Errorf("line overflows (%d): %q", w, line)
			}
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
