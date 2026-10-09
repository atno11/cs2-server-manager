package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantText string
	}{
		{
			name:     "default help",
			args:     nil,
			wantCode: 0,
			wantText: "CServer Manager",
		},
		{
			name:     "explicit help",
			args:     []string{"help"},
			wantCode: 0,
			wantText: "Usage:",
		},
		{
			name:     "version",
			args:     []string{"version"},
			wantCode: 0,
			wantText: Version,
		},
		{
			name:     "unknown command",
			args:     []string{"unknown"},
			wantCode: 2,
			wantText: "Unknown command",
		},
		{
			name:     "invalid config usage",
			args:     []string{"config"},
			wantCode: 2,
			wantText: "Usage: cserver config show",
		},
		{
			name:     "invalid API usage",
			args:     []string{"api"},
			wantCode: 2,
			wantText: "Usage: cserver api serve",
		},
		{
			name:     "invalid TUI usage",
			args:     []string{"tui", "extra"},
			wantCode: 2,
			wantText: "Usage: cserver tui",
		},
		{
			name:     "show config",
			args:     []string{"config", "show"},
			wantCode: 0,
			wantText: "servers_root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CSERVER_ROOT", "")
			t.Setenv("CSERVER_HTTP_ADDRESS", "")
			t.Setenv("CSERVER_LOG_LEVEL", "")
			t.Setenv("CSERVER_LOG_FORMAT", "")

			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := Run(
				tt.args,
				&stdout,
				&stderr,
			)

			if code != tt.wantCode {
				t.Fatalf(
					"expected exit code %d, got %d",
					tt.wantCode,
					code,
				)
			}

			output := stdout.String() + stderr.String()

			if !strings.Contains(output, tt.wantText) {
				t.Fatalf(
					"output %q does not contain %q",
					output,
					tt.wantText,
				)
			}
		})
	}
}

func TestRunInvalidConfiguration(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "relative/path")
	t.Setenv("CSERVER_HTTP_ADDRESS", "")
	t.Setenv("CSERVER_LOG_LEVEL", "")
	t.Setenv("CSERVER_LOG_FORMAT", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{"config", "show"},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"expected exit code 1, got %d",
			code,
		)
	}

	if !strings.Contains(stderr.String(), "absolute path") {
		t.Fatalf(
			"unexpected error: %s",
			stderr.String(),
		)
	}
}

func TestTUIInvalidConfiguration(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "relative/path")
	t.Setenv("CSERVER_HTTP_ADDRESS", "")
	t.Setenv("CSERVER_LOG_LEVEL", "")
	t.Setenv("CSERVER_LOG_FORMAT", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{"tui"},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"expected exit code 1, got %d",
			code,
		)
	}
}

func TestAPIInvalidConfiguration(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "")
	t.Setenv("CSERVER_HTTP_ADDRESS", "0.0.0.0:8080")
	t.Setenv("CSERVER_LOG_LEVEL", "")
	t.Setenv("CSERVER_LOG_FORMAT", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(
		[]string{"api", "serve"},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf(
			"expected exit code 1, got %d",
			code,
		)
	}

	if !strings.Contains(stderr.String(), "127.0.0.1") {
		t.Fatalf(
			"unexpected error: %s",
			stderr.String(),
		)
	}
}
