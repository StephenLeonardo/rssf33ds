package ui

import "github.com/charmbracelet/lipgloss"

var (
	accent = lipgloss.AdaptiveColor{Light: "#5A3FC0", Dark: "#B4A2FF"}
	subtle = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#8C8C8C"}
	warn   = lipgloss.AdaptiveColor{Light: "#A15C00", Dark: "#F0B35A"}

	appStyle   = lipgloss.NewStyle().Padding(1, 2, 0, 2)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	metaStyle  = lipgloss.NewStyle().Foreground(subtle)
	linkStyle  = lipgloss.NewStyle().Foreground(subtle).Faint(true)
	noteStyle  = lipgloss.NewStyle().Foreground(warn).Italic(true)
	ruleStyle  = lipgloss.NewStyle().Foreground(subtle).Faint(true)
	errStyle   = lipgloss.NewStyle().Foreground(warn)
	barStyle   = lipgloss.NewStyle().Foreground(subtle)
	barKey     = lipgloss.NewStyle().Foreground(accent)

	listTitleStyle = lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "#5A3FC0", Dark: "#5A3FC0"}).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)
)
