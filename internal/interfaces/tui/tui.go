package tui

import (
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"cserver/internal/app"
)

// Run starts the interactive terminal user interface.
func Run(application app.App, stdout io.Writer) error {
	program := tea.NewProgram(
		NewModel(application),
		tea.WithInput(os.Stdin),
		tea.WithOutput(stdout),
	)

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("run terminal interface: %w", err)
	}

	return nil
}
