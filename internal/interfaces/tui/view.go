package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7AA2F7"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A9B1D6"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7DCFFF"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C0CAF5"))

	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#858CA6"))
)

func (m Model) render() string {
	var content string

	switch m.screen {
	case screenInformation:
		content = m.renderInformation()

	case screenConfiguration:
		content = m.renderConfiguration()

	default:
		content = m.renderHome()
	}

	header := titleStyle.Render(m.application.Info.Name)
	version := subtitleStyle.Render(
		m.application.Info.Version,
	)

	footer := mutedStyle.Render(
		"↑/↓ Navigate  •  Enter Select  •  Esc Back  •  q Quit",
	)

	return strings.Join([]string{
		"",
		"  " + header + "  " + version,
		"",
		content,
		"",
		"  " + footer,
		"",
	}, "\n")
}

func (m Model) renderHome() string {
	var lines []string

	lines = append(
		lines,
		"  "+subtitleStyle.Render("Main Menu"),
		"",
	)

	for index, item := range menuItems {
		cursor := "  "
		style := normalStyle

		if index == m.cursor {
			cursor = "❯ "
			style = selectedStyle
		}

		lines = append(
			lines,
			"  "+cursor+style.Render(item),
		)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderInformation() string {
	info := m.application.Info

	return strings.Join([]string{
		"  " + subtitleStyle.Render("Application Information"),
		"",
		fmt.Sprintf("  Name:     %s", info.Name),
		fmt.Sprintf("  Version:  %s", info.Version),
		"",
		"  " + mutedStyle.Render(
			"Server management is not available yet.",
		),
	}, "\n")
}

func (m Model) renderConfiguration() string {
	root := m.application.Config.ServersRoot

	if root == "" {
		root = "Not configured"
	}

	return strings.Join([]string{
		"  " + subtitleStyle.Render("Configuration"),
		"",
		fmt.Sprintf("  Servers Root: %s", root),
		"",
		"  " + mutedStyle.Render(
			"Configuration is read-only in this version.",
		),
	}, "\n")
}
