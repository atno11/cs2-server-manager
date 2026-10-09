package app

import (
	"path/filepath"
	"testing"

	"cserver/internal/service"
)

func TestNew(t *testing.T) {
	root := filepath.Join(t.TempDir(), "servers")
	t.Setenv("CSERVER_ROOT", root)

	application, err := New()
	if err != nil {
		t.Fatalf("initialize application: %v", err)
	}

	if application.Config.ServersRoot != root {
		t.Fatalf(
			"expected root %q, got %q",
			root,
			application.Config.ServersRoot,
		)
	}

	if application.Info != service.GetApplicationInfo() {
		t.Fatalf(
			"unexpected application info: %+v",
			application.Info,
		)
	}
}

func TestNewInvalidConfiguration(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "relative/path")

	_, err := New()
	if err == nil {
		t.Fatal("expected configuration error")
	}
}
