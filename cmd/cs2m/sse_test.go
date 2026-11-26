package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/manager"
)

func sseManager(t *testing.T) (*manager.Manager, string, manager.Definition) {
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
	return m, token, manager.Definition{ID: "one", Executable: exe, WorkDir: root, Port: 27015}
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var frame strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		frame.WriteString(line)
		if line == "\n" {
			return frame.String()
		}
	}
}

func TestSSEAuthenticationCapacityAndAdministrativeReserve(t *testing.T) {
	m, token, _ := sseManager(t)
	h := handler(m, token, "127.0.0.1:8080")
	for _, tc := range []struct {
		host, origin, site, credential string
		want                           int
	}{
		{"127.0.0.1:8080", "", "", "", 401},
		{"wrong.example", "", "", token, 421},
		{"127.0.0.1:8080", "https://evil.example", "", token, 403},
		{"127.0.0.1:8080", "", "cross-site", token, 403},
	} {
		out := call(h, "GET", "/api/events", tc.credential, tc.host, tc.origin, tc.site, "", nil)
		if out.Code != tc.want || out.Body.Len() != 0 {
			t.Fatalf("rejection: %d %q", out.Code, out.Body.String())
		}
	}
	if m.EventSubscriberCount() != 0 {
		t.Fatal("rejected requests reserved subscribers")
	}
	subs := make([]interface{ Cancel() }, 0, 8)
	for range 8 {
		_, sub, err := m.Subscribe()
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, sub)
	}
	defer func() {
		for _, sub := range subs {
			sub.Cancel()
		}
	}()
	out := call(h, "GET", "/api/events", token, "127.0.0.1:8080", "", "", "", nil)
	if out.Code != 503 || out.Body.Len() != 0 {
		t.Fatalf("capacity response: %d %q", out.Code, out.Body.String())
	}
	out = call(h, "GET", "/api/instances", token, "127.0.0.1:8080", "", "", "", nil)
	if out.Code != 200 {
		t.Fatalf("administrative request blocked: %d", out.Code)
	}
}

func TestSSEBrowserAssetsKeepTokenOutOfURL(t *testing.T) {
	m, token, _ := sseManager(t)
	h := handler(m, token, "127.0.0.1:8080")
	page := call(h, "GET", "/", "", "127.0.0.1:8080", "", "", "", nil)
	script := call(h, "GET", "/ui.js", "", "127.0.0.1:8080", "", "", "", nil)
	if page.Code != 200 || script.Code != 200 || !strings.Contains(page.Body.String(), `src="/ui.js"`) || !strings.Contains(script.Body.String(), "Authorization:'Bearer '+token") || strings.Contains(script.Body.String(), token) {
		t.Fatal("browser assets missing or embedded credential")
	}
}

