package filesystem

import (
	"context"
	"errors"
	"path/filepath"

	"cserver/internal/core"
)

type associationKey struct {
	kind string
	path string
}

// InspectAll scans mount-unit directories once and indexes associations.
// A unit that points to multiple known servers is not assigned.
func (r InfrastructureReader) InspectAll(
	ctx context.Context,
	servers []core.DiscoveredServer,
) (map[string]core.InfrastructureMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.Paths.Validate(); err != nil {
		return nil, err
	}

	results := make(
		map[string]core.InfrastructureMetadata,
		len(servers),
	)
	if len(servers) == 0 {
		return results, nil
	}

	var units []mountUnit
	var unitWarnings []core.DiscoveryWarning

	scanned := make(
		map[string]struct{},
		len(r.Paths.SystemdUnitDirectories),
	)

	for _, directory := range r.Paths.SystemdUnitDirectories {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		directory = filepath.Clean(directory)

		if _, ok := scanned[directory]; ok {
			continue
		}
		scanned[directory] = struct{}{}

		discovered, err := readMountUnits(ctx, directory)
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}

			unitWarnings = append(unitWarnings, warning(
				"systemd_directory_unavailable",
				"Configured systemd unit directory is unavailable or unsafe",
			))
			continue
		}

		units = append(units, discovered...)
	}

	// Index potential ownership by evidence type and path.
	lookup := make(
		map[associationKey][]int,
		len(servers)*4,
	)

	register := func(kind, path string, index int) {
		if path == "" {
			return
		}

		key := associationKey{
			kind: kind,
			path: path,
		}
		lookup[key] = append(lookup[key], index)
	}

	for i, server := range servers {
		register(
			"where",
			filepath.Join(server.Directory, "merged"),
			i,
		)

		if r.Paths.OverlayRoot != "" {
			base := filepath.Join(
				r.Paths.OverlayRoot,
				server.ID,
			)

			register(
				"where",
				filepath.Join(base, "merged"),
				i,
			)
			register(
				"upper",
				filepath.Join(base, "upper"),
				i,
			)
			register(
				"work",
				filepath.Join(base, "work"),
				i,
			)
		}
	}

	// Store unit indices instead of copying unit contents.
	matches := make([][]int, len(servers))
	ambiguous := make([]bool, len(servers))

	for j, unit := range units {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		owners := make(map[int]struct{}, 3)

		addOwners := func(kind, path string) {
			if path == "" {
				return
			}

			key := associationKey{
				kind: kind,
				path: path,
			}

			for _, index := range lookup[key] {
				owners[index] = struct{}{}
			}
		}

		addOwners("where", unit.where)
		addOwners("upper", unit.upper)
		addOwners("work", unit.work)

		switch len(owners) {
		case 0:
			// Unrelated unit: do not create a server.

		case 1:
			// Multiple matching fields from the same unit
			// still represent one ownership candidate.
			for index := range owners {
				matches[index] = append(
					matches[index],
					j,
				)
			}

		default:
			// Conflicting ownership evidence must never
			// attribute the unit to multiple instances.
			for index := range owners {
				ambiguous[index] = true
			}
		}
	}

	// Read per-instance directory evidence without
	// scanning systemd units again.
	directoryReader := InfrastructureReader{
		Paths: InfrastructurePaths{
			OverlayRoot: r.Paths.OverlayRoot,
		},
	}

	for index, server := range servers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		info, err := directoryReader.Inspect(
			ctx,
			server.ID,
			server.Directory,
		)
		if err != nil {
			return nil, err
		}

		if len(matches[index]) > 0 {
			unit := units[matches[index][0]]

			info.Systemd = &core.SystemdMountMetadata{
				UnitName:     unit.name,
				UnitFile:     unit.file,
				Where:        unit.where,
				RuntimeState: core.RuntimeNotChecked,
			}

			if info.OverlayFS == nil {
				info.OverlayFS = &core.OverlayFSMetadata{
					Source:       "systemd_unit",
					RuntimeState: core.RuntimeNotChecked,
				}
			} else {
				info.OverlayFS.Source =
					"filesystem_and_systemd_unit"
			}

			overlay := info.OverlayFS
			overlay.LowerDirectories = append(
				[]string(nil),
				unit.lower...,
			)

			conflict := false

			// Preserve existing filesystem evidence.
			// Fill missing paths from the unit declaration.
			if overlay.UpperDirectory == "" {
				overlay.UpperDirectory = unit.upper
			} else if unit.upper != "" &&
				overlay.UpperDirectory != unit.upper {
				conflict = true
			}

			if overlay.WorkDirectory == "" {
				overlay.WorkDirectory = unit.work
			} else if unit.work != "" &&
				overlay.WorkDirectory != unit.work {
				conflict = true
			}

			if overlay.MergedDirectory == "" {
				overlay.MergedDirectory = unit.where
			} else if unit.where != "" &&
				overlay.MergedDirectory != unit.where {
				conflict = true
			}

			if conflict {
				info.Warnings = append(
					info.Warnings,
					warning(
						"overlay_path_conflict",
						"Filesystem paths disagree with the selected mount unit declaration",
					),
				)
			}

			if len(matches[index]) > 1 {
				info.Warnings = append(
					info.Warnings,
					warning(
						"multiple_systemd_units",
						"Multiple overlay mount units match this instance; first match selected",
					),
				)
			}
		}

		if ambiguous[index] {
			info.Warnings = append(
				info.Warnings,
				warning(
					"ambiguous_systemd_unit",
					"An overlay mount unit matches multiple CS2 instances and was not assigned",
				),
			)
		}

		info.Warnings = append(
			info.Warnings,
			unitWarnings...,
		)

		if info.OverlayFS != nil ||
			info.Systemd != nil ||
			len(info.Warnings) > 0 {
			results[server.ID] = info
		}
	}

	return results, nil
}
