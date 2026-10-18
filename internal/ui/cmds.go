package ui

import (
	"context"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/aelhadii/rssf33ds/internal/article"
	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/feed"
	"github.com/aelhadii/rssf33ds/internal/fetch"
	"github.com/aelhadii/rssf33ds/internal/search"
)

const feedTTL = 30 * time.Minute

type feedLoadedMsg struct {
	url  string
	feed *feed.Feed
	docs []*search.Doc
	res  fetch.Result
	err  error
}

type articleLoadedMsg struct {
	key  string // see articleKey; results for any other article are dropped
	md   string
	note string
}

type renderedMsg struct {
	key   string
	width int
	out   string
}

type statusMsg string

// loadFeed fetches, parses and indexes one feed. sem bounds how many
// fetches run at once so the startup prefetch does not open dozens of
// connections.
func loadFeed(c *fetch.Client, sem chan struct{}, f catalog.Feed, maxAge time.Duration) tea.Cmd {
	return func() tea.Msg {
		sem <- struct{}{}
		defer func() { <-sem }()

		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		res, err := c.Get(ctx, f.URL, maxAge)
		if err != nil {
			return feedLoadedMsg{url: f.URL, err: err}
		}
		parsed, err := feed.Parse(res.Body, f.URL)
		if err != nil {
			return feedLoadedMsg{url: f.URL, res: res, err: err}
		}
		return feedLoadedMsg{url: f.URL, feed: parsed, docs: search.Index(f.URL, f.Name, parsed), res: res}
	}
}

// loadArticle resolves an article's body off the Update loop.
func loadArticle(c *fetch.Client, a feed.Article) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		r := article.Resolve(ctx, c, a)
		return articleLoadedMsg{key: articleKey(a), md: r.Markdown, note: r.Note}
	}
}

// header is the styled title block shown above the article body.
func header(a feed.Article, feedName, note string, width int) string {
	title := titleStyle.Width(width).Render(a.Title)
	meta := []string{feedName}
	if !a.Published.IsZero() {
		meta = append(meta, a.Published.Local().Format("Mon, 02 Jan 2006"))
	}
	if a.Author != "" {
		meta = append(meta, a.Author)
	}
	lines := []string{title, metaStyle.Width(width).Render(strings.Join(meta, " · "))}
	if a.Link != "" {
		lines = append(lines, linkStyle.Width(width).Render(a.Link))
	}
	if note != "" {
		lines = append(lines, "", noteStyle.Width(width).Render(note))
	}
	lines = append(lines, ruleStyle.Render(strings.Repeat("─", width)))
	return lipgloss.NewStyle().PaddingLeft(2).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// render word-wraps the article to width with glamour. It runs as a command
// because long articles take long enough to render to stall input.
func render(a feed.Article, feedName, md, note, style string, width int) tea.Cmd {
	return func() tea.Msg {
		body, err := glamourRender(md, style, width)
		if err != nil {
			body = lipgloss.NewStyle().Width(width).PaddingLeft(2).Render(md)
		}
		return renderedMsg{
			key:   articleKey(a),
			width: width,
			out:   header(a, feedName, note, width-4) + "\n" + strings.TrimRight(body, "\n") + "\n",
		}
	}
}

func glamourRender(md, style string, width int) (string, error) {
	r, err := glamour.NewTermRenderer(
		styleOption(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	return r.Render(md)
}

// styleOption returns the glamour style. The stock dark and light styles
// print "## " before headings; for reading prose those markers are noise,
// and the headings are already set apart by colour and weight.
func styleOption(name string) glamour.TermRendererOption {
	var cfg ansi.StyleConfig
	switch name {
	case "dark":
		cfg = styles.DarkStyleConfig
	case "light":
		cfg = styles.LightStyleConfig
	default:
		return glamour.WithStandardStyle(name)
	}
	for _, h := range []*ansi.StyleBlock{&cfg.H2, &cfg.H3, &cfg.H4, &cfg.H5, &cfg.H6} {
		h.Prefix = ""
	}
	return glamour.WithStyles(cfg)
}

// articleKey identifies an article across async messages. Links are usually unique;
// the title disambiguates entries that have none.
func articleKey(a feed.Article) string { return a.Link + "\x00" + a.Title }

// openBrowser opens an http(s) URL with the platform opener. Other schemes
// are refused: links come from untrusted feeds.
func openBrowser(link string) tea.Cmd {
	return func() tea.Msg {
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return statusMsg("Refusing to open non-http(s) link")
		}
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", u.String())
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u.String())
		default:
			cmd = exec.Command("xdg-open", u.String())
		}
		if err := cmd.Start(); err != nil {
			return statusMsg("Could not open browser: " + err.Error())
		}
		go cmd.Wait() //nolint:errcheck // reap the opener; its exit status is irrelevant
		return statusMsg("Opened in browser")
	}
}
