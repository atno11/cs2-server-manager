package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cserver/internal/core"
)

func createDir(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(parts...)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeUnit(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInspectExternalOverlayAndEscapedMountUnit(t *testing.T) {
	tmp := t.TempDir()
	root := createDir(t, tmp, "cs2")
	instance := createDir(t, root, "aim")
	overlayRoot := createDir(t, tmp, "external-overlay")
	createDir(t, overlayRoot, "aim", "upper")
	createDir(t, overlayRoot, "aim", "work")
	merged := createDir(t, instance, "merged")
	units := createDir(t, tmp, "units")
	lower := createDir(t, root, "base", "data")
	upper := filepath.Join(overlayRoot, "aim", "upper")
	work := filepath.Join(overlayRoot, "aim", "work")
	unitFile := writeUnit(t, units,
		"mnt-ssd-srv-cs2-aim-merged.mount",
		"[Unit]\nDescription=CS2\n[Mount]\nWhat=overlay\nType=overlay\nWhere="+merged+"\n"+
			"Options=lowerdir="+lower+",upperdir="+upper+",workdir="+work+",password=DO_NOT_EXPOSE\n"+
			"[Service]\nEnvironment=API_TOKEN=DO_NOT_EXPOSE\n",
	)

	reader := InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot:            overlayRoot,
		SystemdUnitDirectories: []string{units},
	}}
	got, err := reader.Inspect(context.Background(), "aim", instance)
	if err != nil {
		t.Fatal(err)
	}
	if got.Systemd == nil || got.Systemd.UnitFile != unitFile ||
		got.Systemd.Where != merged || got.Systemd.RuntimeState != core.RuntimeNotChecked {
		t.Fatalf("unexpected systemd metadata: %#v", got.Systemd)
	}
	if got.OverlayFS == nil || got.OverlayFS.Source != "filesystem_and_systemd_unit" ||
		got.OverlayFS.UpperDirectory != upper || got.OverlayFS.WorkDirectory != work ||
		got.OverlayFS.MergedDirectory != merged ||
		len(got.OverlayFS.LowerDirectories) != 1 ||
		got.OverlayFS.LowerDirectories[0] != lower ||
		got.OverlayFS.RuntimeState != core.RuntimeNotChecked {
		t.Fatalf("unexpected overlay metadata: %#v", got.OverlayFS)
	}
	serialized, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), "DO_NOT_EXPOSE") ||
		strings.Contains(string(serialized), "API_TOKEN") {
		t.Fatal("unit secrets leaked into metadata")
	}
}

func TestInspectUnitWhereExternalToBothRoots(t *testing.T) {
	tmp := t.TempDir()
	instance := createDir(t, tmp, "servers", "skills")
	overlayRoot := createDir(t, tmp, "storage")
	createDir(t, overlayRoot, "skills", "upper")
	unitDir := createDir(t, tmp, "unit-files")
	where := filepath.Join(tmp, "separate", "merged")
	writeUnit(t, unitDir, "custom.mount", "[Mount]\nType=overlay\nWhere="+where+"\n"+
		"Options=upperdir="+filepath.Join(overlayRoot, "skills", "upper")+"\n")

	got, err := (InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot:            overlayRoot,
		SystemdUnitDirectories: []string{unitDir},
	}}).Inspect(context.Background(), "skills", instance)
	if err != nil || got.Systemd == nil || got.Systemd.Where != where ||
		got.OverlayFS == nil || got.OverlayFS.MergedDirectory != where {
		t.Fatalf("external Where must match through upperdir: %#v, %v", got, err)
	}
}

func TestInspectFilesystemOnlyAndUnconfigured(t *testing.T) {
	root := t.TempDir()
	instance := createDir(t, root, "servers", "inspect")
	overlay := createDir(t, root, "overlay")
	createDir(t, overlay, "inspect", "upper")
	createDir(t, overlay, "inspect", "work")

	got, err := (InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot: overlay,
	}}).Inspect(context.Background(), "inspect", instance)
	if err != nil || got.Systemd != nil || got.OverlayFS == nil ||
		got.OverlayFS.Source != "filesystem" || got.OverlayFS.MergedDirectory != "" {
		t.Fatalf("unexpected filesystem-only result: %#v, %v", got, err)
	}

	empty, err := (InfrastructureReader{}).Inspect(context.Background(), "inspect", instance)
	if err != nil || empty.Systemd != nil || empty.OverlayFS != nil ||
		len(empty.Warnings) != 0 {
		t.Fatalf("unconfigured scan should be empty: %#v, %v", empty, err)
	}
}

func TestInspectRejectsUnrelatedMalformedAndSymlinkUnits(t *testing.T) {
	root := t.TempDir()
	instance := createDir(t, root, "instances", "aim")
	unitDir := createDir(t, root, "units")
	createDir(t, root, "other")
	writeUnit(t, unitDir, "unrelated.mount", "[Mount]\nType=overlay\nWhere="+
		filepath.Join(root, "other", "merged")+"\n")
	writeUnit(t, unitDir, "wrong.mount", "[Mount]\nType=ext4\nWhere="+
		filepath.Join(instance, "merged")+"\n")
	writeUnit(t, unitDir, "malformed.mount", "[Mount]\nType=overlay\nWhere=relative\n")
	writeUnit(t, unitDir, "huge.mount", strings.Repeat("A", int(maxUnitSize)+1))
	external := writeUnit(t, root, "external.mount", "[Mount]\nType=overlay\nWhere="+
		filepath.Join(instance, "merged")+"\n")
	if err := os.Symlink(external, filepath.Join(unitDir, "linked.mount")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got, err := (InfrastructureReader{Paths: InfrastructurePaths{
		SystemdUnitDirectories: []string{unitDir},
	}}).Inspect(context.Background(), "aim", instance)
	if err != nil || got.Systemd != nil || got.OverlayFS != nil {
		t.Fatalf("unrelated/unsafe units were associated: %#v, %v", got, err)
	}
}

func TestInspectUnavailableSourcesAndInvalidInputs(t *testing.T) {
	root := t.TempDir()
	instance := createDir(t, root, "aim")
	reader := InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot:            filepath.Join(root, "missing-overlay"),
		SystemdUnitDirectories: []string{filepath.Join(root, "missing-units")},
	}}
	got, err := reader.Inspect(context.Background(), "aim", instance)
	if err != nil || len(got.Warnings) != 2 || got.OverlayFS != nil {
		t.Fatalf("expected safe warnings: %#v, %v", got, err)
	}
	for _, unsafeID := range []string{"../secret", ".", "", "foo/bar", "foo\\bar"} {
		if _, err := reader.Inspect(context.Background(), unsafeID, instance); err == nil {
			t.Fatalf("accepted unsafe id %q", unsafeID)
		}
	}
	if _, err := (InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot: "relative",
	}}).Inspect(context.Background(), "aim", instance); err == nil {
		t.Fatal("accepted relative overlay root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reader.Inspect(ctx, "aim", instance)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestInspectSkipsSymlinkedOverlayInstance(t *testing.T) {
	root := t.TempDir()
	instance := createDir(t, root, "servers", "aim")
	overlay := createDir(t, root, "overlay")
	external := createDir(t, root, "external", "upper")
	if err := os.Symlink(filepath.Dir(external), filepath.Join(overlay, "aim")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got, err := (InfrastructureReader{Paths: InfrastructurePaths{
		OverlayRoot: overlay,
	}}).Inspect(context.Background(), "aim", instance)
	if err != nil || got.OverlayFS != nil {
		t.Fatalf("followed overlay symlink: %#v, %v", got, err)
	}
}
