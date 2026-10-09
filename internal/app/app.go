package app

import (
	"cserver/internal/config"
	"cserver/internal/service"
)

// App contains shared application state.
type App struct {
	Config config.Config
	Info   service.ApplicationInfo
}

// New initializes the application without modifying
// external infrastructure.
func New() (App, error) {
	cfg, err := config.Load()
	if err != nil {
		return App{}, err
	}

	return App{
		Config: cfg,
		Info:   service.GetApplicationInfo(),
	}, nil
}
