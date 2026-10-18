package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/search"
)

const (
	searchDebounce = 120 * time.Millisecond
	searchLimit    = 200
	searchHeader   = 4 // title, blank, input and info lines above the results
)

type searchTickMsg struct{ seq int }

// searchResultsMsg carries one search's hits plus what it covered, so the
// view can say how complete the answer is.
type searchResultsMsg struct {
	seq     int
	query   string
	hits    []search.Hit
	docs    int // articles searched
	indexed int // feeds that contributed articles
	total   int // feeds in scope
}

type hitItem struct {
	h        search.Hit
	showFeed bool // false when the search is scoped to one feed
}

func (i hitItem) Title() string       { return i.h.Doc.Article.Title }
func (i hitItem) FilterValue() string { return i.h.Doc.Article.Title }

func (i hitItem) Description() string {
	var parts []string
	if i.showFeed {
		parts = append(parts, i.h.Doc.FeedName)
	}
	if a := i.h.Doc.Article; !a.Published.IsZero() {
		parts = append(parts, a.Published.Local().Format("2006-01-02"))
	}
	if i.h.Snippet != "" {
		parts = append(parts, i.h.Snippet)
	}
	return strings.Join(parts, " · ")
}

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.PromptStyle = titleStyle
	ti.CharLimit = 200
	return ti
}

func newResultsList() list.Model {
	l := list.New(nil, newDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetStatusBarItemName("result", "results")
	l.DisableQuitKeybindings()
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "read")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "edit query")),
			key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
		}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser"))}
	}
	return l
}

// openSearch shows the search view over one feed (scope is its URL) or
// every feed (scope ""). The query is kept when reopening the same scope.
func (m Model) openSearch(scope string) (tea.Model, tea.Cmd) {
	m.searchFrom = m.view
	m.view = searchView
	m.status = ""
	if scope != m.searchScope {
		m.search.SetValue("")
		m.searchQuery = ""
		m.results.SetItems(nil)
	}
	m.searchScope = scope
	if scope == "" {
		m.search.Placeholder = "search every blog — e.g. kafka, \"exactly once\", cloudflare dns"
	} else {
		m.search.Placeholder = "search " + m.opts.Feeds[m.feedIndex(scope)].Name
	}
	m.search.CursorEnd()

	cmds := []tea.Cmd{m.search.Focus()}
	// Results are only as complete as the index: load whatever is missing.
	for _, f := range m.scopeFeeds() {
		if st := m.states[f.URL]; st.feed == nil && !st.loading {
			st.loading = true
			cmds = append(cmds, loadFeed(m.opts.Client, m.sem, f, feedTTL))
		}
	}
	m.searchSeq++
	cmds = append(cmds, m.searchCmd())
	return m, tea.Batch(cmds...)
}

func (m Model) scopeFeeds() []catalog.Feed {
	if m.searchScope == "" {
		return m.opts.Feeds
	}
	return []catalog.Feed{m.opts.Feeds[m.feedIndex(m.searchScope)]}
}

func (m Model) inScope(url string) bool {
	return m.searchScope == "" || m.searchScope == url
}

// scheduleSearch debounces: it starts a new generation and runs the search
// only if no newer generation has started by the time the tick fires.
func (m *Model) scheduleSearch() tea.Cmd {
	m.searchSeq++
	seq := m.searchSeq
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchTickMsg{seq: seq} })
}

// searchCmd snapshots the index on the Update loop and searches it in a
// command. Docs are immutable, so sharing them with the command is safe.
func (m Model) searchCmd() tea.Cmd {
	msg := searchResultsMsg{seq: m.searchSeq, query: strings.TrimSpace(m.search.Value())}
	var docs []*search.Doc
	for _, f := range m.scopeFeeds() {
		msg.total++
		if st := m.states[f.URL]; st.docs != nil {
			msg.indexed++
			docs = append(docs, st.docs...)
		}
	}
	msg.docs = len(docs)
	return func() tea.Msg {
		msg.hits = search.Search(docs, msg.query, searchLimit)
		return msg
	}
}

func (m Model) searchResults(msg searchResultsMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.searchSeq {
		return m, nil
	}
	items := make([]list.Item, len(msg.hits))
	for i, h := range msg.hits {
		items[i] = hitItem{h: h, showFeed: m.searchScope == ""}
	}
	cmd := m.results.SetItems(items)
	if msg.query != m.searchQuery {
		m.results.Select(0) // new query: start at the best hit
	}
	m.searchQuery = msg.query
	m.searchStats = msg
	return m, cmd
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	if m.search.Focused() {
		switch msg.String() {
		case "esc":
			return m.back()
		case "enter", "down", "tab":
			if len(m.results.Items()) > 0 {
				m.search.Blur()
			}
			return m, nil
		}
		before := m.search.Value()
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		if m.search.Value() != before {
			return m, tea.Batch(cmd, m.scheduleSearch())
		}
		return m, cmd
	}

	switch msg.String() {
	case "q", "esc":
		return m.back()
	case "/", "s", "tab":
		return m, m.search.Focus()
	case "up", "k":
		if m.results.Index() == 0 {
			return m, m.search.Focus()
		}
	case "enter":
		if it, ok := m.results.SelectedItem().(hitItem); ok {
			m.cur = m.feedIndex(it.h.Doc.FeedURL)
			return m.openArticle(it.h.Doc.Article)
		}
		return m, nil
	case "o":
		if it, ok := m.results.SelectedItem().(hitItem); ok {
			return m, openBrowser(it.h.Doc.Article.Link)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.results, cmd = m.results.Update(msg)
	return m, cmd
}

func (m Model) renderSearch() string {
	title := "Search · all blogs"
	if m.searchScope != "" {
		title = "Search · " + m.opts.Feeds[m.feedIndex(m.searchScope)].Name
	}
	head := []string{listTitleStyle.Render(title), "", m.search.View(), m.searchInfo()}

	if strings.TrimSpace(m.search.Value()) == "" {
		hint := lipgloss.JoinVertical(lipgloss.Left, "",
			metaStyle.Render("Matches titles, blog names, authors and full article text."),
			metaStyle.Render(`All words must match · "quoted phrase" · add a blog name to narrow, e.g. netflix kafka`),
			"",
			barStyle.Render("enter/tab results · esc back"),
		)
		return lipgloss.JoinVertical(lipgloss.Left, append(head, hint)...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(head, m.results.View())...)
}

func (m Model) searchInfo() string {
	s := m.searchStats
	var info string
	switch {
	case s.query == "" || s.query != strings.TrimSpace(m.search.Value()):
		info = fmt.Sprintf("%d articles indexed", s.docs)
	case len(s.hits) == searchLimit:
		info = fmt.Sprintf("top %d results", searchLimit)
	case len(s.hits) == 1:
		info = "1 result"
	default:
		info = fmt.Sprintf("%d results", len(s.hits))
	}
	if s.total > 1 {
		info += fmt.Sprintf(" · %d/%d blogs", s.indexed, s.total)
	}
	loading := 0
	for _, f := range m.scopeFeeds() {
		if m.states[f.URL].loading {
			loading++
		}
	}
	if loading > 0 {
		info += fmt.Sprintf(" · indexing %d more…", loading)
	}
	if m.status != "" {
		info += " · " + m.status
	}
	return metaStyle.MaxWidth(max(10, m.width-appStyle.GetHorizontalFrameSize())).Render(info)
}
