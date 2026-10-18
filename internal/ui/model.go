// Package ui is the Bubble Tea program: a state machine of feeds → articles
// → reader, with a search view reachable from either list, whose network,
// indexing, search and rendering work all runs in commands, so Update never
// blocks.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/content"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
)

type view int

const (
	feedsView view = iota
	articlesView
	readerView
	searchView
)

const (
	maxMeasure   = 100 // widest reading column, in cells
	minMeasure   = 20
	maxInflight  = 8
	readerFooter = 1
)

// Options configures the program.
type Options struct {
	Feeds    []catalog.Feed
	Client   *fetch.Client
	Style    string // glamour standard style: "dark", "light", "notty", …
	Prefetch bool   // load every feed in the background at startup
}

// Model is the root Bubble Tea model.
type Model struct {
	opts   Options
	sem    chan struct{}
	states map[string]*feedState

	width, height int
	view          view

	feeds    list.Model
	articles list.Model
	reader   viewport.Model
	spin     spinner.Model
	spinning bool

	cur        int    // index into opts.Feeds of the feed being read
	listURL    string // feed whose articles the articles list holds
	readerFrom view   // where the reader returns to

	search      textinput.Model
	results     list.Model
	searchScope string // feed URL to search, or "" for every feed
	searchFrom  view   // where the search view returns to
	searchSeq   int    // generation; results from older searches are dropped
	searchQuery string // query the current results answer
	searchStats searchResultsMsg

	article  feed.Article // article open in the reader
	md, note string       // its Markdown and any fallback note
	loading  bool         // article content is being fetched
	rendered int          // wrap width of the reader's current content; 0 = none
	status   string       // transient reader status line
}

// New builds the model.
func New(opts Options) Model {
	m := Model{
		opts:   opts,
		sem:    make(chan struct{}, maxInflight),
		states: make(map[string]*feedState, len(opts.Feeds)),
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		reader: viewport.New(0, 0),
		search: newSearchInput(),
	}
	m.results = newResultsList()

	items := make([]list.Item, len(opts.Feeds))
	for i, f := range opts.Feeds {
		st := &feedState{}
		m.states[f.URL] = st
		items[i] = feedItem{f: f, st: st}
	}
	m.feeds = newList(items, "rssf33ds · backend & systems engineering", "feed", "feeds")
	m.articles = newList(nil, "", "article", "articles")
	// State changes belong here, not in Init: Bubble Tea discards the
	// model Init is called on.
	if opts.Prefetch {
		for _, st := range m.states {
			st.loading = true
		}
		m.feeds.StartSpinner()
	}
	return m
}

func newDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(accent).BorderForeground(accent)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(subtle).BorderForeground(accent)
	return d
}

func newList(items []list.Item, title, singular, plural string) list.Model {
	l := list.New(items, newDelegate(), 0, 0)
	l.Title = title
	l.Styles.Title = listTitleStyle
	l.SetStatusBarItemName(singular, plural)
	// q and esc mean "back" here, which the model handles itself. Setting
	// the bindings disabled is not enough: the list re-enables Quit after
	// every filter unless told not to.
	l.DisableQuitKeybindings()
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "search")),
			key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q/esc", "back")),
		}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
			key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
		}
	}
	return l
}

// Init starts the background prefetch.
func (m Model) Init() tea.Cmd {
	if !m.opts.Prefetch {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(m.opts.Feeds)+1)
	for _, f := range m.opts.Feeds {
		cmds = append(cmds, loadFeed(m.opts.Client, m.sem, f, feedTTL))
	}
	cmds = append(cmds, m.feeds.StartSpinner()) // only for its first tick
	return tea.Batch(cmds...)
}

