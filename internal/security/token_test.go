package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenBootstrapAndPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := LoadOrCreateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreateToken(dir)
	if err != nil || a != b || len(a) != 64 {
		t.Fatal("token not stable")
	}
	if !EqualToken(a, b) || EqualToken("wrong", b) {
		t.Fatal("comparison failed")
	}
	path := filepath.Join(dir, "admin.token")
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("credential permissions")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(dir); err == nil {
		t.Fatal("unsafe credential accepted")
	}
}
func TestTokenRejectsUnsafeDirectoryAndSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(dir); err == nil {
		t.Fatal("unsafe directory accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/null", filepath.Join(dir, "admin.token")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateToken(dir); err == nil {
		t.Fatal("symlink credential accepted")
	}
}
