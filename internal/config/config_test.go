package config

import (
	"path/filepath"
	"testing"
)

func TestFromRoot(t *testing.T) {
	t.Run("empty root", func(t *testing.T) {
		cfg, err := FromRoot("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.ServersRoot != "" {
			t.Fatalf(
				"expected empty root, got %q",
				cfg.ServersRoot,
			)
		}

		if cfg.HTTPAddress != DefaultHTTPAddress {
			t.Fatalf(
				"unexpected HTTP address: %q",
				cfg.HTTPAddress,
			)
		}
	})

	t.Run("relative path", func(t *testing.T) {
		_, err := FromRoot("servers/cs2")
		if err == nil {
			t.Fatal("expected error for relative path")
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "servers")

		cfg, err := FromRoot(root)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.ServersRoot != root {
			t.Fatalf(
				"expected %q, got %q",
				root,
				cfg.ServersRoot,
			)
		}
	})
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "")
	t.Setenv("CSERVER_HTTP_ADDRESS", "")
	t.Setenv("CSERVER_LOG_LEVEL", "")
	t.Setenv("CSERVER_LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if cfg.HTTPAddress != DefaultHTTPAddress {
		t.Fatalf(
			"unexpected HTTP address: %q",
			cfg.HTTPAddress,
		)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf(
			"unexpected log level: %q",
			cfg.LogLevel,
		)
	}

	if cfg.LogFormat != DefaultLogFormat {
		t.Fatalf(
			"unexpected log format: %q",
			cfg.LogFormat,
		)
	}
}

func TestLoadOverrides(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cs2")

	t.Setenv("CSERVER_ROOT", root)
	t.Setenv("CSERVER_HTTP_ADDRESS", "127.0.0.1:9090")
	t.Setenv("CSERVER_LOG_LEVEL", "DEBUG")
	t.Setenv("CSERVER_LOG_FORMAT", "JSON")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	if cfg.ServersRoot != root {
		t.Fatalf(
			"expected root %q, got %q",
			root,
			cfg.ServersRoot,
		)
	}

	if cfg.HTTPAddress != "127.0.0.1:9090" {
		t.Fatalf(
			"unexpected HTTP address: %q",
			cfg.HTTPAddress,
		)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf(
			"unexpected log level: %q",
			cfg.LogLevel,
		)
	}

	if cfg.LogFormat != "json" {
		t.Fatalf(
			"unexpected log format: %q",
			cfg.LogFormat,
		)
	}
}

func TestLoadInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "relative root",
			key:   "CSERVER_ROOT",
			value: "relative/path",
		},
		{
			name:  "external HTTP address",
			key:   "CSERVER_HTTP_ADDRESS",
			value: "0.0.0.0:8080",
		},
		{
			name:  "invalid HTTP port",
			key:   "CSERVER_HTTP_ADDRESS",
			value: "127.0.0.1:70000",
		},
		{
			name:  "missing HTTP port",
			key:   "CSERVER_HTTP_ADDRESS",
			value: "127.0.0.1",
		},
		{
			name:  "invalid log level",
			key:   "CSERVER_LOG_LEVEL",
			value: "verbose",
		},
		{
			name:  "invalid log format",
			key:   "CSERVER_LOG_FORMAT",
			value: "xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CSERVER_ROOT", "")
			t.Setenv("CSERVER_HTTP_ADDRESS", "")
			t.Setenv("CSERVER_LOG_LEVEL", "")
			t.Setenv("CSERVER_LOG_FORMAT", "")
			t.Setenv(tt.key, tt.value)

			if _, err := Load(); err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}
