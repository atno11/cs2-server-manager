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

func createCLIFixture(t *testing.T, root string) {
	t.Helper()

	dir := filepath.Join(root, "aim")

	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		".env": "CS2_SERVERNAME=AIM Server\n" +
			"CS2_PORT=27018\n" +
			"CS2_RCONPW=private-value\n",
		"compose.yaml": "services:\n" +
			"  cs2:\n" +
			"    image: joedwards32/cs2:latest\n",
	}

	for name, content := range files {
		if err := os.WriteFile(
			filepath.Join(dir, name),
			[]byte(content),
			0600,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestServersList(t *testing.T) {
	root := t.TempDir()
	createCLIFixture(t, root)

	t.Setenv("CSERVER_ROOT", root)

	var stdout, stderr bytes.Buffer

	code := Run(
		[]string{"servers", "list"},
		&stdout,
		&stderr,
	)

	if code != 0 {
		t.Fatalf(
			"expected success, got %d: %s",
			code,
			stderr.String(),
		)
	}

	var servers []core.DiscoveredServer

	if err := json.Unmarshal(
		stdout.Bytes(),
		&servers,
	); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}

	if len(servers) != 1 ||
		servers[0].ID != "aim" ||
		servers[0].Status != core.DiscoveryComplete {
		t.Fatalf("unexpected servers: %#v", servers)
	}

	if strings.Contains(
		stdout.String(),
		"private-value",
	) {
		t.Fatal("discovery leaked a private value")
	}
}

func TestServersListEmpty(t *testing.T) {
	t.Setenv("CSERVER_ROOT", t.TempDir())

	var stdout, stderr bytes.Buffer

	code := Run(
		[]string{"servers", "list"},
		&stdout,
		&stderr,
	)

	if code != 0 {
		t.Fatalf("expected success, got %d", code)
	}

	if strings.TrimSpace(stdout.String()) != "[]" {
		t.Fatalf(
			"expected empty JSON array, got %q",
			stdout.String(),
		)
	}
}

func TestServersListNotConfigured(t *testing.T) {
	t.Setenv("CSERVER_ROOT", "")

	var stdout, stderr bytes.Buffer

	code := Run(
		[]string{"servers", "list"},
		&stdout,
		&stderr,
	)

	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}

	if !strings.Contains(
		stderr.String(),
		"CSERVER_ROOT",
	) {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestServersInvalidUsage(t *testing.T) {
	for _, args := range [][]string{
		{"servers"},
		{"servers", "start"},
		{"servers", "list", "extra"},
	} {
		var stdout, stderr bytes.Buffer

		code := Run(args, &stdout, &stderr)

		if code != 2 {
			t.Fatalf(
				"args %v: expected exit code 2, got %d",
				args,
				code,
			)
		}

		if !strings.Contains(
			stderr.String(),
			"Usage: cserver servers list",
		) {
			t.Fatalf(
				"unexpected usage output: %s",
				stderr.String(),
			)
		}
	}
}
