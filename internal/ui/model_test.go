package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
	"github.com/aelhadii/rssf33ds/internal/search"
)

var testFeeds = []catalog.Feed{
	{Name: "Alpha", URL: "https://alpha.test/feed", Category: catalog.Company},
	{Name: "Beta", URL: "https://beta.test/feed", Category: catalog.People},
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func send(t *testing.T, m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		var tm tea.Model
		tm, cmd = m.Update(msg)
		m = tm.(Model)
	}
	return m, cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func newTestModel(t *testing.T) Model {
	m := New(Options{Feeds: testFeeds, Client: fetch.New(""), Style: "notty"})
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func loaded(url string, titles ...string) feedLoadedMsg {
	f := &feed.Feed{}
	for i, title := range titles {
		f.Articles = append(f.Articles, feed.Article{
			Title:     title,
			Link:      url + "/" + title,
			Published: time.Date(2026, 10, 1+i, 0, 0, 0, 0, time.UTC),
			HTML:      "<p>" + strings.Repeat("body of "+title+" ", 300) + "</p>",
		})
	}
	name := ""
	for _, tf := range testFeeds {
		if tf.URL == url {
			name = tf.Name
		}
	}
	return feedLoadedMsg{url: url, feed: f, docs: search.Index(url, name, f), res: fetch.Result{Source: fetch.Network}}
}

func TestHelpDoesNotAdvertiseQuit(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "one"), enter, runes("/"), runes("o"), enter)
	if m.articles.FilterState() != list.FilterApplied {
		t.Fatalf("filter not applied: %v", m.articles.FilterState())
	}
	if m.articles.KeyMap.Quit.Enabled() {
		t.Fatal("list re-enabled its own quit binding after filtering")
	}
}

func TestNavigationStateMachine(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "one", "two"))

	// j moves the feed cursor; k moves it back.
	m, _ = send(t, m, runes("j"))
	if m.feeds.Index() != 1 {
		t.Fatalf("j: index = %d", m.feeds.Index())
	}
	m, _ = send(t, m, runes("k"), enter)
	if m.view != articlesView || m.cur != 0 || len(m.articles.Items()) != 2 {
		t.Fatalf("enter on feed: view=%d cur=%d items=%d", m.view, m.cur, len(m.articles.Items()))
	}

	m, cmd := send(t, m, enter)
	if m.view != readerView || !m.loading || cmd == nil {
		t.Fatalf("enter on article: view=%d loading=%v", m.view, m.loading)
	}
	if got := m.article.Title; got != "one" {
		t.Fatalf("opened %q", got)
	}

	m, _ = send(t, m, esc)
	if m.view != articlesView {
		t.Fatalf("esc in reader: view=%d", m.view)
	}
	m, _ = send(t, m, runes("q"))
	if m.view != feedsView {
		t.Fatalf("q in articles: view=%d", m.view)
	}
	if _, cmd := send(t, m, runes("q")); !isQuit(cmd) {
		t.Fatal("q on feed list should quit")
	}
}

func TestFilterKeepsQAndEsc(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "one"), enter)

	m, _ = send(t, m, runes("/"))
	if m.articles.FilterState() != list.Filtering {
		t.Fatalf("/ did not start filtering: %v", m.articles.FilterState())
	}
	m, cmd := send(t, m, runes("q"))
	if m.view != articlesView || isQuit(cmd) {
		t.Fatal("q while typing a filter must type, not go back")
	}
	m, _ = send(t, m, esc)
	if m.view != articlesView || m.articles.FilterState() != list.Unfiltered {
		t.Fatalf("esc while filtering should cancel the filter only: view=%d state=%v", m.view, m.articles.FilterState())
	}
	m, _ = send(t, m, esc)
	if m.view != feedsView {
		t.Fatal("second esc should go back")
	}
}

func TestStaleResultsAreDropped(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "one", "two"), enter, enter) // reader on "one"

	// A late result for a different article must not replace the reader.
	other := m.articles.Items()[1].(articleItem).a
	m, cmd := send(t, m, articleLoadedMsg{key: articleKey(other), md: "WRONG"})
	if cmd != nil || m.md != "" || !m.loading {
		t.Fatalf("stale article result applied: md=%q", m.md)
	}

	// A feed loading in the background must not clobber the open article list.
	m, _ = send(t, m, loaded(testFeeds[1].URL, "beta-post"))
	if got := m.articles.Items()[0].(articleItem).a.Title; got != "one" {
		t.Fatalf("other feed's items leaked into list: %q", got)
	}

	// The right result renders, and a render for an old width is dropped.
	m, cmd = send(t, m, articleLoadedMsg{key: articleKey(m.article), md: "# Hello\n\nworld"})
	if cmd == nil || m.loading {
		t.Fatal("article result should trigger a render")
	}
	if m, _ = send(t, m, renderedMsg{key: articleKey(m.article), width: 7, out: "old"}); m.rendered != 0 {
		t.Fatal("render for a stale width applied")
	}
	m, _ = send(t, m, cmd())
	if m.rendered != m.measure() || !strings.Contains(m.reader.View(), "Hello") {
		t.Fatalf("render not applied: rendered=%d view=%q", m.rendered, m.reader.View())
	}

	// Leaving the reader then receiving its late render must not resurrect it.
	m, _ = send(t, m, esc, renderedMsg{key: articleKey(m.article), width: m.measure(), out: "late"})
	if m.view != articlesView {
		t.Fatal("late render changed view")
	}
}

