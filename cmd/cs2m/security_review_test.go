package main

import (
	"github.com/atno11/cs2-server-manager/internal/manager"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewHTTPDuplicateFields(t *testing.T) {
	h, token, root, exe := securityFixture(t)
	for _, fields := range []string{`"id":"one","ID":"two"`, `"id":"one","id":"two"`} {
		body := []byte(`{` + fields + `,"executable":"` + exe + `","work_dir":"` + root + `","port":27015}`)
		w := call(h, "POST", "/api/instances", token, "127.0.0.1:8080", "", "", "application/json", body)
		if w.Code != 400 {
			t.Fatalf("ambiguous HTTP JSON status=%d", w.Code)
		}
	}
}

func TestReviewPublicErrorsRedacted(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(root, "data"), manager.Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}, KnownSecrets: []string{"invalid"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	h := handler(m, strings.Repeat("t", 64), "127.0.0.1:8080")
	w := call(h, "POST", "/api/instances", strings.Repeat("t", 64), "127.0.0.1:8080", "", "", "application/json", []byte("{"))
	if w.Code != 400 || strings.Contains(w.Body.String(), "invalid") {
		t.Fatalf("status=%d response=%q", w.Code, w.Body.String())
	}
}
