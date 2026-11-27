package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
	"github.com/atno11/cs2-server-manager/internal/manager"
)

func apiFixture(t *testing.T) (*manager.Manager, manager.Definition) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(root, "data"), manager.Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, manager.Definition{ID: "one", Executable: exe, WorkDir: root, Port: 27015}
}
func TestHeadlessDefaultAndValidationUIOptIn(t *testing.T) {
	m, _ := apiFixture(t)
	token := strings.Repeat("a", 64)
	for _, tc := range []struct {
		enabled      bool
		root, script int
	}{{false, 404, 404}, {true, 200, 200}} {
		h := Handler(m, token, "127.0.0.1:8080", Options{ValidationUI: tc.enabled, HTML: "validation page", JavaScript: "validation script"})
		for _, check := range []struct {
			path string
			want int
		}{{"/", tc.root}, {"/ui.js", tc.script}, {"/api/instances", 200}} {
			r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+check.path, nil)
			if strings.HasPrefix(check.path, "/api/") {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != check.want {
				t.Fatalf("enabled=%v path=%s status=%d", tc.enabled, check.path, w.Code)
			}
			if !tc.enabled && check.want == 404 && w.Body.Len() != 0 {
				t.Fatalf("disabled UI leaked body: %s", w.Body.String())
			}
		}
	}
}
func TestPublicRESTAndSSERepresentationMatch(t *testing.T) {
	m, d := apiFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", 64)
	h := Handler(m, token, "127.0.0.1:8080", Options{})
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/instances", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("list: %d", w.Code)
	}
	var list []apitypes.Instance
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list payload: %q %v", w.Body.String(), err)
	}
	snapshot, sub, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	var value apitypes.Snapshot
	if err = json.Unmarshal([]byte(snapshot.Data), &value); err != nil || len(value.Instances) != 1 || value.Instances[0] != list[0] {
		t.Fatalf("snapshot: %v", err)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API cache protection missing")
	}
	if got := http.StatusOK; w.Code != got {
		t.Fatal("status changed")
	}
}

// Relocated with the status-only response writer; the original assertion is unchanged.
func TestFollowupHTTPOutputNotLogBounded(t *testing.T) {
	payload := strings.Repeat("x", 70*1024)
	out := httptest.NewRecorder()
	w := &statusOnlyWriter{ResponseWriter: out}
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if out.Body.String() != payload {
		t.Fatalf("HTTP body length=%d want=%d", out.Body.Len(), len(payload))
	}
}
