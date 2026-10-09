package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cserver/internal/core"
)

func TestNewWiresInfrastructureWithoutScanningOnStartup(t *testing.T) {
	root := t.TempDir()
	instances := filepath.Join(root, "instances")
	aim := filepath.Join(instances, "aim")
	for _, dir := range []string{aim, filepath.Join(root, "overlay", "aim", "upper"), filepath.Join(root, "units")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(aim, ".env"), []byte("CS2_PORT=27018\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aim, "compose.yaml"), []byte("services:\n  cs2:\n    image: joedwards32/cs2:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	where := filepath.Join(aim, "merged")
	unit := "[Mount]\nType=overlay\nWhere=" + where + "\n"
	if err := os.WriteFile(filepath.Join(root, "units", "aim.mount"), []byte(unit), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CSERVER_ROOT", instances)
	t.Setenv("CSERVER_OVERLAY_ROOT", filepath.Join(root, "overlay"))
	t.Setenv("CSERVER_SYSTEMD_UNIT_DIRS", filepath.Join(root, "units"))
	t.Setenv("CSERVER_HTTP_ADDRESS", "")
	t.Setenv("CSERVER_LOG_LEVEL", "")
	t.Setenv("CSERVER_LOG_FORMAT", "")

	application, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if application.Discovery == nil || application.Config.OverlayRoot != filepath.Join(root, "overlay") {
		t.Fatalf("incorrect application wiring: %#v", application.Config)
	}
	servers, err := application.Discovery.List(context.Background())
	if err != nil || len(servers) != 1 || servers[0].ID != "aim" || servers[0].Infrastructure == nil ||
		servers[0].Infrastructure.Systemd == nil || servers[0].Infrastructure.OverlayFS == nil ||
		servers[0].Infrastructure.Systemd.RuntimeState != core.RuntimeNotChecked {
		t.Fatalf("infrastructure not wired through app: %#v (%v)", servers, err)
	}
}
