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
	"sync"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/manager"
)

type controlledStreamWriter struct {
	mu             sync.Mutex
	status         int
	writes         int
	started        chan struct{}
	blocked        chan struct{}
	release        chan struct{}
	deadlineErr    error
	deadlineCalls  int
	failDeadlineAt int
}

func (w *controlledStreamWriter) Header() http.Header  { return make(http.Header) }
func (w *controlledStreamWriter) WriteHeader(code int) { w.mu.Lock(); w.status = code; w.mu.Unlock() }
func (w *controlledStreamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.writes++
	n := w.writes
	w.mu.Unlock()
	if n == 1 && w.started != nil {
		close(w.started)
	}
	if n == 2 {
		close(w.blocked)
		<-w.release
	}
	return len(p), nil
}
func (w *controlledStreamWriter) Flush() {}
func (w *controlledStreamWriter) SetWriteDeadline(time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deadlineCalls++
	if w.failDeadlineAt == 0 || w.deadlineCalls == w.failDeadlineAt {
		return w.deadlineErr
	}
	return nil
}

func TestSSEDeadlineSetupFailureRejectsBeforeHeaders(t *testing.T) {
	m, token, _ := sseManager(t)
	w := &controlledStreamWriter{blocked: make(chan struct{}), release: make(chan struct{}), deadlineErr: io.ErrClosedPipe}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/events", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	handler(m, token, "127.0.0.1:8080").ServeHTTP(w, r)
	if w.status != 503 || w.writes != 0 || m.EventSubscriberCount() != 0 {
		t.Fatalf("unsafe stream setup: status=%d writes=%d subscribers=%d", w.status, w.writes, m.EventSubscriberCount())
	}
}

func TestSSEDeadlineRefreshFailureEndsCommittedStream(t *testing.T) {
	m, token, _ := sseManager(t)
	w := &controlledStreamWriter{blocked: make(chan struct{}), release: make(chan struct{}), deadlineErr: io.ErrClosedPipe, failDeadlineAt: 2}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/events", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	handler(m, token, "127.0.0.1:8080").ServeHTTP(w, r)
	if w.status != 200 || w.writes != 1 || w.deadlineCalls != 2 || m.EventSubscriberCount() != 0 {
		t.Fatalf("failed refresh retained stream: status=%d writes=%d deadline calls=%d subscribers=%d", w.status, w.writes, w.deadlineCalls, m.EventSubscriberCount())
	}
}

func TestSSEOverflowKeepsHTTPHandlerReservations(t *testing.T) {
	m, token, d := sseManager(t)
	h := handler(m, token, "127.0.0.1:8080")
	writers := make([]*controlledStreamWriter, 8)
	done := make([]chan struct{}, 8)
	released := false
	for i := range writers {
		w := &controlledStreamWriter{started: make(chan struct{}), blocked: make(chan struct{}), release: make(chan struct{})}
		writers[i] = w
		done[i] = make(chan struct{})
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/events", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		go func(i int) { h.ServeHTTP(w, r); close(done[i]) }(i)
	}
	defer func() {
		if !released {
			for _, w := range writers {
				close(w.release)
			}
			for _, ch := range done {
				<-ch
			}
		}
	}()
	for _, w := range writers {
		select {
		case <-w.started:
		case <-time.After(3 * time.Second):
			t.Fatal("stream did not start")
		}
	}
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	for _, w := range writers {
		select {
		case <-w.blocked:
		case <-time.After(3 * time.Second):
			t.Fatal("stream did not reach blocked write")
		}
	}
	for i := 0; i < 65; i++ {
		if err := m.Remove(d.ID); err != nil {
			t.Fatal(err)
		}
		if err := m.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	if got := m.EventSubscriberCount(); got != 0 {
		t.Fatalf("overflow retained %d broker subscribers", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/events", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+token)
	out := httptest.NewRecorder()
	result := make(chan struct{})
	go func() { h.ServeHTTP(out, request); close(result) }()
	select {
	case <-result:
	case <-time.After(time.Second):
		cancel()
		<-result
		t.Fatal("ninth HTTP handler entered")
	}
	if out.Code != 503 {
		t.Fatalf("ninth HTTP handler entered: %d", out.Code)
	}
	admin := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/instances", nil)
	admin.Header.Set("Authorization", "Bearer "+token)
	adminOut := httptest.NewRecorder()
	h.ServeHTTP(adminOut, admin)
	if adminOut.Code != 200 {
		t.Fatalf("administrative request blocked by stream reservations: %d", adminOut.Code)
	}
	for _, w := range writers {
		close(w.release)
	}
	for _, ch := range done {
		<-ch
	}
	released = true
	ctx2, cancel2 := context.WithCancel(context.Background())
	req2 := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/events", nil).WithContext(ctx2)
	req2.Header.Set("Authorization", "Bearer "+token)
	out2 := &controlledStreamWriter{started: make(chan struct{}), blocked: make(chan struct{}), release: make(chan struct{})}
	done2 := make(chan struct{})
	go func() { h.ServeHTTP(out2, req2); close(done2) }()
	select {
	case <-out2.started:
	case <-time.After(time.Second):
		cancel2()
		<-done2
		t.Fatal("stream slot was not released")
	}
	cancel2()
	<-done2
}

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
