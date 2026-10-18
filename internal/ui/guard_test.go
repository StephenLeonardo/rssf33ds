package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aelhadii/rssf33ds/internal/feed"
)

func TestGuardTurnsPanicIntoMessage(t *testing.T) {
	cmd := guard(func() tea.Msg { panic("boom") }, func(err error) tea.Msg { return statusMsg(err.Error()) })
	if got := cmd(); got != statusMsg("internal error: boom") {
		t.Fatalf("got %#v", got)
	}
}

func TestPanickingFeedLoadIsSkippedAndReleasesSlot(t *testing.T) {
	sem := make(chan struct{}, 1)
	// A nil client makes the fetch panic inside the command.
	for range 2 { // a leaked semaphore slot would block the second call
		done := make(chan tea.Msg, 1)
		go func() { done <- loadFeed(nil, sem, testFeeds[0], feedTTL)() }()
		select {
		case msg := <-done:
			fl, ok := msg.(feedLoadedMsg)
			if !ok || fl.err == nil || !strings.Contains(fl.err.Error(), "internal error") {
				t.Fatalf("got %#v", msg)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("semaphore slot leaked by the panic")
		}
	}

	m := newTestModel(t)
	m, _ = send(t, m, loadFeed(nil, m.sem, testFeeds[0], feedTTL)())
	if got := feedNames(m); len(got) != 1 || got[0] != "Beta" {
		t.Fatalf("panicking feed should be skipped like any failure: %q", got)
	}
}

func TestPanickingArticleShowsNote(t *testing.T) {
	a := feed.Article{Title: "t", Link: "https://x.test/a", HTML: "<p>short</p>"}
	msg := loadArticle(nil, a)().(articleLoadedMsg) // nil client panics on the page fetch
	if msg.key != articleKey(a) || !strings.Contains(msg.note, "internal error") || msg.md == "" {
		t.Fatalf("got %+v", msg)
	}
}
