package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultHTTPAddress = "127.0.0.1:8080"
	DefaultLogLevel    = "info"
	DefaultLogFormat   = "text"
)

// Config contains the application's global configuration.
type Config struct {
	ServersRoot            string   `json:"servers_root"`
	OverlayRoot            string   `json:"overlay_root,omitempty"`
	SystemdUnitDirectories []string `json:"systemd_unit_directories,omitempty"`
	HTTPAddress            string   `json:"http_address"`
	LogLevel               string   `json:"log_level"`
	LogFormat              string   `json:"log_format"`
}

// Load reads and validates environment-based configuration.
func Load() (Config, error) {
	cfg, err := FromRoot(os.Getenv("CSERVER_ROOT"))
	if err != nil {
		return Config{}, err
	}

	if raw := os.Getenv("CSERVER_OVERLAY_ROOT"); raw != "" {
		cfg.OverlayRoot = strings.TrimSpace(raw)
	}
	if raw := os.Getenv("CSERVER_SYSTEMD_UNIT_DIRS"); raw != "" {
		for _, directory := range strings.Split(raw, string(os.PathListSeparator)) {
			directory = strings.TrimSpace(directory)
			if directory == "" {
				return Config{}, errors.New("CSERVER_SYSTEMD_UNIT_DIRS contains an empty path")
			}
			cfg.SystemdUnitDirectories = append(cfg.SystemdUnitDirectories, directory)
		}
	}
	if value := os.Getenv("CSERVER_HTTP_ADDRESS"); value != "" {
		cfg.HTTPAddress = value
	}
	if value := os.Getenv("CSERVER_LOG_LEVEL"); value != "" {
		cfg.LogLevel = strings.ToLower(value)
	}
	if value := os.Getenv("CSERVER_LOG_FORMAT"); value != "" {
		cfg.LogFormat = strings.ToLower(value)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.OverlayRoot != "" {
		cfg.OverlayRoot = filepath.Clean(cfg.OverlayRoot)
	}
	for i, dir := range cfg.SystemdUnitDirectories {
		cfg.SystemdUnitDirectories[i] = filepath.Clean(dir)
	}
	return cfg, nil
}

// FromRoot creates a configuration using the supplied root.
func FromRoot(root string) (Config, error) {
	cfg := Config{
		HTTPAddress: DefaultHTTPAddress,
		LogLevel:    DefaultLogLevel,
		LogFormat:   DefaultLogFormat,
	}
	if root != "" {
		if !filepath.IsAbs(root) {
			return Config{}, errors.New("CSERVER_ROOT must be an absolute path")
		}
		cfg.ServersRoot = filepath.Clean(root)
	}
	return cfg, nil
}

// Validate checks the complete application configuration.
func (c Config) Validate() error {
	if c.ServersRoot != "" && !filepath.IsAbs(c.ServersRoot) {
		return errors.New("CSERVER_ROOT must be an absolute path")
	}
	if c.OverlayRoot != "" && !filepath.IsAbs(c.OverlayRoot) {
		return errors.New("CSERVER_OVERLAY_ROOT must be an absolute path")
	}
	for _, dir := range c.SystemdUnitDirectories {
		if dir == "" || !filepath.IsAbs(dir) {
			return errors.New("CSERVER_SYSTEMD_UNIT_DIRS must contain absolute, non-empty paths")
		}
	}
	host, port, err := net.SplitHostPort(c.HTTPAddress)
	if err != nil {
		return fmt.Errorf("invalid CSERVER_HTTP_ADDRESS: %w", err)
	}
	if host != "127.0.0.1" {
		return errors.New("CSERVER_HTTP_ADDRESS must use 127.0.0.1")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("CSERVER_HTTP_ADDRESS must use a port between 1 and 65535")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("CSERVER_LOG_LEVEL must be debug, info, warn, or error")
	}
	switch c.LogFormat {
	case "text", "json":
	default:
		return errors.New("CSERVER_LOG_FORMAT must be text or json")
	}
	return nil
}
