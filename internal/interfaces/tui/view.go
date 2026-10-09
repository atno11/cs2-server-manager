package tui

import (
	"fmt"
	"strings"
	"unicode"

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
	version := subtitleStyle.Render(m.application.Info.Version)

	footerText := "↑/↓ Navigate  •  Enter Select  •  Esc Back  •  q Quit"
	if m.screen == screenServers {
		footerText = "↑/↓ Scroll  •  r Refresh  •  Esc Back  •  q Back"
	}

	return strings.Join([]string{
		"",
		"  " + header + "  " + version,
		"",
		content,
		"",
		"  " + mutedStyle.Render(footerText),
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
	cfg := m.application.Config

	root := cfg.ServersRoot
	if root == "" {
		root = "Not configured"
	}

	overlayRoot := cfg.OverlayRoot
	if overlayRoot == "" {
		overlayRoot = "Not configured"
	}

	systemdDirs := "Not configured"
	if len(cfg.SystemdUnitDirectories) > 0 {
		systemdDirs = strings.Join(cfg.SystemdUnitDirectories, ", ")
	}

	return strings.Join([]string{
		"  " + subtitleStyle.Render("Configuration"),
		"",
		"  Servers Root: " + terminalSafe(root),
		"  Overlay Root: " + terminalSafe(overlayRoot),
		"  systemd Unit Directories: " + terminalSafe(systemdDirs),
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
			"  "+mutedStyle.Render(terminalSafe(m.serverError)),
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
			"  %d instance(s) discovered (configuration evidence only)",
			len(m.servers),
		),
		"",
	)

	visible := len(m.servers)
	if m.height > 0 {
		// Reserve space for the focused instance's infrastructure details.
		visible = max(1, m.height-23)
	}

	start := min(m.serverOffset, len(m.servers)-1)
	end := min(start+visible, len(m.servers))

	for index, server := range m.servers[start:end] {
		prefix := "    "
		style := normalStyle

		if index == 0 {
			prefix = "  ❯ "
			style = selectedStyle
		}

		lines = append(
			lines,
			prefix+style.Render(serverLine(server)),
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

	// The first visible instance is the focus of the detail panel.
	lines = append(lines, "")
	lines = append(lines, renderInfrastructureDetails(m.servers[start])...)

	return strings.Join(lines, "\n")
}

func serverLine(server core.DiscoveredServer) string {
	port := ""
	if server.Port != nil {
		port = fmt.Sprintf("  port %d", *server.Port)
	}

	warningCount := len(server.Warnings)
	infraStatus := ""

	if infrastructure := server.Infrastructure; infrastructure != nil {
		warningCount += len(infrastructure.Warnings)

		if infrastructure.OverlayFS != nil {
			infraStatus += "  OverlayFS: config"
		}
		if infrastructure.Systemd != nil {
			infraStatus += "  systemd: config"
		}
	}

	warnings := ""
	if warningCount > 0 {
		warnings = fmt.Sprintf("  %d warning(s)", warningCount)
	}

	return fmt.Sprintf(
		"%s (%s)  [%s]%s%s%s",
		terminalSafe(server.Name),
		terminalSafe(server.ID),
		server.Status,
		port,
		infraStatus,
		warnings,
	)
}

// renderInfrastructureDetails displays configuration evidence only.
// No runtime state is inferred from any unit or directory.
func renderInfrastructureDetails(server core.DiscoveredServer) []string {
	lines := []string{
		"  " + subtitleStyle.Render(
			"Infrastructure: "+terminalSafe(server.ID),
		),
		"  " + mutedStyle.Render(
			"Configuration only; mount and service activity not checked.",
		),
	}

	infra := server.Infrastructure
	if infra == nil {
		return append(
			lines,
			"  "+mutedStyle.Render(
				"No infrastructure metadata available.",
			),
		)
	}

	if overlay := infra.OverlayFS; overlay != nil {
		lines = append(
			lines,
			"  OverlayFS: configuration found",
			"    Source: "+terminalSafe(overlay.Source),
			"    Runtime: "+string(overlay.RuntimeState),
		)

		if len(overlay.LowerDirectories) > 0 {
			for _, directory := range overlay.LowerDirectories {
				lines = append(
					lines,
					"    Lower: "+terminalSafe(directory),
				)
			}
		}
		if overlay.UpperDirectory != "" {
			lines = append(
				lines,
				"    Upper: "+terminalSafe(overlay.UpperDirectory),
			)
		}
		if overlay.WorkDirectory != "" {
			lines = append(
				lines,
				"    Work: "+terminalSafe(overlay.WorkDirectory),
			)
		}
		if overlay.MergedDirectory != "" {
			lines = append(
				lines,
				"    Merged: "+terminalSafe(overlay.MergedDirectory),
			)
		}
	} else {
		lines = append(
			lines,
			"  "+mutedStyle.Render("OverlayFS: no metadata available"),
		)
	}

	if unit := infra.Systemd; unit != nil {
		lines = append(
			lines,
			"  systemd: mount unit configuration found",
			"    Unit: "+terminalSafe(unit.UnitName),
			"    File: "+terminalSafe(unit.UnitFile),
			"    Where: "+terminalSafe(unit.Where),
			"    Runtime: "+string(unit.RuntimeState),
		)
	} else {
		lines = append(
			lines,
			"  "+mutedStyle.Render("systemd: no metadata available"),
		)
	}

	for index, item := range infra.Warnings {
		if index >= 3 {
			lines = append(
				lines,
				fmt.Sprintf(
					"  ... %d additional infrastructure warning(s)",
					len(infra.Warnings)-index,
				),
			)
			break
		}

		lines = append(
			lines,
			"  Warning: "+terminalSafe(item.Code)+
				" — "+terminalSafe(item.Message),
		)
	}

	return lines
}

// terminalSafe prevents discovered strings from injecting terminal control
// characters or bidirectional formatting controls into the TUI.
func terminalSafe(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '�'
		}
		return r
	}, value)
}
