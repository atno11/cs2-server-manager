package app

import (
	"cserver/internal/config"
	"cserver/internal/infrastructure/filesystem"
	"cserver/internal/service"
)

// App contains shared application state and services.
type App struct {
	Config    config.Config
	Info      service.ApplicationInfo
	Discovery *service.ServerDiscoveryService
}

// New initializes the application without modifying
// external infrastructure or scanning server directories.
func New() (App, error) {
	cfg, err := config.Load()
	if err != nil {
		return App{}, err
	}

	discovery, err := service.NewServerDiscoveryService(
		cfg.ServersRoot,
		filesystem.Discoverer{},
	)
	if err != nil {
		return App{}, err
	}

	return App{
		Config:    cfg,
		Info:      service.GetApplicationInfo(),
		Discovery: discovery,
	}, nil
}