// Update handles one message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.view {
		case feedsView:
			return m.updateFeeds(msg)
		case articlesView:
			return m.updateArticles(msg)
		case searchView:
			return m.updateSearch(msg)
		default:
			return m.updateReader(msg)
		}

	case feedLoadedMsg:
		return m.feedLoaded(msg)

	case searchTickMsg:
		if msg.seq != m.searchSeq {
			return m, nil // superseded by a later keystroke
		}
		return m, m.searchCmd()

	case searchResultsMsg:
		return m.searchResults(msg)

	case articleLoadedMsg:
		if m.view != readerView || msg.key != articleKey(m.article) {
			return m, nil // the user moved on; drop the stale result
		}
		m.loading, m.md, m.note = false, msg.md, msg.note
		return m, m.renderCmd()

	case renderedMsg:
		if m.view != readerView || msg.key != articleKey(m.article) || msg.width != m.measure() {
			return m, nil
		}
		m.setReaderContent(msg.out, msg.width)
		return m, nil

	case statusMsg:
		if m.view == readerView || m.view == searchView {
			m.status = string(msg)
			return m, nil
		}
		return m, m.activeList().NewStatusMessage(string(msg))

	case spinner.TickMsg:
		// Each spinner only accepts ticks carrying its own ID, so it is
		// safe to offer the tick to all three.
		var c1, c2, c3 tea.Cmd
		m.feeds, c1 = m.feeds.Update(msg)
		m.articles, c2 = m.articles.Update(msg)
		if msg.ID == m.spin.ID() {
			if m.readerBusy() {
				m.spin, c3 = m.spin.Update(msg)
			} else {
				m.spinning = false
			}
		}
		return m, tea.Batch(c1, c2, c3)
	}

	var cmd tea.Cmd
	switch m.view {
	case feedsView:
		m.feeds, cmd = m.feeds.Update(msg)
	case articlesView:
		m.articles, cmd = m.articles.Update(msg)
	case searchView:
		m.search, cmd = m.search.Update(msg) // cursor blink
	default:
		m.reader, cmd = m.reader.Update(msg)
	}
	return m, cmd
}

// readerBusy reports whether the reader is still waiting on a fetch or render.
func (m Model) readerBusy() bool {
	return m.view == readerView && m.rendered == 0
}

func (m *Model) activeList() *list.Model {
	if m.view == articlesView {
		return &m.articles
	}
	return &m.feeds
}

func (m Model) updateFeeds(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.feeds.FilterState() != list.Filtering {
		switch msg.String() {
		case "enter":
			if it, ok := m.feeds.SelectedItem().(feedItem); ok {
				return m.openFeed(it.f)
			}
			return m, nil
		case "q":
			return m, tea.Quit
		case "esc":
			if m.feeds.FilterState() == list.Unfiltered {
				return m, tea.Quit
			}
		case "r":
			return m.refreshAll()
		case "s":
			return m.openSearch("")
		}
	}
	var cmd tea.Cmd
	m.feeds, cmd = m.feeds.Update(msg)
	return m, cmd
}

