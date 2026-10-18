// Command rssf33ds is a distraction-free terminal reader for backend and
// systems engineering blogs.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/fetch"
	"github.com/aelhadii/rssf33ds/internal/ui"
)

func main() {
	style := flag.String("style", "", `reader style: "dark", "light", "dracula", "tokyo-night", "pink", "ascii" or "notty" (default: from terminal background)`)
	noPrefetch := flag.Bool("no-prefetch", false, "don't load every feed in the background at startup")
	cacheDir := flag.String("cache", fetch.DefaultDir(), "cache directory (empty disables the disk cache)")
	flag.Parse()

	// Resolve the style before the program owns the terminal: detecting the
	// background colour queries the terminal and would race Bubble Tea's
	// input reader.
	if *style == "" {
		*style = os.Getenv("GLAMOUR_STYLE")
	}
	if *style == "" {
		*style = "light"
		if lipgloss.HasDarkBackground() {
			*style = "dark"
		}
	}

	m := ui.New(ui.Options{
		Feeds:    catalog.Feeds,
		Client:   fetch.New(*cacheDir),
		Style:    *style,
		Prefetch: !*noPrefetch,
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "rssf33ds:", err)
		os.Exit(1)
	}
}
