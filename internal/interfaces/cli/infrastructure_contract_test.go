package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cserver/internal/core"
)

func TestServersListExposesSafeInfrastructureMetadata(t *testing.T) {
	root := t.TempDir()
	serversRoot := filepath.Join(root, "servers")
	overlayRoot := filepath.Join(root, "overlay")
	unitDir := filepath.Join(root, "units")

	for _, dir := range []string{
		serversRoot,
		unitDir,
		filepath.Join(overlayRoot, "aim", "upper"),
		filepath.Join(overlayRoot, "aim", "work"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}

	createCLIFixture(t, serversRoot)

	upper := filepath.Join(overlayRoot, "aim", "upper")
	work := filepath.Join(overlayRoot, "aim", "work")
	where := filepath.Join(serversRoot, "aim", "merged")

	unit := "[Mount]\n" +
		"Type=overlay\n" +
		"Where=" + where + "\n" +
		"Options=upperdir=" + upper +
		",workdir=" + work +
		",password=unit-private-value\n" +
		"[Service]\n" +
		"Environment=API_TOKEN=token-private-value\n"

	if err := os.WriteFile(
		filepath.Join(unitDir, "aim.mount"),
		[]byte(unit),
		0600,
	); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CSERVER_ROOT", serversRoot)
	t.Setenv("CSERVER_OVERLAY_ROOT", overlayRoot)
	t.Setenv("CSERVER_SYSTEMD_UNIT_DIRS", unitDir)

	var stdout, stderr bytes.Buffer
	if code := Run(
		[]string{"servers", "list"},
		&stdout,
		&stderr,
	); code != 0 {
		t.Fatalf("CLI failed: %d, %s", code, stderr.String())
	}

	var servers []core.DiscoveredServer
	if err := json.Unmarshal(stdout.Bytes(), &servers); err != nil {
		t.Fatal(err)
	}

	if len(servers) != 1 || servers[0].ID != "aim" {
		t.Fatalf("unexpected inventory: %#v", servers)
	}

	infra := servers[0].Infrastructure
	if infra == nil || infra.OverlayFS == nil || infra.Systemd == nil {
		t.Fatalf("missing infrastructure: %#v", infra)
	}

	if infra.OverlayFS.RuntimeState != core.RuntimeNotChecked ||
		infra.Systemd.RuntimeState != core.RuntimeNotChecked {
		t.Fatal("configuration evidence was represented as runtime state")
	}

	if infra.OverlayFS.UpperDirectory != upper ||
		infra.OverlayFS.WorkDirectory != work ||
		infra.Systemd.UnitName != "aim.mount" {
		t.Fatalf("unexpected metadata: %#v", infra)
	}

	for _, secret := range []string{
		"private-value",
		"unit-private-value",
		"token-private-value",
		"API_TOKEN",
		"CS2_RCONPW",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("CLI exposed restricted configuration: %s", secret)
		}
	}

	if !strings.Contains(stdout.String(), `"infrastructure"`) ||
		!strings.Contains(stdout.String(), `"not_checked"`) {
		t.Fatalf("infrastructure JSON missing: %s", stdout.String())
	}
}

func TestServersListOmitsUnconfiguredInfrastructure(t *testing.T) {
	root := t.TempDir()
	createCLIFixture(t, root)

	t.Setenv("CSERVER_ROOT", root)
	t.Setenv("CSERVER_OVERLAY_ROOT", "")
	t.Setenv("CSERVER_SYSTEMD_UNIT_DIRS", "")

	var stdout, stderr bytes.Buffer
	if code := Run(
		[]string{"servers", "list"},
		&stdout,
		&stderr,
	); code != 0 {
		t.Fatalf("CLI failed: %d, %s", code, stderr.String())
	}

	if strings.Contains(stdout.String(), `"infrastructure"`) {
		t.Fatalf(
			"unconfigured metadata should be omitted: %s",
			stdout.String(),
		)
	}

	var servers []core.DiscoveredServer
	if err := json.Unmarshal(stdout.Bytes(), &servers); err != nil {
		t.Fatal(err)
	}

	if len(servers) != 1 ||
		servers[0].Infrastructure != nil ||
		servers[0].Status != core.DiscoveryComplete {
		t.Fatalf("legacy discovery regression: %#v", servers)
	}
}
