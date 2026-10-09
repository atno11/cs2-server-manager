package filesystem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"cserver/internal/core"
)

const maxArtifactSize = 256 * 1024

var composeNames = []string{
	"compose.yaml",
	"compose.yml",
	"docker-compose.yaml",
	"docker-compose.yml",
}

// Discoverer reads the configured root one directory level deep.
// It never follows child directory or configuration-file symlinks.
type Discoverer struct{}

// Discover identifies CS2 instances without changing their files.
func (Discoverer) Discover(
	ctx context.Context,
	root string,
) ([]core.DiscoveredServer, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New(
			"discovery root must be absolute",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf(
			"read discovery root: %w",
			err,
		)
	}

	servers := make([]core.DiscoveredServer, 0)

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// This also excludes symbolic links.
		if !entry.IsDir() {
			continue
		}

		dir := filepath.Join(root, entry.Name())

		server, found := inspectDirectory(
			dir,
			entry.Name(),
		)

		if found {
			servers = append(servers, server)
		}
	}

	return servers, nil
}

func inspectDirectory(
	dir string,
	id string,
) (core.DiscoveredServer, bool) {
	server := core.DiscoveredServer{
		ID:        id,
		Name:      id,
		Directory: dir,
		Status:    core.DiscoveryIncomplete,
		Evidence:  []string{},
	}

	envPath := filepath.Join(dir, ".env")

	env, envFound, envErr := readRegularFile(envPath)

	if envErr != nil {
		server.Warnings = append(
			server.Warnings,
			core.DiscoveryWarning{
				Code: "env_unreadable",
				Message: "The .env file is not a readable, " +
					"regular file within the size limit",
			},
		)
	}

	var envFields map[string]string

	if envFound {
		envFields = parseEnv(env)
		server.EnvFile = envPath
		server.Evidence = append(
			server.Evidence,
			"env",
		)
	}

	cs2Env := hasCS2Settings(envFields)

	if cs2Env {
		name := strings.TrimSpace(
			envFields["CS2_SERVERNAME"],
		)

		if name != "" &&
			len(name) <= 120 &&
			strings.IndexFunc(name, unicode.IsControl) == -1 {
			server.Name = name
		}

		rawPort := strings.TrimSpace(
			envFields["CS2_PORT"],
		)

		if rawPort != "" {
			port, err := strconv.Atoi(rawPort)

			if err != nil || port < 1 || port > 65535 {
				server.Warnings = append(
					server.Warnings,
					core.DiscoveryWarning{
						Code:    "invalid_port",
						Message: "CS2_PORT must be between 1 and 65535",
					},
				)
			} else {
				server.Port = &port
			}
		}
	}

	composeMatches := false

	for _, name := range composeNames {
		path := filepath.Join(dir, name)

		data, found, err := readRegularFile(path)

		if err != nil {
			server.Warnings = append(
				server.Warnings,
				core.DiscoveryWarning{
					Code: "compose_unreadable",
					Message: "A Compose file is not a readable, " +
						"regular file within the size limit",
				},
			)
			continue
		}

		if found {
			server.ComposeFile = path
			server.Evidence = append(
				server.Evidence,
				"compose",
			)

			composeMatches = looksLikeCS2Compose(data)
			break
		}
	}

	// Do not classify an ordinary directory as a CS2 server.
	if !cs2Env && !composeMatches {
		return core.DiscoveredServer{}, false
	}

	merged := filepath.Join(dir, "merged")

	if info, err := os.Lstat(merged); err == nil &&
		info.IsDir() {
		server.MergedDirectory = merged

		server.Evidence = append(
			server.Evidence,
			"merged_directory",
		)
	}

	if server.EnvFile != "" &&
		server.ComposeFile != "" {
		server.Status = core.DiscoveryComplete
	} else {
		server.Warnings = append(
			server.Warnings,
			core.DiscoveryWarning{
				Code: "incomplete_configuration",
				Message: "Both .env and a Compose file " +
					"are required for complete discovery",
			},
		)
	}

	return server, true
}

// readRegularFile rejects symlinks, special files and oversized files.
func readRegularFile(
	path string,
) ([]byte, bool, error) {
	info, err := os.Lstat(path)

	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	if !info.Mode().IsRegular() ||
		info.Size() > maxArtifactSize {
		return nil, false, errors.New(
			"not a permitted regular file",
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	if len(data) > maxArtifactSize {
		return nil, false, errors.New(
			"file exceeds size limit",
		)
	}

	return data, true, nil
}

// parseEnv retains only explicitly allowed public CS2 settings.
func parseEnv(data []byte) map[string]string {
	values := make(map[string]string)

	for _, line := range strings.Split(
		string(data),
		"\n",
	) {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)

		if !isKnownCS2Key(key) {
			// Never retain passwords or authentication tokens.
			continue
		}

		value = strings.TrimSpace(value)

		if len(value) >= 2 &&
			((value[0] == '"' &&
				value[len(value)-1] == '"') ||
				(value[0] == '\'' &&
					value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		values[key] = value
	}

	return values
}

func isKnownCS2Key(key string) bool {
	switch key {
	case "CS2_SERVERNAME",
		"CS2_PORT",
		"CS2_MAXPLAYERS",
		"CS2_STARTMAP",
		"CS2_GAMETYPE",
		"CS2_GAMEMODE":
		return true

	default:
		return false
	}
}

func hasCS2Settings(
	values map[string]string,
) bool {
	return len(values) > 0
}

// looksLikeCS2Compose uses conservative, explicit indicators.
// Full Compose parsing is outside this initial discovery increment.
func looksLikeCS2Compose(data []byte) bool {
	for _, line := range bytes.Split(
		data,
		[]byte{'\n'},
	) {
		value := strings.TrimSpace(
			string(line),
		)

		if strings.HasPrefix(value, "#") {
			continue
		}

		if strings.Contains(
			strings.ToLower(value),
			"joedwards32/cs2",
		) ||
			strings.Contains(
				value,
				"CS2_SERVERNAME",
			) ||
			strings.Contains(
				value,
				"CS2_STARTMAP",
			) {
			return true
		}
	}

	return false
}
