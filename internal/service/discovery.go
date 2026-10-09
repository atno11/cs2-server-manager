package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"cserver/internal/core"
)

var ErrDiscoveryRootNotConfigured = errors.New(
	"CSERVER_ROOT is not configured",
)

// ServerReader is the storage boundary for read-only server discovery.
type ServerReader interface {
	Discover(context.Context, string) ([]core.DiscoveredServer, error)
}

// InfrastructureBatchReader enriches the inventory without scanning unit
// directories individually for each instance.
type InfrastructureBatchReader interface {
	InspectAll(
		context.Context,
		[]core.DiscoveredServer,
	) (map[string]core.InfrastructureMetadata, error)
}

// ServerDiscoveryService exposes one use case to all interfaces.
type ServerDiscoveryService struct {
	root           string
	reader         ServerReader
	infrastructure InfrastructureBatchReader
}

// NewServerDiscoveryService preserves the original construction contract.
func NewServerDiscoveryService(
	root string,
	reader ServerReader,
) (*ServerDiscoveryService, error) {
	return NewServerDiscoveryServiceWithInfrastructure(root, reader, nil)
}

// NewServerDiscoveryServiceWithInfrastructure enables optional enrichment.
func NewServerDiscoveryServiceWithInfrastructure(
	root string,
	reader ServerReader,
	infrastructure InfrastructureBatchReader,
) (*ServerDiscoveryService, error) {
	if reader == nil {
		return nil, errors.New("server discovery reader is required")
	}

	if root != "" && !filepath.IsAbs(root) {
		return nil, errors.New("server discovery root must be absolute")
	}

	if root != "" {
		root = filepath.Clean(root)
	}

	return &ServerDiscoveryService{
		root:           root,
		reader:         reader,
		infrastructure: infrastructure,
	}, nil
}

// List returns read-only server inventory with optional infrastructure metadata.
// Public JSON includes infrastructure only when metadata is available.
func (s *ServerDiscoveryService) List(
	ctx context.Context,
) ([]core.DiscoveredServer, error) {
	if s == nil {
		return nil, errors.New("server discovery service is nil")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if s.root == "" {
		return nil, ErrDiscoveryRootNotConfigured
	}

	servers, err := s.reader.Discover(ctx, s.root)
	if err != nil {
		return nil, fmt.Errorf("discover servers: %w", err)
	}

	if s.infrastructure == nil || len(servers) == 0 {
		return servers, nil
	}

	metadata, err := s.infrastructure.InspectAll(ctx, servers)
	if err != nil {
		return nil, fmt.Errorf("inspect infrastructure: %w", err)
	}

	for i := range servers {
		info, ok := metadata[servers[i].ID]
		if ok && (info.OverlayFS != nil ||
			info.Systemd != nil ||
			len(info.Warnings) > 0) {
			servers[i].Infrastructure = &info
		}
	}

	return servers, nil
}
