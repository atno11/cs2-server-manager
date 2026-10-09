package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"cserver/internal/config"
)

func TestNewLoggerText(t *testing.T) {
	var output bytes.Buffer

	cfg := config.Config{
		LogLevel:  "info",
		LogFormat: "text",
	}

	logger, err := NewLogger(cfg, &output)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	logger.Info("application started", "component", "http")

	result := output.String()

	if !strings.Contains(result, "application started") {
		t.Fatalf("missing log message: %s", result)
	}

	if !strings.Contains(result, "component=http") {
		t.Fatalf("missing structured attribute: %s", result)
	}
}

func TestNewLoggerJSON(t *testing.T) {
	var output bytes.Buffer

	cfg := config.Config{
		LogLevel:  "debug",
		LogFormat: "json",
	}

	logger, err := NewLogger(cfg, &output)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	logger.Debug("application initialized", "component", "api")

	var entry map[string]any

	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode JSON log: %v", err)
	}

	if entry["level"] != "DEBUG" {
		t.Fatalf(
			"unexpected log level: %v",
			entry["level"],
		)
	}

	if entry["msg"] != "application initialized" {
		t.Fatalf(
			"unexpected log message: %v",
			entry["msg"],
		)
	}

	if entry["component"] != "api" {
		t.Fatalf(
			"unexpected component: %v",
			entry["component"],
		)
	}
}

func TestNewLoggerLevelFiltering(t *testing.T) {
	var output bytes.Buffer

	cfg := config.Config{
		LogLevel:  "warn",
		LogFormat: "text",
	}

	logger, err := NewLogger(cfg, &output)
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	logger.Info("hidden message")
	logger.Warn("visible message")

	if strings.Contains(output.String(), "hidden message") {
		t.Fatal("info message was not filtered")
	}

	if !strings.Contains(output.String(), "visible message") {
		t.Fatal("warning message was filtered")
	}
}

func TestNewLoggerInvalidOptions(t *testing.T) {
	tests := []config.Config{
		{
			LogLevel:  "invalid",
			LogFormat: "text",
		},
		{
			LogLevel:  "info",
			LogFormat: "invalid",
		},
	}

	for _, cfg := range tests {
		var output bytes.Buffer

		_, err := NewLogger(cfg, &output)
		if err == nil {
			t.Fatalf(
				"expected error for configuration %+v",
				cfg,
			)
		}
	}
}
