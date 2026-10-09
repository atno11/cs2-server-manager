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

// New sets up readers lazily without scanning or changing external state.
func New() (App, error) {
	cfg, err := config.Load()
	if err != nil {
		return App{}, err
	}

	var infrastructure service.InfrastructureBatchReader
	if cfg.OverlayRoot != "" || len(cfg.SystemdUnitDirectories) != 0 {
		infrastructure = filesystem.InfrastructureReader{Paths: filesystem.InfrastructurePaths{
			OverlayRoot:            cfg.OverlayRoot,
			SystemdUnitDirectories: cfg.SystemdUnitDirectories,
		}}
	}
	discovery, err := service.NewServerDiscoveryServiceWithInfrastructure(
		cfg.ServersRoot,
		filesystem.Discoverer{},
		infrastructure,
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
