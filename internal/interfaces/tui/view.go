package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"cserver/internal/core"
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

	case screenServers:
		content = m.renderServers()

	default:
		content = m.renderHome()
	}

	header := titleStyle.Render(m.application.Info.Name)
	version := subtitleStyle.Render(
		m.application.Info.Version,
	)

	footerText := "↑/↓ Navigate  •  Enter Select  •  Esc Back  •  q Quit"

	if m.screen == screenServers {
		footerText = "↑/↓ Scroll  •  r Refresh  •  Esc Back  •  q Back"
	}

	footer := mutedStyle.Render(footerText)

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
			"Server lifecycle operations are not available yet.",
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

func (m Model) renderServers() string {
	lines := []string{
		"  " + subtitleStyle.Render("Discovered Servers"),
		"",
	}

	if m.serverLoading {
		return strings.Join(append(
			lines,
			"  "+mutedStyle.Render("Discovering servers..."),
		), "\n")
	}

	if m.serverError != "" {
		return strings.Join(append(
			lines,
			"  "+normalStyle.Render("Discovery failed"),
			"  "+mutedStyle.Render(m.serverError),
			"",
			"  "+mutedStyle.Render("Press r to retry."),
		), "\n")
	}

	if len(m.servers) == 0 {
		return strings.Join(append(
			lines,
			"  "+mutedStyle.Render("No CS2 instances found."),
			"",
			"  "+mutedStyle.Render("Press r to refresh."),
		), "\n")
	}

	lines = append(
		lines,
		fmt.Sprintf(
			"  %d instance(s) discovered (filesystem only)",
			len(m.servers),
		),
		"",
	)

	visible := len(m.servers)

	if m.height > 0 {
		visible = max(1, m.height-11)
	}

	start := min(m.serverOffset, len(m.servers)-1)
	end := min(start+visible, len(m.servers))

	for _, server := range m.servers[start:end] {
		lines = append(
			lines,
			"  "+normalStyle.Render(serverLine(server)),
		)
	}

	if end < len(m.servers) {
		lines = append(
			lines,
			"",
			"  "+mutedStyle.Render(
				fmt.Sprintf(
					"%d more instance(s) below",
					len(m.servers)-end,
				),
			),
		)
	}

	return strings.Join(lines, "\n")
}

func serverLine(server core.DiscoveredServer) string {
	port := ""

	if server.Port != nil {
		port = fmt.Sprintf("  port %d", *server.Port)
	}

	warnings := ""

	if count := len(server.Warnings); count > 0 {
		warnings = fmt.Sprintf("  %d warning(s)", count)
	}

	return fmt.Sprintf(
		"%s (%s)  [%s]%s%s",
		server.Name,
		server.ID,
		server.Status,
		port,
		warnings,
	)
}