func TestSSERealNetworkFlushHeartbeatAndDisconnect(t *testing.T) {
	m, token, definition := sseManager(t)
	ts := httptest.NewUnstartedServer(nil)
	authority := ts.Listener.Addr().String()
	ts.Config.Handler = handlerWithHeartbeat(m, token, authority, 15*time.Millisecond)
	ts.Config.WriteTimeout = 40 * time.Millisecond
	ts.Start()
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream; charset=utf-8" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers: %d %#v", response.StatusCode, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	if frame := readSSEFrame(t, reader); !strings.Contains(frame, "event: snapshot\n") || !strings.Contains(frame, "data: {\"instances\":[]}") {
		t.Fatalf("snapshot frame: %q", frame)
	}
	if err := m.Register(definition); err != nil {
		t.Fatal(err)
	}
	if frame := readSSEFrame(t, reader); !strings.Contains(frame, "id: 1\n") || !strings.Contains(frame, "event: instance.registered\n") || strings.Contains(frame, definition.Executable) || strings.Contains(frame, token) {
		t.Fatalf("registration frame: %q", frame)
	}
	for range 5 {
		if frame := readSSEFrame(t, reader); frame != ": heartbeat\n\n" {
			t.Fatalf("heartbeat: %q", frame)
		}
	}
	if m.EventSubscriberCount() != 1 {
		t.Fatalf("stream not active after write deadline: %d", m.EventSubscriberCount())
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for m.EventSubscriberCount() != 0 {
		select {
		case <-deadline:
			t.Fatal("disconnect did not release subscriber")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestSSEEightStreamsLeaveAdministrativeCapacity(t *testing.T) {
	m, token, _ := sseManager(t)
	ts := httptest.NewUnstartedServer(nil)
	ts.Config.Handler = handler(m, token, ts.Listener.Addr().String())
	ts.Start()
	defer ts.Close()
	client := ts.Client()
	cancels := make([]context.CancelFunc, 0, 8)
	responses := make([]*http.Response, 0, 8)
	defer func() {
		for _, cancel := range cancels {
			cancel()
		}
		for _, response := range responses {
			_ = response.Body.Close()
		}
	}()
	for range 8 {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
		if response.StatusCode != 200 || !strings.Contains(readSSEFrame(t, bufio.NewReader(response.Body)), "event: snapshot") {
			t.Fatal("stream did not start")
		}
	}
	admin, err := http.NewRequest("GET", ts.URL+"/api/instances", nil)
	if err != nil {
		t.Fatal(err)
	}
	admin.Header.Set("Authorization", "Bearer "+token)
	result, err := client.Do(admin)
	if err != nil {
		t.Fatal(err)
	}
	_ = result.Body.Close()
	if result.StatusCode != 200 {
		t.Fatalf("administrative request at capacity: %d", result.StatusCode)
	}
	tooMany, err := http.NewRequest("GET", ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	tooMany.Header.Set("Authorization", "Bearer "+token)
	result, err = client.Do(tooMany)
	if err != nil {
		t.Fatal(err)
	}
	_ = result.Body.Close()
	if result.StatusCode != 503 {
		t.Fatalf("ninth stream: %d", result.StatusCode)
	}
}

func TestSSEServerShutdownClosesActiveStream(t *testing.T) {
	m, token, _ := sseManager(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	authority := listener.Addr().String()
	srv := &http.Server{Handler: handlerWithHeartbeat(m, token, authority, 15*time.Millisecond), WriteTimeout: 40 * time.Millisecond, ReadHeaderTimeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveWithTimeout(ctx, srv, newLimitedListener(listener, 64), m, time.Second) }()
	req, err := http.NewRequest("GET", "http://"+authority+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	if frame := readSSEFrame(t, reader); !strings.Contains(frame, "event: snapshot") {
		t.Fatal(frame)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown waited for active stream")
	}
	_ = response.Body.Close()
	if m.EventSubscriberCount() != 0 {
		t.Fatal("shutdown retained subscriber")
	}
}

func TestSSEStalledWriterDoesNotBlockManager(t *testing.T) {
	m, token, d := sseManager(t)
	listener := &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: handlerWithStreamConfig(m, token, "localhost", time.Hour, 50*time.Millisecond)}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	defer func() { _ = srv.Close(); <-done }()
	client, server := net.Pipe()
	defer client.Close()
	select {
	case listener.connections <- server:
	case <-time.After(time.Second):
		t.Fatal("listener unavailable")
	}
	if _, err := io.WriteString(client, "GET /api/events HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer "+token+"\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if frame := readSSEFrame(t, bufio.NewReader(response.Body)); !strings.Contains(frame, "event: snapshot") {
		t.Fatal(frame)
	}
	registered := make(chan error, 1)
	go func() { registered <- m.Register(d) }()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("stalled stream blocked registration")
	}
	deadline := time.After(time.Second)
	for m.EventSubscriberCount() != 0 {
		select {
		case <-deadline:
			t.Fatal("write deadline did not release stalled stream")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
