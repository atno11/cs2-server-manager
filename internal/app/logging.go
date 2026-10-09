package app

import (
	"fmt"
	"io"
	"log/slog"

	"cserver/internal/config"
)

// NewLogger creates a structured application logger.
func NewLogger(cfg config.Config, output io.Writer) (*slog.Logger, error) {
	var level slog.Level

	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf(
			"unsupported log level: %q",
			cfg.LogLevel,
		)
	}

	options := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler

	switch cfg.LogFormat {
	case "text":
		handler = slog.NewTextHandler(output, options)
	case "json":
		handler = slog.NewJSONHandler(output, options)
	default:
		return nil, fmt.Errorf(
			"unsupported log format: %q",
			cfg.LogFormat,
		)
	}

	return slog.New(handler), nil
}