func TestReaderWrapsAndScrolls(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, loaded(testFeeds[0].URL, "long"), enter, enter)
	m, cmd := send(t, m, articleLoadedMsg{key: articleKey(m.article), md: strings.Repeat("word ", 2000)})
	m, _ = send(t, m, cmd())

	for _, line := range strings.Split(m.reader.View(), "\n") {
		if w := len([]rune(line)); w > m.reader.Width {
			t.Fatalf("line wider than viewport (%d > %d): %q", w, m.reader.Width, line)
		}
	}
	m, _ = send(t, m, runes("j"), runes("j"))
	if m.reader.YOffset != 2 {
		t.Fatalf("j scroll: offset=%d", m.reader.YOffset)
	}
	m, _ = send(t, m, runes("G"))
	if !m.reader.AtBottom() {
		t.Fatal("G should jump to bottom")
	}
	m, _ = send(t, m, runes("g"))
	if !m.reader.AtTop() {
		t.Fatal("g should jump to top")
	}

	// Narrowing the terminal re-wraps the article to the new width.
	m, cmd = send(t, m, tea.WindowSizeMsg{Width: 50, Height: 30})
	if cmd == nil {
		t.Fatal("resize should re-render")
	}
	m, _ = send(t, m, cmd())
	if m.rendered != m.measure() || m.measure() > 50 {
		t.Fatalf("re-render width=%d measure=%d", m.rendered, m.measure())
	}
}

func feedNames(m Model) []string {
	var out []string
	for _, it := range m.feeds.Items() {
		out = append(out, it.(feedItem).f.Name)
	}
	return out
}

func TestFailedFeedsAreSkipped(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, runes("j")) // cursor on Beta
	m, _ = send(t, m, feedLoadedMsg{url: testFeeds[0].URL, err: errString("x509: certificate is revoked")})
	if got := feedNames(m); len(got) != 1 || got[0] != "Beta" {
		t.Fatalf("failed feed should be hidden: %q", got)
	}
	if it := m.feeds.SelectedItem().(feedItem); it.f.Name != "Beta" {
		t.Fatalf("cursor moved off Beta to %q", it.f.Name)
	}
	if v := m.View(); !strings.Contains(v, "Skipped Alpha") || strings.Contains(v, "x509") {
		t.Fatalf("expected a quiet skip notice, not the error:\n%s", v)
	}

	// Search neither retries nor counts the skipped feed.
	m, _ = send(t, m, runes("s"))
	if m.states[testFeeds[0].URL].loading {
		t.Fatal("search should not retry a skipped feed")
	}
	m, _ = send(t, m, esc)

	// Refreshing retries it, and it comes back once it loads.
	m, _ = send(t, m, runes("r"))
	if !m.states[testFeeds[0].URL].loading {
		t.Fatal("r should retry skipped feeds")
	}
	m, _ = send(t, m, loaded(testFeeds[0].URL, "back"))
	if got := feedNames(m); len(got) != 2 || got[0] != "Alpha" {
		t.Fatalf("recovered feed should reappear in catalog order: %q", got)
	}
}

func TestFeedFailingWhileOpenReturnsToList(t *testing.T) {
	m := newTestModel(t)
	m, _ = send(t, m, runes("j"), enter) // Beta isn't loaded yet: it starts loading
	if m.view != articlesView || !m.states[testFeeds[1].URL].loading {
		t.Fatalf("view=%d", m.view)
	}
	m, _ = send(t, m, feedLoadedMsg{url: testFeeds[1].URL, err: errString("GET: 403 Forbidden")})
	if m.view != feedsView || len(m.feeds.Items()) != 1 {
		t.Fatalf("should be back on the list without Beta: view=%d items=%q", m.view, feedNames(m))
	}
}

func TestStaleFeedIsKeptAndFlagged(t *testing.T) {
	m := newTestModel(t)
	stale := loaded(testFeeds[0].URL, "cached")
	stale.res = fetch.Result{Source: fetch.Stale, Err: errString("offline")}
	m, _ = send(t, m, stale)
	if d := m.feeds.Items()[0].(feedItem).Description(); !strings.Contains(d, "offline") {
		t.Fatalf("stale feed not flagged: %q", d)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
