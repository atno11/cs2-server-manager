package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func securityFixture(t *testing.T) (http.Handler, string, string, string) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, token, err := testManager(root, exe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return handler(m, token, "127.0.0.1:8080"), token, root, exe
}
func call(h http.Handler, method, path, token, host, origin, fetchSite, contentType string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Host = host
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if fetchSite != "" {
		r.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func registration(exe, root string) []byte {
	b, _ := json.Marshal(map[string]any{"id": "one", "executable": exe, "work_dir": root, "port": 27015, "args": []string{"safevalue"}})
	return b
}
func TestAuthenticationHostOriginAndMethods(t *testing.T) {
	h, token, root, exe := securityFixture(t)
	body := registration(exe, root)
	for _, tc := range []struct {
		name, method, path, cred, host, origin, site string
		want                                         int
	}{{"missing", "GET", "/api/instances", "", "127.0.0.1:8080", "", "", 401}, {"invalid", "GET", "/api/instances", "wrong", "127.0.0.1:8080", "", "", 401}, {"host", "POST", "/api/instances", token, "evil.example", "", "", 421}, {"rebinding", "POST", "/api/instances", token, "localhost:8080", "", "", 421}, {"origin", "POST", "/api/instances", token, "127.0.0.1:8080", "https://evil.example", "", 403}, {"fetch-site", "POST", "/api/instances", token, "127.0.0.1:8080", "", "cross-site", 403}, {"wrong-route", "DELETE", "/api/instances/one/start", token, "127.0.0.1:8080", "", "", 405}, {"wrong-method", "GET", "/api/instances/one/start", token, "127.0.0.1:8080", "", "", 405}} {
		t.Run(tc.name, func(t *testing.T) {
			w := call(h, tc.method, tc.path, tc.cred, tc.host, tc.origin, tc.site, "application/json", body)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d", w.Code, tc.want)
			}
		})
	}
	w := call(h, "GET", "/api/instances", token, "127.0.0.1:8080", "", "", "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = call(h, "POST", "/api/instances", token, "127.0.0.1:8080", "http://127.0.0.1:8080", "same-origin", "application/json", body)
	if w.Code != 201 {
		t.Fatalf("same origin registration=%d", w.Code)
	}
	for _, secret := range []string{token, root, exe, "safevalue"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private definition data exposed")
		}
	}
	w = call(h, "GET", "/api/instances/one", token, "127.0.0.1:8080", "", "", "", nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), exe) {
		t.Fatal("public inspection exposed executable")
	}
}
func TestStrictJSONAndNoPartialMutation(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		body          []byte
		want          int
	}{{"malformed", "application/json", []byte(`{"id":`), 400}, {"unknown", "application/json", []byte(`{"unknown":1}`), 400}, {"trailing", "application/json", []byte(`{} {}`), 400}, {"oversized", "application/json", []byte(`"` + strings.Repeat("x", 16*1024+1) + `"`), 413}, {"content-type", "text/plain", []byte(`{}`), 415}} {
		t.Run(tc.name, func(t *testing.T) {
			h, token, _, _ := securityFixture(t)
			w := call(h, "POST", "/api/instances", token, "127.0.0.1:8080", "", "", tc.content, tc.body)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d", w.Code, tc.want)
			}
			list := call(h, "GET", "/api/instances", token, "127.0.0.1:8080", "", "", "", nil)
			if !strings.Contains(list.Body.String(), "[]") {
				t.Fatal("invalid request mutated instances")
			}
		})
	}
}
func TestUntrustedForwardedHeadersIgnored(t *testing.T) {
	h, token, _, _ := securityFixture(t)
	r := httptest.NewRequest("GET", "/api/instances", nil)
	r.Host = "127.0.0.1:8080"
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Forwarded-Host", "evil.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("forwarded headers changed authority: %d", w.Code)
	}
}

func TestHTTPUntrustedExecutableRejectedWithoutPathLeak(t *testing.T) {
	h, token, root, _ := securityFixture(t)
	body := registration("/bin/sh", root)
	w := call(h, "POST", "/api/instances", token, "127.0.0.1:8080", "", "", "application/json", body)
	if w.Code != 400 || strings.Contains(w.Body.String(), "/bin/sh") {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}
func TestConnectionLimit(t *testing.T) {
	underlying := &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	limited := newLimitedListener(underlying, 1)
	accepted := make(chan net.Conn, 1)
	go func() { c, _ := limited.Accept(); accepted <- c }()
	client1, server1 := net.Pipe()
	underlying.connections <- server1
	first := <-accepted
	client2, server2 := net.Pipe()
	done := make(chan struct{})
	go func() { _, _ = limited.Accept(); close(done) }()
	underlying.connections <- server2
	_ = client2.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err := client2.Read(b[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("excess connection not closed: %v", err)
	}
	_ = first.Close()
	_ = client1.Close()
	_ = client2.Close()
	_ = limited.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("limited accept did not close")
	}
}
