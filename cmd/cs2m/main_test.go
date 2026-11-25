package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atno11/cs2-server-manager/internal/manager"
)

func TestAPI(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho hello\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := handler(m)
	body := `{"id":"one","executable":"` + exe + `","work_dir":"` + dir + `","port":27015}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/instances", strings.NewReader(body)))
	if w.Code != 201 {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/instances/one", nil))
	if w.Code != 200 {
		t.Fatalf("inspect: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/instances/one/logs", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("logs: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/instances/one", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
}
