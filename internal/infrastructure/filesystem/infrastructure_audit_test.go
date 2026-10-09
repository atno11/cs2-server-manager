package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cserver/internal/core"
)

func auditMkdir(t *testing.T, parts ...string) string {
	t.Helper()

	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	return dir
}

func auditUnit(t *testing.T, directory, name, value string) {
	t.Helper()

	if err := os.WriteFile(
		filepath.Join(directory, name),
		[]byte(value),
		0600,
	); err != nil {
		t.Fatal(err)
	}
}

func containsWarning(
	warnings []core.DiscoveryWarning,
	code string,
) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}

	return false
}

func TestInspectAllRejectsCrossInstanceUnitAmbiguity(t *testing.T) {
	root := t.TempDir()
	servers := auditMkdir(t, root, "servers")
	aim := auditMkdir(t, servers, "aim")
	skills := auditMkdir(t, servers, "skills")

	overlay := auditMkdir(t, root, "overlay")
	auditMkdir(t, overlay, "aim", "upper")
	auditMkdir(t, overlay, "skills", "upper")
	auditMkdir(t, overlay, "skills", "work")

	units := auditMkdir(t, root, "units")

	auditUnit(
		t,
		units,
		"cross-instance.mount",
		"[Mount]\nType=overlay\nWhere="+
			filepath.Join(aim, "merged")+
			"\nOptions=upperdir="+
			filepath.Join(overlay, "skills", "upper")+
			",workdir="+
			filepath.Join(overlay, "skills", "work")+"\n",
	)

	reader := InfrastructureReader{
		Paths: InfrastructurePaths{
			OverlayRoot:            overlay,
			SystemdUnitDirectories: []string{units},
		},
	}

	got, err := reader.InspectAll(
		context.Background(),
		[]core.DiscoveredServer{
			{ID: "aim", Directory: aim},
			{ID: "skills", Directory: skills},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"aim", "skills"} {
		if got[id].Systemd != nil {
			t.Fatalf(
				"ambiguous unit assigned to %s: %#v",
				id,
				got[id],
			)
		}

		if !containsWarning(
			got[id].Warnings,
			"ambiguous_systemd_unit",
		) {
			t.Fatalf(
				"missing ambiguity warning for %s: %#v",
				id,
				got[id],
			)
		}
	}
}

func TestInspectAllKeepsExistingFilesystemPathsOnConflict(t *testing.T) {
	root := t.TempDir()
	instance := auditMkdir(t, root, "servers", "aim")
	overlay := auditMkdir(t, root, "overlay")

	upper := auditMkdir(t, overlay, "aim", "upper")
	work := auditMkdir(t, overlay, "aim", "work")
	merged := auditMkdir(t, overlay, "aim", "merged")

	units := auditMkdir(t, root, "units")
	where := filepath.Join(instance, "merged")

	auditUnit(
		t,
		units,
		"mismatch.mount",
		"[Mount]\nType=overlay\nWhere="+where+
			"\nOptions=upperdir="+
			filepath.Join(root, "unexpected", "upper")+
			",workdir="+
			filepath.Join(root, "unexpected", "work")+"\n",
	)

	got, err := (InfrastructureReader{
		Paths: InfrastructurePaths{
			OverlayRoot:            overlay,
			SystemdUnitDirectories: []string{units},
		},
	}).InspectAll(
		context.Background(),
		[]core.DiscoveredServer{
			{ID: "aim", Directory: instance},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	item := got["aim"]

	if item.Systemd == nil || item.Systemd.Where != where {
		t.Fatalf("unit not associated: %#v", item)
	}

	if item.OverlayFS == nil ||
		item.OverlayFS.UpperDirectory != upper ||
		item.OverlayFS.WorkDirectory != work ||
		item.OverlayFS.MergedDirectory != merged {
		t.Fatalf("existing paths overwritten: %#v", item)
	}

	if item.OverlayFS.Source != "filesystem_and_systemd_unit" ||
		item.OverlayFS.RuntimeState != core.RuntimeNotChecked ||
		!containsWarning(item.Warnings, "overlay_path_conflict") {
		t.Fatalf("conflict not reported: %#v", item)
	}
}

func TestInspectAllDeduplicatesConfiguredUnitDirectories(t *testing.T) {
	root := t.TempDir()
	instance := auditMkdir(t, root, "aim")
	dir := auditMkdir(t, root, "units")

	auditUnit(
		t,
		dir,
		"aim.mount",
		"[Mount]\nType=overlay\nWhere="+
			filepath.Join(instance, "merged")+"\n",
	)

	got, err := (InfrastructureReader{
		Paths: InfrastructurePaths{
			SystemdUnitDirectories: []string{dir, dir + "/."},
		},
	}).InspectAll(
		context.Background(),
		[]core.DiscoveredServer{
			{ID: "aim", Directory: instance},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if got["aim"].Systemd == nil ||
		containsWarning(got["aim"].Warnings, "multiple_systemd_units") {
		t.Fatalf(
			"repeated unit directory caused duplicate matches: %#v",
			got,
		)
	}
}

func TestInspectAllPreservesSingleOwnerSelection(t *testing.T) {
	root := t.TempDir()
	instance := auditMkdir(t, root, "servers", "aim")
	units := auditMkdir(t, root, "units")

	auditUnit(
		t,
		units,
		"aim.mount",
		"[Mount]\nType=overlay\nWhere="+
			filepath.Join(instance, "merged")+"\n",
	)

	got, err := (InfrastructureReader{
		Paths: InfrastructurePaths{
			SystemdUnitDirectories: []string{units},
		},
	}).InspectAll(
		context.Background(),
		[]core.DiscoveredServer{
			{ID: "aim", Directory: instance},
		},
	)

	if err != nil ||
		got["aim"].Systemd == nil ||
		got["aim"].OverlayFS == nil ||
		got["aim"].OverlayFS.Source != "systemd_unit" {
		t.Fatalf("valid unit lost: %#v %v", got, err)
	}
}

func TestInspectAllAuditPreservesMultipleUnitsWarning(t *testing.T) {
	root := t.TempDir()
	instance := auditMkdir(t, root, "aim")
	units := auditMkdir(t, root, "units")
	where := filepath.Join(instance, "merged")

	for _, name := range []string{"a.mount", "b.mount"} {
		auditUnit(
			t,
			units,
			name,
			"[Mount]\nType=overlay\nWhere="+where+"\n",
		)
	}

	got, err := (InfrastructureReader{
		Paths: InfrastructurePaths{
			SystemdUnitDirectories: []string{units},
		},
	}).InspectAll(
		context.Background(),
		[]core.DiscoveredServer{
			{ID: "aim", Directory: instance},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	item := got["aim"]
	if item.Systemd == nil ||
		item.Systemd.UnitName != "a.mount" ||
		!containsWarning(item.Warnings, "multiple_systemd_units") {
		t.Fatalf(
			"existing multiple-unit behavior changed: %#v",
			item,
		)
	}
}

func BenchmarkInspectAllManyInstances(b *testing.B) {
	root := b.TempDir()
	serverRoot := filepath.Join(root, "servers")
	overlayRoot := filepath.Join(root, "overlay")
	unitsRoot := filepath.Join(root, "units")

	for _, dir := range []string{
		serverRoot,
		overlayRoot,
		unitsRoot,
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			b.Fatal(err)
		}
	}

	const serverCount = 60
	const unitCount = 120

	servers := make(
		[]core.DiscoveredServer,
		0,
		serverCount,
	)

	for i := 0; i < serverCount; i++ {
		id := fmt.Sprintf("server-%03d", i)
		path := filepath.Join(serverRoot, id)

		if err := os.MkdirAll(path, 0700); err != nil {
			b.Fatal(err)
		}

		servers = append(servers, core.DiscoveredServer{
			ID:        id,
			Directory: path,
		})
	}

	for i := 0; i < unitCount; i++ {
		id := fmt.Sprintf("server-%03d", i%serverCount)
		path := filepath.Join(
			unitsRoot,
			fmt.Sprintf("unit-%03d.mount", i),
		)

		data := "[Mount]\nType=overlay\nWhere=" +
			filepath.Join(serverRoot, id, "merged") + "\n"

		if err := os.WriteFile(
			path,
			[]byte(data),
			0600,
		); err != nil {
			b.Fatal(err)
		}
	}

	reader := InfrastructureReader{
		Paths: InfrastructurePaths{
			OverlayRoot:            overlayRoot,
			SystemdUnitDirectories: []string{unitsRoot},
		},
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := reader.InspectAll(
			context.Background(),
			servers,
		); err != nil {
			b.Fatal(err)
		}
	}
}
