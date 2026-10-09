package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"cserver/internal/core"
)

type fakeServerReader struct {
	called bool
	root   string
}

func (f *fakeServerReader) Discover(
	_ context.Context,
	root string,
) ([]core.DiscoveredServer, error) {
	f.called = true
	f.root = root

	return []core.DiscoveredServer{
		{
			ID:   "aim",
			Name: "AIM",
		},
	}, nil
}

func TestDiscoveryService(t *testing.T) {
	root := filepath.Join(
		t.TempDir(),
		"servers",
	)

	reader := &fakeServerReader{}

	svc, err := NewServerDiscoveryService(
		root,
		reader,
	)
	if err != nil {
		t.Fatal(err)
	}

	servers, err := svc.List(context.Background())

	if err != nil ||
		len(servers) != 1 ||
		!reader.called ||
		reader.root != root {
		t.Fatalf(
			"unexpected discovery: %#v, %v",
			servers,
			err,
		)
	}
}

func TestDiscoveryServiceUnconfigured(t *testing.T) {
	svc, err := NewServerDiscoveryService(
		"",
		&fakeServerReader{},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.List(context.Background())

	if !errors.Is(
		err,
		ErrDiscoveryRootNotConfigured,
	) {
		t.Fatalf(
			"expected unconfigured root error, got %v",
			err,
		)
	}
}

func TestDiscoveryServiceRejectsInvalidSetup(t *testing.T) {
	_, err := NewServerDiscoveryService(
		"relative",
		&fakeServerReader{},
	)
	if err == nil {
		t.Fatal("expected absolute-path validation")
	}

	_, err = NewServerDiscoveryService(
		t.TempDir(),
		nil,
	)
	if err == nil {
		t.Fatal("expected missing-reader validation")
	}
}

func TestDiscoveryServiceCancelled(t *testing.T) {
	reader := &fakeServerReader{}

	svc, err := NewServerDiscoveryService(
		t.TempDir(),
		reader,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	_, err = svc.List(ctx)

	if !errors.Is(err, context.Canceled) ||
		reader.called {
		t.Fatalf(
			"expected early cancellation, got %v",
			err,
		)
	}
}
