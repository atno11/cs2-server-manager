package tui

import (
	"strings"
	"testing"

	"cserver/internal/core"
)

func TestServersViewRendersInfrastructureMetadata(t *testing.T) {
	m := testModel()
	m.screen = screenServers
	m.height = 40
	m.servers = []core.DiscoveredServer{
		{
			ID:       "aim",
			Name:     "AIM Server",
			Status:   core.DiscoveryComplete,
			Evidence: []string{"env", "compose"},
			Infrastructure: &core.InfrastructureMetadata{
				OverlayFS: &core.OverlayFSMetadata{
					LowerDirectories: []string{"/example/base/data"},
					UpperDirectory:   "/example/overlay/aim/upper",
					WorkDirectory:    "/example/overlay/aim/work",
					MergedDirectory:  "/example/servers/aim/merged",
					Source:           "filesystem_and_systemd_unit",
					RuntimeState:     core.RuntimeNotChecked,
				},
				Systemd: &core.SystemdMountMetadata{
					UnitName:     "aim.mount",
					UnitFile:     "/example/units/aim.mount",
					Where:        "/example/servers/aim/merged",
					RuntimeState: core.RuntimeNotChecked,
				},
			},
		},
	}

	view := m.renderServers()
	for _, expected := range []string{
		"AIM Server",
		"OverlayFS: configuration found",
		"systemd: mount unit configuration found",
		"filesystem_and_systemd_unit",
		"not_checked",
		"/example/base/data",
		"/example/overlay/aim/upper",
		"/example/overlay/aim/work",
		"/example/servers/aim/merged",
		"aim.mount",
		"mount and service activity not checked",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q:\n%s", expected, view)
		}
	}
}

func TestServersViewFocusesFirstVisibleInstance(t *testing.T) {
	m := testModel()
	m.screen = screenServers
	m.height = 40
	m.servers = []core.DiscoveredServer{
		{
			ID:   "aim",
			Name: "AIM",
			Infrastructure: &core.InfrastructureMetadata{
				Systemd: &core.SystemdMountMetadata{
					UnitName:     "aim.mount",
					UnitFile:     "/units/aim.mount",
					Where:        "/servers/aim/merged",
					RuntimeState: core.RuntimeNotChecked,
				},
			},
		},
		{
			ID:   "skills",
			Name: "Skills",
		},
	}

	initial := m.renderServers()
	if !strings.Contains(initial, "Infrastructure: aim") {
		t.Fatal("incorrect initial focus")
	}

	m.serverOffset = 1
	next := m.renderServers()

	if !strings.Contains(next, "Infrastructure: skills") ||
		!strings.Contains(next, "No infrastructure metadata available.") ||
		strings.Contains(next, "Unit: aim.mount") {
		t.Fatalf("focus did not follow scrolling:\n%s", next)
	}
}

func TestServersViewDoesNotConfuseWarningsWithRuntime(t *testing.T) {
	m := testModel()
	m.screen = screenServers
	m.servers = []core.DiscoveredServer{
		{
			ID:   "inspect",
			Name: "Inspect",
			Infrastructure: &core.InfrastructureMetadata{
				Warnings: []core.DiscoveryWarning{
					{
						Code:    "systemd_directory_unavailable",
						Message: "Configured systemd unit directory is unavailable or unsafe",
					},
				},
			},
		},
	}

	view := m.renderServers()

	if !strings.Contains(view, "systemd_directory_unavailable") ||
		!strings.Contains(view, "no metadata available") ||
		strings.Contains(view, "mounted") ||
		strings.Contains(view, "active") {
		t.Fatalf("unexpected view:\n%s", view)
	}
}

func TestTerminalSafeRemovesControlCharacters(t *testing.T) {
	input := "unit\x1b[31m\n\u202esecret"
	output := terminalSafe(input)

	if strings.Contains(output, "\x1b") ||
		strings.Contains(output, "\n") ||
		strings.Contains(output, "\u202e") {
		t.Fatalf("terminal control sequence survived: %q", output)
	}

	if !strings.Contains(output, "unit") {
		t.Fatal("printable content unexpectedly removed")
	}
}
