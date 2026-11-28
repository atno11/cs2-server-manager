package main

import (
	"path/filepath"
	"testing"
)

func TestTUIConfigurationDefaultsAndOverrides(t *testing.T) {
	t.Setenv("CS2M_DATA", "/tmp/private-runtime")
	addr, path, err := configuration(nil)
	if err != nil || addr != "http://127.0.0.1:8080" || path != "/tmp/private-runtime/admin.token" {
		t.Fatalf("defaults: %q %q %v", addr, path, err)
	}
	addr, path, err = configuration([]string{"--addr", "http://[::1]:9000", "--token-file", "/tmp/alternate/admin.token"})
	if err != nil || addr != "http://[::1]:9000" || path != "/tmp/alternate/admin.token" {
		t.Fatalf("overrides: %q %q %v", addr, path, err)
	}
	if _, _, err = configuration([]string{"extra"}); err == nil {
		t.Fatal("positional argument accepted")
	}
	t.Setenv("CS2M_DATA", "")
	t.Setenv("HOME", "/tmp/home")
	_, path, err = configuration(nil)
	if err != nil || path != filepath.Join("/tmp/home", ".local", "share", "cs2m", "admin.token") {
		t.Fatalf("home fallback: %q %v", path, err)
	}
}
