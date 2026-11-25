package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestHTTPShutdownCoordinatesManager(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho ready\nwhile :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Register(manager.Definition{ID: "one", Executable: exe, WorkDir: dir, Port: 27015}); err != nil {
		t.Fatal(err)
	}
	listener := &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{Handler: handler(m)}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, listener, m) }()
	request := func(method, path string) *http.Response {
		t.Helper()
		client, server := net.Pipe()
		select {
		case listener.connections <- server:
		case <-time.After(time.Second):
			t.Fatal("listener unavailable")
		}
		if _, err := io.WriteString(client, method+" "+path+" HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(client), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		return resp
	}
	resp := request("POST", "/api/instances/one/start")
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("start status %d", resp.StatusCode)
	}
	resp = request("POST", "/api/instances/one/restart")
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("restart status %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("server shutdown timed out")
	}
	v, err := m.Get("one")
	if err != nil || v.State != "stopped" {
		t.Fatalf("manager not closed: %+v %v", v, err)
	}
	if err := m.Start("one"); err == nil {
		t.Fatal("start accepted after shutdown")
	}
}

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
	closeErr    error
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connections:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *pipeListener) Close() error   { l.once.Do(func() { close(l.closed) }); return l.closeErr }
func (l *pipeListener) Addr() net.Addr { return pipeAddr("pipe") }

type pipeAddr string

func (a pipeAddr) Network() string { return string(a) }
func (a pipeAddr) String() string  { return string(a) }

func TestHTTPShutdownErrorPropagates(t *testing.T) {
	m, err := manager.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	listener := &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{}), closeErr: errors.New("listener close failure")}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, listener, m) }()
	client, server := net.Pipe()
	select {
	case listener.connections <- server:
	case <-time.After(time.Second):
		t.Fatal("listener unavailable")
	}
	_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	_ = client.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "listener close failure") {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timed out")
	}
	if err := m.Start("none"); err == nil {
		t.Fatal("manager remained open")
	}
}
func TestHTTPShutdownDeadlinePropagates(t *testing.T) {
	m, err := manager.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	listener := &pipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	entered := make(chan struct{})
	released := make(chan struct{})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done(); close(released) })}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveWithTimeout(ctx, srv, listener, m, 30*time.Millisecond) }()
	client, server := net.Pipe()
	select {
	case listener.connections <- server:
	case <-time.After(time.Second):
		t.Fatal("listener unavailable")
	}
	_, _ = io.WriteString(client, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n")
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown deadline error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown blocked")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("active connection not closed")
	}
	_ = client.Close()
}
