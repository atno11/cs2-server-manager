package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewInitializesDiscoveryLazily(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")

	t.Setenv("CSERVER_ROOT", root)

	application, err := New()
	if err != nil {
		t.Fatalf("initialize application: %v", err)
	}

	if application.Discovery == nil {
		t.Fatal("expected initialized discovery service")
	}

	// Startup must not create or scan the configured root.
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup unexpectedly changed root: %v", err)
	}

	_, err = application.Discovery.List(context.Background())

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing-root error, got %v", err)
	}
}
