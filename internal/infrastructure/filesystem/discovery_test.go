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

func writeFixture(
	t *testing.T,
	root, name, filename, content string,
) {
	t.Helper()

	dir := filepath.Join(root, name)

	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, filename)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0600,
	); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverRecognizesInstancesAndSkipsHelpers(
	t *testing.T,
) {
	root := t.TempDir()

	writeFixture(
		t,
		root,
		"aim",
		".env",
		"CS2_SERVERNAME='AIM server'\n"+
			"CS2_PORT=27018\n"+
			"CS2_RCONPW=do-not-leak\n",
	)

	writeFixture(
		t,
		root,
		"aim",
		"compose.yaml",
		"services:\n"+
			"  cs2:\n"+
			"    image: joedwards32/cs2:latest\n",
	)

	writeFixture(
		t,
		root,
		"base",
		"compose.yaml",
		"services:\n"+
			"  cs2:\n"+
			"    image: joedwards32/cs2:latest\n",
	)

	writeFixture(
		t,
		root,
		"mysql",
		".env",
		"MYSQL_ROOT_PASSWORD=secret\n",
	)

	writeFixture(
		t,
		root,
		"mysql",
		"compose.yml",
		"services:\n"+
			"  mysql:\n"+
			"    image: mysql:8.4\n",
	)

	if err := os.Mkdir(
		filepath.Join(root, "backups"),
		0700,
	); err != nil {
		t.Fatal(err)
	}

	servers, err := (Discoverer{}).Discover(
		context.Background(),
		root,
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(servers) != 2 ||
		servers[0].ID != "aim" ||
		servers[1].ID != "base" {
		t.Fatalf(
			"unexpected servers: %#v",
			servers,
		)
	}

	if servers[0].Name != "AIM server" ||
		servers[0].Port == nil ||
		*servers[0].Port != 27018 ||
		servers[0].Status != core.DiscoveryComplete {
		t.Fatalf(
			"unexpected AIM: %#v",
			servers[0],
		)
	}

	if servers[1].Status != core.DiscoveryIncomplete {
		t.Fatalf(
			"expected incomplete base: %#v",
			servers[1],
		)
	}

	if strings.Contains(
		servers[0].Name,
		"do-not-leak",
	) {
		t.Fatal("secret disclosed")
	}
}

func TestDiscoverInvalidPortAndMissingCompose(
	t *testing.T,
) {
	root := t.TempDir()

	writeFixture(
		t,
		root,
		"skills",
		".env",
		"CS2_STARTMAP=de_mirage\n"+
			"CS2_PORT=bogus\n",
	)

	servers, err := (Discoverer{}).Discover(
		context.Background(),
		root,
	)

	if err != nil ||
		len(servers) != 1 ||
		servers[0].Port != nil ||
		servers[0].Status != core.DiscoveryIncomplete ||
		len(servers[0].Warnings) < 2 {
		t.Fatalf(
			"expected incomplete with warnings: %#v, %v",
			servers,
			err,
		)
	}
}

func TestDiscoverDoesNotFollowSymlinks(
	t *testing.T,
) {
	root := t.TempDir()
	external := t.TempDir()

	writeFixture(
		t,
		external,
		"secret",
		".env",
		"CS2_PORT=27015",
	)

	err := os.Symlink(
		filepath.Join(external, "secret"),
		filepath.Join(root, "linked"),
	)
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	writeFixture(
		t,
		root,
		"inspect",
		"compose.yaml",
		"services:\n"+
			"  game:\n"+
			"    image: joedwards32/cs2\n",
	)

	err = os.Symlink(
		filepath.Join(external, "secret", ".env"),
		filepath.Join(root, "inspect", ".env"),
	)
	if err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	servers, err := (Discoverer{}).Discover(
		context.Background(),
		root,
	)

	if err != nil ||
		len(servers) != 1 ||
		servers[0].ID != "inspect" ||
		servers[0].EnvFile != "" {
		t.Fatalf(
			"unexpected symlink discovery: %#v, %v",
			servers,
			err,
		)
	}
}

func TestDiscoverUnreadableOversizedEnv(
	t *testing.T,
) {
	root := t.TempDir()

	writeFixture(
		t,
		root,
		"aim",
		"compose.yaml",
		"image: joedwards32/cs2:latest\n",
	)

	writeFixture(
		t,
		root,
		"aim",
		".env",
		"CS2_SERVERNAME="+
			strings.Repeat("X", maxArtifactSize),
	)

	servers, err := (Discoverer{}).Discover(
		context.Background(),
		root,
	)

	if err != nil ||
		len(servers) != 1 ||
		servers[0].Status != core.DiscoveryIncomplete {
		t.Fatalf(
			"unexpected oversized-file discovery: %#v, %v",
			servers,
			err,
		)
	}
}

func TestDiscoverRootAndCancellationErrors(
	t *testing.T,
) {
	reader := Discoverer{}

	if _, err := reader.Discover(
		context.Background(),
		"relative",
	); err == nil {
		t.Fatal("expected invalid-root error")
	}

	missing := filepath.Join(
		t.TempDir(),
		"missing",
	)

	_, err := reader.Discover(
		context.Background(),
		missing,
	)

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(
			"expected missing-root error, got %v",
			err,
		)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	_, err = reader.Discover(
		ctx,
		t.TempDir(),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected cancellation, got %v",
			err,
		)
	}
}
