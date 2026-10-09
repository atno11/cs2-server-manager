package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cleanInfrastructureEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CSERVER_ROOT", "CSERVER_OVERLAY_ROOT", "CSERVER_SYSTEMD_UNIT_DIRS",
		"CSERVER_HTTP_ADDRESS", "CSERVER_LOG_LEVEL", "CSERVER_LOG_FORMAT",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadInfrastructurePaths(t *testing.T) {
	cleanInfrastructureEnvironment(t)
	root := t.TempDir()
	t.Setenv("CSERVER_ROOT", root)
	t.Setenv("CSERVER_OVERLAY_ROOT", filepath.Join(root, "overlay", "..", "storage"))
	dirs := []string{filepath.Join(root, "etc", "systemd"), filepath.Join(root, "run", "systemd")}
	t.Setenv("CSERVER_SYSTEMD_UNIT_DIRS", strings.Join(dirs, string(os.PathListSeparator)))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OverlayRoot != filepath.Join(root, "storage") || len(cfg.SystemdUnitDirectories) != 2 ||
		cfg.SystemdUnitDirectories[0] != dirs[0] || cfg.SystemdUnitDirectories[1] != dirs[1] {
		t.Fatalf("unexpected infrastructure config: %#v", cfg)
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || !strings.Contains(string(encoded), `"overlay_root"`) || !strings.Contains(string(encoded), `"systemd_unit_directories"`) {
		t.Fatalf("config not serializable: %s (%v)", encoded, err)
	}
}

func TestLoadInfrastructureDefaultsAndRejections(t *testing.T) {
	cleanInfrastructureEnvironment(t)
	cfg, err := Load()
	if err != nil || cfg.OverlayRoot != "" || len(cfg.SystemdUnitDirectories) != 0 {
		t.Fatalf("unexpected defaults: %#v, %v", cfg, err)
	}
	encoded, err := json.Marshal(cfg)
	if err != nil || strings.Contains(string(encoded), "overlay_root") ||
		strings.Contains(string(encoded), "systemd_unit_directories") {
		t.Fatalf("default JSON changed: %s (%v)", encoded, err)
	}

	tests := []struct{ name, variable, value string }{
		{"relative overlay", "CSERVER_OVERLAY_ROOT", "relative"},
		{"relative unit", "CSERVER_SYSTEMD_UNIT_DIRS", "relative"},
		{"empty list component", "CSERVER_SYSTEMD_UNIT_DIRS", string(os.PathListSeparator) + t.TempDir()},
		{"trailing empty list component", "CSERVER_SYSTEMD_UNIT_DIRS", t.TempDir() + string(os.PathListSeparator)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanInfrastructureEnvironment(t)
			t.Setenv(tc.variable, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("expected invalid infrastructure path error")
			}
		})
	}
}
