package tui

import (
	tea "charm.land/bubbletea/v2"

	"cserver/internal/app"
)

type screen int

const (
	screenHome screen = iota
	screenInformation
	screenConfiguration
)

var menuItems = []string{
	"Application Information",
	"Configuration",
	"Exit",
}

// Model represents the initial terminal interface state.
type Model struct {
	application app.App
	screen      screen
	cursor      int
	width       int
	height      int
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

// Update handles terminal events and menu navigation.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

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
			if m.screen == screenHome && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.screen == screenHome &&
				m.cursor < len(menuItems)-1 {
				m.cursor++
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
