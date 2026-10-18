// Command rssf33ds is a distraction-free terminal reader for backend and
// systems engineering blogs.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/aelhadii/rssf33ds/internal/catalog"
	"github.com/aelhadii/rssf33ds/internal/fetch"
	"github.com/aelhadii/rssf33ds/internal/ui"
)

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

// buildVersion reports the release version, falling back to the module
// version that `go install …@vX.Y.Z` embeds.
func buildVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

func main() {
	style := flag.String("style", "", `reader style: "dark", "light", "dracula", "tokyo-night", "pink", "ascii" or "notty" (default: from terminal background)`)
	noPrefetch := flag.Bool("no-prefetch", false, "don't load every feed in the background at startup")
	cacheDir := flag.String("cache", fetch.DefaultDir(), "cache directory (empty disables the disk cache)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("rssf33ds", buildVersion())
		return
	}

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

	client := fetch.New(*cacheDir)
	// Keep the cache bounded. It runs alongside startup: anything it races
	// with is at worst re-fetched.
	go client.Prune(fetch.PruneAge, fetch.PruneBytes) //nolint:errcheck // best effort

	m := ui.New(ui.Options{
		Feeds:    catalog.Feeds,
		Client:   client,
		Style:    *style,
		Prefetch: !*noPrefetch,
	})
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "rssf33ds:", err)
		os.Exit(1)
	}
}
