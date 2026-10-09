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

// ServerReader is the storage boundary for read-only discovery.
type ServerReader interface {
	Discover(
		context.Context,
		string,
	) ([]core.DiscoveredServer, error)
}

// ServerDiscoveryService exposes one use case to all interfaces.
type ServerDiscoveryService struct {
	root   string
	reader ServerReader
}

func NewServerDiscoveryService(
	root string,
	reader ServerReader,
) (*ServerDiscoveryService, error) {
	if reader == nil {
		return nil, errors.New(
			"server discovery reader is required",
		)
	}

	if root != "" && !filepath.IsAbs(root) {
		return nil, errors.New(
			"server discovery root must be absolute",
		)
	}

	if root != "" {
		root = filepath.Clean(root)
	}

	return &ServerDiscoveryService{
		root:   root,
		reader: reader,
	}, nil
}

// List returns an inventory without changing the discovered directories.
func (s *ServerDiscoveryService) List(
	ctx context.Context,
) ([]core.DiscoveredServer, error) {
	if s == nil {
		return nil, errors.New(
			"server discovery service is nil",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if s.root == "" {
		return nil, ErrDiscoveryRootNotConfigured
	}

	servers, err := s.reader.Discover(ctx, s.root)
	if err != nil {
		return nil, fmt.Errorf(
			"discover servers: %w",
			err,
		)
	}

	return servers, nil
}
