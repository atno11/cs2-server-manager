package filesystem

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"cserver/internal/core"
)

const maxUnitSize int64 = 256 * 1024

// InfrastructurePaths are explicit, trusted scan locations. No host-wide
// systemd or mount search takes place by default.
type InfrastructurePaths struct {
	OverlayRoot            string
	SystemdUnitDirectories []string
}

func (p InfrastructurePaths) Validate() error {
	if p.OverlayRoot != "" && !filepath.IsAbs(p.OverlayRoot) {
		return errors.New("overlay root must be absolute")
	}
	for _, directory := range p.SystemdUnitDirectories {
		if !filepath.IsAbs(directory) {
			return errors.New("systemd unit directories must be absolute")
		}
	}
	return nil
}

// InfrastructureReader reads external OverlayFS and systemd configuration.
// It never invokes systemctl, mount, Docker or other lifecycle commands.
type InfrastructureReader struct {
	Paths InfrastructurePaths
}

// Inspect associates infrastructure evidence with one already-discovered CS2
// instance. ID must be a single directory name from the existing discoverer.
func (r InfrastructureReader) Inspect(
	ctx context.Context,
	instanceID string,
	instanceDirectory string,
) (core.InfrastructureMetadata, error) {
	var result core.InfrastructureMetadata

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := r.Paths.Validate(); err != nil {
		return result, err
	}
	if instanceID == "" || instanceID == "." || instanceID == ".." ||
		filepath.Base(instanceID) != instanceID ||
		strings.ContainsAny(instanceID, `/\`) ||
		!filepath.IsAbs(instanceDirectory) {
		return result, errors.New("invalid instance identity")
	}

	localMerged := filepath.Clean(filepath.Join(instanceDirectory, "merged"))
	var externalBase, externalMerged string
	var upperExists, workExists, mergedExists bool

	if r.Paths.OverlayRoot != "" {
		root := filepath.Clean(r.Paths.OverlayRoot)
		if !isDirectoryWithoutSymlink(root) {
			result.Warnings = append(result.Warnings, warning(
				"overlay_root_unavailable",
				"Configured OverlayFS root is unavailable or unsafe",
			))
		} else {
			externalBase = filepath.Join(root, instanceID)
			if isDirectoryWithoutSymlink(externalBase) {
				upperExists = isDirectoryWithoutSymlink(filepath.Join(externalBase, "upper"))
				workExists = isDirectoryWithoutSymlink(filepath.Join(externalBase, "work"))
				mergedExists = isDirectoryWithoutSymlink(filepath.Join(externalBase, "merged"))
			}
		}
		externalMerged = filepath.Join(root, instanceID, "merged")
	}

	var overlay *core.OverlayFSMetadata
	if upperExists || workExists || mergedExists {
		overlay = &core.OverlayFSMetadata{
			Source:       "filesystem",
			RuntimeState: core.RuntimeNotChecked,
		}
		if upperExists {
			overlay.UpperDirectory = filepath.Join(externalBase, "upper")
		}
		if workExists {
			overlay.WorkDirectory = filepath.Join(externalBase, "work")
		}
		if mergedExists {
			overlay.MergedDirectory = filepath.Join(externalBase, "merged")
		}
	}

	var matched *mountUnit
	for _, directory := range r.Paths.SystemdUnitDirectories {
		if err := ctx.Err(); err != nil {
			return core.InfrastructureMetadata{}, err
		}
		units, err := readMountUnits(ctx, filepath.Clean(directory))
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return core.InfrastructureMetadata{}, err
			}
			result.Warnings = append(result.Warnings, warning(
				"systemd_directory_unavailable",
				"Configured systemd unit directory is unavailable or unsafe",
			))
			continue
		}
		for i := range units {
			unit := &units[i]
			whereMatches := unit.where == localMerged ||
				(externalMerged != "" && unit.where == externalMerged)
			optionsMatch := externalBase != "" && (unit.upper == filepath.Join(externalBase, "upper") ||
				unit.work == filepath.Join(externalBase, "work"))
			if whereMatches || optionsMatch {
				matched = unit
				break
			}
		}
		if matched != nil {
			break
		}
	}

	if matched != nil {
		result.Systemd = &core.SystemdMountMetadata{
			UnitName:     matched.name,
			UnitFile:     matched.file,
			Where:        matched.where,
			RuntimeState: core.RuntimeNotChecked,
		}
		if overlay == nil {
			overlay = &core.OverlayFSMetadata{
				Source:       "systemd_unit",
				RuntimeState: core.RuntimeNotChecked,
			}
		} else {
			overlay.Source = "filesystem_and_systemd_unit"
		}
		overlay.LowerDirectories = append([]string(nil), matched.lower...)
		if matched.upper != "" {
			overlay.UpperDirectory = matched.upper
		}
		if matched.work != "" {
			overlay.WorkDirectory = matched.work
		}
		overlay.MergedDirectory = matched.where
	}

	result.OverlayFS = overlay
	return result, nil
}

func warning(code, message string) core.DiscoveryWarning {
	return core.DiscoveryWarning{Code: code, Message: message}
}

func isDirectoryWithoutSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

type mountUnit struct {
	name, file, where string
	lower             []string
	upper, work       string
}

func readMountUnits(ctx context.Context, directory string) ([]mountUnit, error) {
	if !isDirectoryWithoutSymlink(directory) {
		return nil, errors.New("unsafe or missing unit directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	var units []mountUnit
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !strings.HasSuffix(entry.Name(), ".mount") || entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		data, err := readSmallRegularFile(path)
		if err != nil {
			// Missing, unreadable, oversized, and symlinked units are skipped.
			continue
		}
		unit, ok := parseMountUnit(data)
		if !ok {
			continue
		}
		unit.name, unit.file = entry.Name(), path
		units = append(units, unit)
	}
	return units, nil
}

func readSmallRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxUnitSize {
		return nil, errors.New("unsafe unit file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUnitSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxUnitSize {
		return nil, errors.New("unit file exceeds size limit")
	}
	return data, nil
}

// parseMountUnit recognizes literal [Mount] declarations only.
// Unknown sections, including Environment/Exec* values, are discarded.
func parseMountUnit(data []byte) (mountUnit, bool) {
	var unit mountUnit
	inMount := false
	var filesystemType, rawOptions string

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inMount = line == "[Mount]"
			continue
		}
		if !inMount {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Type":
			filesystemType = strings.TrimSpace(value)
		case "Where":
			unit.where = cleanAbsolutePath(strings.TrimSpace(value))
		case "Options":
			rawOptions = strings.TrimSpace(value)
		}
	}
	if filesystemType != "overlay" || unit.where == "" {
		return mountUnit{}, false
	}
	for _, entry := range strings.Split(rawOptions, ",") {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "upperdir":
			unit.upper = cleanAbsolutePath(value)
		case "workdir":
			unit.work = cleanAbsolutePath(value)
		case "lowerdir":
			for _, dir := range strings.Split(value, ":") {
				if clean := cleanAbsolutePath(dir); clean != "" {
					unit.lower = append(unit.lower, clean)
				}
			}
		}
	}
	return unit, true
}

func cleanAbsolutePath(path string) string {
	if path == "" || len(path) > 4096 || !filepath.IsAbs(path) ||
		strings.IndexFunc(path, unicode.IsControl) >= 0 ||
		strings.ContainsAny(path, `%\`) {
		return ""
	}
	return filepath.Clean(path)
}
