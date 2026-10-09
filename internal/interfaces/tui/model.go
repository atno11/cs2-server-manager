package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"cserver/internal/app"
	"cserver/internal/core"
)

type screen int

const (
	screenHome screen = iota
	screenInformation
	screenConfiguration
	screenServers
)

var menuItems = []string{
	"Application Information",
	"Configuration",
	"Servers",
	"Exit",
}

type discoveryResultMsg struct {
	requestID uint64
	servers   []core.DiscoveredServer
	err       error
}

// Model represents the terminal interface state.
type Model struct {
	application app.App
	screen      screen
	cursor      int
	width       int
	height      int

	servers          []core.DiscoveredServer
	serverError      string
	serverLoading    bool
	serverOffset     int
	discoveryRequest uint64
}

// NewModel creates the initial TUI model.
func NewModel(application app.App) Model {
	return Model{
		application: application,
		screen:      screenHome,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// requestServers schedules discovery without blocking Update.
func (m *Model) requestServers() tea.Cmd {
	m.discoveryRequest++
	requestID := m.discoveryRequest

	m.serverLoading = true
	m.serverError = ""
	m.serverOffset = 0
	m.servers = nil

	discovery := m.application.Discovery

	return func() tea.Msg {
		if discovery == nil {
			return discoveryResultMsg{
				requestID: requestID,
				err: errors.New(
					"server discovery service is unavailable",
				),
			}
		}

		servers, err := discovery.List(
			context.Background(),
		)

		return discoveryResultMsg{
			requestID: requestID,
			servers:   servers,
			err:       err,
		}
	}
}

// Update handles terminal events and menu navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case discoveryResultMsg:
		// Ignore results from an earlier screen or refresh.
		if m.screen != screenServers ||
			msg.requestID != m.discoveryRequest {
			return m, nil
		}

		m.serverLoading = false

		if msg.err != nil {
			m.serverError = msg.err.Error()
			m.servers = nil
		} else {
			m.serverError = ""
			m.servers = msg.servers
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q":
			if m.screen == screenHome {
				return m, tea.Quit
			}

			m.screen = screenHome

		case "esc", "backspace":
			m.screen = screenHome

		case "up", "k":
			switch m.screen {
			case screenHome:
				if m.cursor > 0 {
					m.cursor--
				}
			case screenServers:
				if m.serverOffset > 0 {
					m.serverOffset--
				}
			}

		case "down", "j":
			switch m.screen {
			case screenHome:
				if m.cursor < len(menuItems)-1 {
					m.cursor++
				}
			case screenServers:
				if m.serverOffset < len(m.servers)-1 {
					m.serverOffset++
				}
			}

		case "r":
			if m.screen == screenServers {
				return m, m.requestServers()
			}

		case "enter":
			if m.screen != screenHome {
				return m, nil
			}

			switch m.cursor {
			case 0:
				m.screen = screenInformation

			case 1:
				m.screen = screenConfiguration

			case 2:
				m.screen = screenServers
				return m, m.requestServers()

			case 3:
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

// View implements the Bubble Tea v2 view interface.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true

	return view
}