func (m Model) updateArticles(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.articles.FilterState() != list.Filtering {
		switch msg.String() {
		case "enter":
			if it, ok := m.articles.SelectedItem().(articleItem); ok {
				return m.openArticle(it.a)
			}
			return m, nil
		case "q", "backspace":
			return m.back()
		case "esc":
			if m.articles.FilterState() == list.Unfiltered {
				return m.back()
			}
		case "r":
			return m.refresh(m.opts.Feeds[m.cur])
		case "s":
			return m.openSearch(m.opts.Feeds[m.cur].URL)
		case "o":
			if it, ok := m.articles.SelectedItem().(articleItem); ok {
				return m, openBrowser(it.a.Link)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.articles, cmd = m.articles.Update(msg)
	return m, cmd
}

func (m Model) updateReader(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.String() {
	case "q", "esc", "backspace":
		return m.back()
	case "g", "home":
		m.reader.GotoTop()
		return m, nil
	case "G", "end":
		m.reader.GotoBottom()
		return m, nil
	case "o":
		return m, openBrowser(m.article.Link)
	}
	var cmd tea.Cmd
	m.reader, cmd = m.reader.Update(msg)
	return m, cmd
}

func (m Model) back() (tea.Model, tea.Cmd) {
	switch m.view {
	case readerView:
		m.view = m.readerFrom
		m.loading, m.md, m.note, m.rendered, m.status = false, "", "", 0, ""
		m.reader.SetContent("")
	case searchView:
		m.view = m.searchFrom
		m.search.Blur()
		m.status = ""
		if m.view == articlesView {
			m.cur = m.feedIndex(m.listURL) // a hit may have been in another feed
		}
	case articlesView:
		m.view = feedsView
		m.articles.StopSpinner()
	}
	return m, nil
}

func (m Model) feedIndex(url string) int {
	for i := range m.opts.Feeds {
		if m.opts.Feeds[i].URL == url {
			return i
		}
	}
	return 0
}

func (m Model) openFeed(f catalog.Feed) (tea.Model, tea.Cmd) {
	m.cur = m.feedIndex(f.URL)
	m.listURL = f.URL
	m.view = articlesView
	m.articles.Title = f.Name
	m.articles.ResetFilter()
	st := m.states[f.URL]

	var cmds []tea.Cmd
	if st.feed != nil {
		cmds = append(cmds, m.articles.SetItems(articleItems(st.feed)))
	} else {
		cmds = append(cmds, m.articles.SetItems(nil))
	}
	m.articles.Select(0)

	if !st.loading && st.feed == nil {
		st.loading = true
		cmds = append(cmds, loadFeed(m.opts.Client, m.sem, f, feedTTL))
	}
	if st.loading {
		cmds = append(cmds, m.articles.StartSpinner())
	} else {
		m.articles.StopSpinner()
	}
	if st.feed != nil && st.err != nil {
		cmds = append(cmds, m.articles.NewStatusMessage(m.offlineNote(st)))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) refresh(f catalog.Feed) (tea.Model, tea.Cmd) {
	st := m.states[f.URL]
	if st.loading {
		return m, nil
	}
	st.loading = true
	return m, tea.Batch(
		loadFeed(m.opts.Client, m.sem, f, 0),
		m.articles.StartSpinner(),
		m.articles.NewStatusMessage("Refreshing…"),
	)
}

func (m Model) refreshAll() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, f := range m.opts.Feeds {
		if st := m.states[f.URL]; !st.loading {
			st.loading = true
			cmds = append(cmds, loadFeed(m.opts.Client, m.sem, f, 0))
		}
	}
	if len(cmds) == 0 {
		return m, nil
	}
	cmds = append(cmds, m.feeds.StartSpinner(), m.feeds.NewStatusMessage("Refreshing all feeds…"))
	return m, tea.Batch(cmds...)
}

func (m Model) feedLoaded(msg feedLoadedMsg) (tea.Model, tea.Cmd) {
	st, ok := m.states[msg.url]
	if !ok {
		return m, nil
	}
	st.loading = false
	st.err = msg.err
	if msg.err == nil {
		st.err = msg.res.Err // non-nil when served from stale cache
	}
	if msg.feed != nil {
		st.feed, st.source, st.fetchedAt = msg.feed, msg.res.Source, msg.res.FetchedAt
		st.docs = msg.docs
	}

	var cmds []tea.Cmd
	if !m.anyLoading() {
		m.feeds.StopSpinner()
	}
	if msg.feed != nil && m.inScope(msg.url) && (m.view == searchView || m.searchQuery != "") {
		cmds = append(cmds, m.scheduleSearch()) // newly indexed articles may match
	}
	if m.view != feedsView && m.listURL == msg.url {
		m.articles.StopSpinner()
		if msg.feed != nil {
			cmds = append(cmds, m.articles.SetItems(articleItems(msg.feed)))
		}
		switch {
		case st.feed != nil && st.err != nil:
			cmds = append(cmds, m.articles.NewStatusMessage(m.offlineNote(st)))
		case st.err != nil:
			cmds = append(cmds, m.articles.NewStatusMessage(errStyle.Render("Error: "+content.Sanitize(st.err.Error()))))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) offlineNote(st *feedState) string {
	return errStyle.Render("Offline — showing cached copy from " + st.fetchedAt.Local().Format("Jan 2 15:04"))
}

func (m Model) anyLoading() bool {
	for _, st := range m.states {
		if st.loading {
			return true
		}
	}
	return false
}

func articleItems(f *feed.Feed) []list.Item {
	items := make([]list.Item, len(f.Articles))
	for i, a := range f.Articles {
		items[i] = articleItem{a: a}
	}
	return items
}

func (m Model) openArticle(a feed.Article) (tea.Model, tea.Cmd) {
	m.readerFrom = m.view
	m.view = readerView
	m.article = a
	m.loading, m.md, m.note, m.rendered, m.status = true, "", "", 0, ""
	m.reader.SetContent("")
	m.reader.GotoTop()
	cmds := []tea.Cmd{loadArticle(m.opts.Client, a)}
	if !m.spinning {
		m.spinning = true
		cmds = append(cmds, m.spin.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) renderCmd() tea.Cmd {
	return render(m.article, m.opts.Feeds[m.cur].Name, m.md, m.note, m.opts.Style, m.measure())
}

// measure is the reading column width: the terminal width, capped so lines
// stay comfortable on wide screens.
func (m Model) measure() int {
	w := min(m.width-appStyle.GetHorizontalFrameSize(), maxMeasure)
	return max(w, minMeasure)
}

func (m *Model) setReaderContent(s string, width int) {
	first := m.rendered == 0
	pct := m.reader.ScrollPercent()
	m.reader.SetContent(s)
	m.rendered = width
	if first {
		m.reader.GotoTop()
		return
	}
	// Re-wrapped after a resize: keep the reader at the same relative spot.
	maxOff := max(0, m.reader.TotalLineCount()-m.reader.Height)
	m.reader.SetYOffset(int(pct * float64(maxOff)))
}

func (m Model) resize(w, h int) (tea.Model, tea.Cmd) {
	m.width, m.height = w, h
	fw, fh := appStyle.GetFrameSize()
	m.feeds.SetSize(w-fw, h-fh)
	m.articles.SetSize(w-fw, h-fh)
	m.results.SetSize(w-fw, max(1, h-fh-searchHeader))
	m.search.Width = max(10, w-fw-4)
	m.reader.Width = w - fw
	m.reader.Height = max(1, h-fh-readerFooter)
	if m.view == readerView && !m.loading && m.md != "" && m.rendered != m.measure() {
		return m, m.renderCmd()
	}
	return m, nil
}

// View renders the active screen.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.view {
	case feedsView:
		return appStyle.Render(m.feeds.View())
	case articlesView:
		st := m.states[m.opts.Feeds[m.cur].URL]
		switch {
		case st.feed == nil && st.loading:
			return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
				listTitleStyle.Render(m.opts.Feeds[m.cur].Name), "",
				m.articles.Styles.Spinner.Render("Loading feed…")))
		case st.feed == nil && st.err != nil:
			return appStyle.Render(m.feedError(st))
		}
		return appStyle.Render(m.articles.View())
	case searchView:
		return appStyle.Render(m.renderSearch())
	default:
		return appStyle.Render(m.readerView())
	}
}

func (m Model) feedError(st *feedState) string {
	f := m.opts.Feeds[m.cur]
	return lipgloss.JoinVertical(lipgloss.Left,
		listTitleStyle.Render(f.Name), "",
		errStyle.Width(m.measure()).Render("Could not load this feed: "+content.Sanitize(st.err.Error())), "",
		metaStyle.Render(f.URL), "",
		barStyle.Render("r retry · q back"),
	)
}

func (m Model) readerView() string {
	var body string
	if m.rendered == 0 {
		msg := "Rendering…"
		if m.loading {
			msg = "Fetching article…"
		}
		body = lipgloss.Place(m.reader.Width, m.reader.Height, lipgloss.Center, lipgloss.Center,
			m.spin.View()+" "+metaStyle.Render(msg))
	} else {
		body = m.reader.View()
	}
	return body + "\n" + m.footer()
}

func (m Model) footer() string {
	left := m.opts.Feeds[m.cur].Name
	if m.rendered != 0 {
		left += fmt.Sprintf(" · %3.0f%%", m.reader.ScrollPercent()*100)
	}
	if m.status != "" {
		left += " · " + m.status
	}
	keys := []string{"j/k", "scroll", "space/b", "page", "g/G", "top/end", "o", "browser", "q", "back"}
	var help strings.Builder
	for i := 0; i < len(keys); i += 2 {
		help.WriteString("  " + barKey.Render(keys[i]) + " " + barStyle.Render(keys[i+1]))
	}
	line := barStyle.Render(left) + help.String()
	return lipgloss.NewStyle().MaxWidth(m.reader.Width).Render(line)
}
