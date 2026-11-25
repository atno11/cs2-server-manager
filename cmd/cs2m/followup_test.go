package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atno11/cs2-server-manager/internal/manager"
)

func TestFollowupJSONSchemaIsStable(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(root, "data"), manager.Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}, KnownSecrets: []string{"port"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	h := handler(m, strings.Repeat("t", 64), "127.0.0.1:8080")
	registered := call(h, "POST", "/api/instances", strings.Repeat("t", 64), "127.0.0.1:8080", "", "", "application/json", registration(exe, root))
	if registered.Code != 201 {
		t.Fatalf("register status=%d", registered.Code)
	}
	for _, response := range []*httptest.ResponseRecorder{registered, call(h, "GET", "/api/instances", strings.Repeat("t", 64), "127.0.0.1:8080", "", "", "", nil), call(h, "GET", "/api/instances/one", strings.Repeat("t", 64), "127.0.0.1:8080", "", "", "", nil)} {
		var value any
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatalf("invalid JSON: %q: %v", response.Body.String(), err)
		}
		if !strings.Contains(response.Body.String(), `"port"`) {
			t.Fatalf("port field changed: %q", response.Body.String())
		}
		if strings.Contains(response.Body.String(), `"[REDACTED]"`) {
			t.Fatalf("schema was rewritten: %q", response.Body.String())
		}
	}
}

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

func TestFollowupErrorsHaveNoEchoedBody(t *testing.T) {
	h, token, _, _ := securityFixture(t)
	for _, tc := range []struct {
		name, method, path, credential, contentType string
		body                                        []byte
		want                                        int
	}{
		{"unknown-path", "GET", "/api/unknown", token, "", nil, 404},
		{"wrong-method", "GET", "/api/instances/one/start", token, "", nil, 405},
		{"invalid-json", "POST", "/api/instances", token, "application/json", []byte("{"), 400},
		{"unauthorized", "GET", "/api/instances", "", "", nil, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := call(h, tc.method, tc.path, tc.credential, "127.0.0.1:8080", "", "", tc.contentType, tc.body)
			if w.Code != tc.want || w.Body.Len() != 0 {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if tc.want == 401 && w.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing authentication challenge")
			}
		})
	}
}
