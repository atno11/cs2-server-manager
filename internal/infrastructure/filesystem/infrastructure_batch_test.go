package filesystem

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cserver/internal/core"
)

func makeBatchDirectory(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInspectAllAssociatesMultipleInstancesAndUnits(t *testing.T) {
	root := t.TempDir()
	serversRoot := makeBatchDirectory(t, root, "servers")
	overlayRoot := makeBatchDirectory(t, root, "external", "overlay")
	unitDir := makeBatchDirectory(t, root, "external", "units")
	aim := makeBatchDirectory(t, serversRoot, "aim")
	skills := makeBatchDirectory(t, serversRoot, "skills")
	upper := makeBatchDirectory(t, overlayRoot, "aim", "upper")
	work := makeBatchDirectory(t, overlayRoot, "aim", "work")
	makeBatchDirectory(t, overlayRoot, "skills", "upper")
	destination := filepath.Join(root, "another", "merged")
	contents := "[Mount]\nType=overlay\nWhere=" + destination + "\n" +
		"Options=lowerdir=" + filepath.Join(root, "base", "data") + ",upperdir=" + upper + ",workdir=" + work + ",passphrase=PRIVATE\n"
	if err := os.WriteFile(filepath.Join(unitDir, "escaped-name.mount"), []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "irrelevant.mount"), []byte("[Mount]\nType=overlay\nWhere=/unrelated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	servers := []core.DiscoveredServer{{ID: "aim", Directory: aim}, {ID: "skills", Directory: skills}}
	reader := InfrastructureReader{Paths: InfrastructurePaths{OverlayRoot: overlayRoot, SystemdUnitDirectories: []string{unitDir}}}
	got, err := reader.InspectAll(context.Background(), servers)
	if err != nil || len(got) != 2 {
		t.Fatalf("unexpected results: %#v (%v)", got, err)
	}
	aimResult := got["aim"]
	if aimResult.Systemd == nil || aimResult.Systemd.UnitName != "escaped-name.mount" ||
		aimResult.OverlayFS == nil || aimResult.OverlayFS.MergedDirectory != destination ||
		aimResult.OverlayFS.Source != "filesystem_and_systemd_unit" ||
		aimResult.OverlayFS.RuntimeState != core.RuntimeNotChecked {
		t.Fatalf("unexpected aim result: %#v", aimResult)
	}
	skillsResult := got["skills"]
	if skillsResult.Systemd != nil || skillsResult.OverlayFS == nil || skillsResult.OverlayFS.Source != "filesystem" {
		t.Fatalf("unrelated unit associated with skills: %#v", skillsResult)
	}
	if strings.Contains(aimResult.OverlayFS.MergedDirectory, "PRIVATE") {
		t.Fatal("secret leaked")
	}
}

func TestInspectAllReportsMissingUnitDirectorySafely(t *testing.T) {
	root := t.TempDir()
	server := core.DiscoveredServer{ID: "inspect", Directory: makeBatchDirectory(t, root, "inspect")}
	reader := InfrastructureReader{Paths: InfrastructurePaths{SystemdUnitDirectories: []string{filepath.Join(root, "missing")}}}
	result, err := reader.InspectAll(context.Background(), []core.DiscoveredServer{server})
	if err != nil || len(result["inspect"].Warnings) != 1 || result["inspect"].Warnings[0].Code != "systemd_directory_unavailable" {
		t.Fatalf("unexpected warnings: %#v %v", result, err)
	}
	if strings.Contains(result["inspect"].Warnings[0].Message, root) {
		t.Fatal("exposed filesystem path in warning")
	}
}

func TestInspectAllMultipleMatchingUnitsProducesWarning(t *testing.T) {
	root := t.TempDir()
	instance := makeBatchDirectory(t, root, "aim")
	units := makeBatchDirectory(t, root, "units")
	target := filepath.Join(instance, "merged")
	for _, name := range []string{"a.mount", "b.mount"} {
		if err := os.WriteFile(filepath.Join(units, name), []byte("[Mount]\nType=overlay\nWhere="+target+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := (InfrastructureReader{Paths: InfrastructurePaths{SystemdUnitDirectories: []string{units}}}).InspectAll(context.Background(), []core.DiscoveredServer{{ID: "aim", Directory: instance}})
	if err != nil || got["aim"].Systemd == nil || got["aim"].Systemd.UnitName != "a.mount" || len(got["aim"].Warnings) != 1 || got["aim"].Warnings[0].Code != "multiple_systemd_units" {
		t.Fatalf("unexpected collision: %#v %v", got, err)
	}
}

func TestInspectAllCancellationAndEmptyInventory(t *testing.T) {
	reader := InfrastructureReader{}
	result, err := reader.InspectAll(context.Background(), nil)
	if err != nil || len(result) != 0 {
		t.Fatalf("unexpected empty scan: %#v %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reader.InspectAll(ctx, []core.DiscoveredServer{{ID: "aim", Directory: t.TempDir()}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error: %v", err)
	}
	_, err = (InfrastructureReader{Paths: InfrastructurePaths{OverlayRoot: "relative"}}).InspectAll(context.Background(), []core.DiscoveredServer{{ID: "aim", Directory: t.TempDir()}})
	if err == nil {
		t.Fatal("invalid path accepted")
	}
}
