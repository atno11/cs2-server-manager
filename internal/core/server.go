package core

import (
	"errors"
	"strings"
)

// Server represents a managed Counter-Strike 2 instance.
type Server struct {
	ID        string
	Name      string
	Directory string
}

// Validate checks the server's fundamental properties.
func (s Server) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("server ID is required")
	}

	if strings.TrimSpace(s.Name) == "" {
		return errors.New("server name is required")
	}

	if strings.TrimSpace(s.Directory) == "" {
		return errors.New("server directory is required")
	}

	return nil
}
