package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

const testToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestClientEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"http://example.com:80", "https://127.0.0.1:80", "http://127.0.0.1:80/", "http://127.0.0.1:80?", "http://127.0.0.1:80#", "http://user:pass@127.0.0.1:80", "http://127.0.0.1:0", "http://localhost:80"} {
		if _, err := New(endpoint, testToken); err == nil {
			t.Errorf("accepted endpoint %q", endpoint)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:8080", "http://[::1]:8080"} {
		c, err := New(endpoint, testToken)
		if err != nil {
			t.Fatalf("%s: %v", endpoint, err)
		}
		c.CloseIdleConnections()
	}
	if _, err := New("http://127.0.0.1:80", "secret"); err == nil {
		t.Fatal("accepted short credential")
	}
	c, err := New("http://127.0.0.1:80", testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	if _, err = c.Register(context.Background(), apitypes.Definition{ID: "one", Executable: strings.Repeat("x", 4097)}); err == nil {
		t.Fatal("oversized definition reached transport")
	}
}

func TestClientRESTBoundsStatusRedirectAndProxy(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			t.Error("missing bearer header")
		}
		switch r.URL.Path {
		case "/api/instances":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `[{"id":"one","port":27015,"state":"stopped"}]`)
		case "/api/instances/one":
			w.WriteHeader(404)
		case "/api/instances/two":
			http.Redirect(w, r, "http://example.com/", http.StatusFound)
		case "/api/instances/one/logs":
			w.Header().Set("X-Log-Start", "2")
			w.Header().Set("X-Log-End", "5")
			io.WriteString(w, "abc")
		case "/api/instances/bad/logs":
			w.Header().Set("X-Log-Start", "0")
			w.Header().Set("X-Log-End", "3")
			io.WriteString(w, "ab")
		case "/api/instances/huge/logs":
			io.WriteString(w, strings.Repeat("x", 65537))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	c, err := New(srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	list, err := c.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != "one" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err = c.Get(context.Background(), "one"); !IsStatus(err, 404) {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatal("status error exposed credential")
	}
	if _, err = c.Get(context.Background(), "two"); !IsStatus(err, 302) {
		t.Fatalf("redirect followed: %v", err)
	}
	log, err := c.Logs(context.Background(), "one")
	if err != nil || string(log.Bytes) != "abc" || log.Start != 2 || log.End != 5 {
		t.Fatalf("log: %+v %v", log, err)
	}
	if _, err = c.Logs(context.Background(), "huge"); err == nil {
		t.Fatal("oversized log accepted")
	}
	if _, err = c.Logs(context.Background(), "bad"); err == nil {
		t.Fatal("invalid log offsets accepted")
	}
	if requests != 6 {
		t.Fatalf("requests=%d", requests)
	}
	if _, err = c.Get(context.Background(), "one/../../escape"); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}

type deadlineConn struct{ net.Conn }

func (*deadlineConn) SetReadDeadline(time.Time) error { return nil }
func testStream(input string) *Stream {
	return &Stream{body: io.NopCloser(strings.NewReader(input)), reader: bufioReader(input), conn: &deadlineConn{}}
}
func bufioReader(input string) *bufio.Reader { return bufio.NewReader(strings.NewReader(input)) }

func TestStreamParsingSequenceAndBounds(t *testing.T) {
	frame := func(id int, kind, data string) string {
		return fmt.Sprintf("id: %d\r\nevent: %s\r\ndata: %s\r\n\r\n", id, kind, data)
	}
	s := testStream(": heartbeat\r\n\r\n" + frame(8, "snapshot", `{"instances":[]}`) + frame(9, "instance.log", `{"id":"one","start":0,"end":2,"bytes":"YWI="}`) + frame(10, "future.event", `{"extra":true}`))
	defer s.Close()
	e, err := s.Next()
	if err != nil || e.Type != "snapshot" || e.Sequence != 8 {
		t.Fatalf("snapshot: %+v %v", e, err)
	}
	e, err = s.Next()
	if err != nil || e.Log == nil || string(e.Log.Bytes) != "ab" {
		t.Fatalf("log: %+v %v", e, err)
	}
	e, err = s.Next()
	if err != nil || e.Type != "future.event" || len(e.Raw) == 0 {
		t.Fatalf("unknown: %+v %v", e, err)
	}
	for _, tc := range []struct {
		name, input string
		want        error
	}{
		{"missing snapshot", frame(1, "instance.state", `{"instance":{"id":"one"}}`), ErrInvalidEvent},
		{"invalid ID", "id: token\nevent: snapshot\ndata: {\"instances\":[]}\n\n", ErrInvalidEvent},
		{"oversize", `data: ` + strings.Repeat("x", maxSSEFrame) + "\n\n", ErrInvalidEvent},
		{"incomplete", "id: 0\nevent: snapshot\ndata: {\"instances\":[]}", ErrStreamInterrupted},
		{"malformed JSON", frame(0, "snapshot", `{"instances":`), ErrInvalidEvent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := testStream(tc.input)
			defer reader.Close()
			_, err := reader.Next()
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	gap := testStream(frame(0, "snapshot", `{"instances":[]}`) + frame(2, "instance.removed", `{"id":"one"}`))
	defer gap.Close()
	if _, err = gap.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err = gap.Next(); !errors.Is(err, ErrEventGap) {
		t.Fatalf("gap: %v", err)
	}
	badLog := testStream(frame(0, "snapshot", `{"instances":[]}`) + frame(1, "instance.log", `{"id":"one","start":0,"end":3,"bytes":"YWI="}`))
	defer badLog.Close()
	if _, err = badLog.Next(); err != nil {
		t.Fatal(err)
	}
	if _, err = badLog.Next(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid log: %v", err)
	}
}

type oneByteReader struct{ data []byte }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	p[0] = r.data[0]
	r.data = r.data[1:]
	return 1, nil
}
func TestStreamFragmentedUTF8AndMultilineData(t *testing.T) {
	text := ": heartbeat\r\n\r\nid: 0\r\nevent: snapshot\r\ndata: {\"instances\":\r\ndata: []}\r\n\r\nid: 1\nevent: future.event\ndata: {\"text\":\"é\"}\n\n"
	r := &oneByteReader{data: []byte(text)}
	s := &Stream{body: io.NopCloser(r), reader: bufio.NewReader(r), conn: &deadlineConn{}}
	defer s.Close()
	first, err := s.Next()
	if err != nil || first.Snapshot == nil {
		t.Fatalf("multiline snapshot: %+v %v", first, err)
	}
	second, err := s.Next()
	if err != nil || !bytes.Contains(second.Raw, []byte("é")) {
		t.Fatalf("UTF-8 event: %+v %v", second, err)
	}
}

func TestLogReconciliationAndStaleTickets(t *testing.T) {
	s := NewState()
	if err := s.Apply(Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{{ID: "one", State: "stopped", Port: 1}}}}); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 2, End: 4, Bytes: []byte("cd")}}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(ticket, LogSnapshot{Bytes: []byte("abc"), Start: 0, End: 3}); err != nil {
		t.Fatal(err)
	}
	log, ok := s.Log()
	if !ok || string(log.Bytes) != "abcd" {
		t.Fatalf("reconciled: %+v", log)
	}
	if err = s.Apply(Event{Type: "instance.removed", RemovedID: "one"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "instance.registered", Instance: &apitypes.Instance{ID: "one", Port: 1, State: "stopped"}}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(ticket, LogSnapshot{Bytes: []byte("old"), Start: 0, End: 3}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("old ticket: %v", err)
	}
	newTicket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(newTicket, LogSnapshot{Bytes: []byte("new"), Start: 0, End: 3}); err != nil {
		t.Fatal(err)
	}
	log, _ = s.Log()
	if string(log.Bytes) != "new" {
		t.Fatalf("new incarnation: %q", log.Bytes)
	}
	if err = log.Apply(apitypes.LogEvent{ID: "one", Start: 5, End: 6, Bytes: []byte("x")}); !errors.Is(err, ErrLogGap) {
		t.Fatalf("gap: %v", err)
	}
	newTicket, err = s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{{ID: "one", Port: 1, State: "stopped"}}}}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(newTicket, LogSnapshot{Bytes: []byte("old"), End: 3}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("reconnect ticket: %v", err)
	}
}

func TestStreamAuthenticationCancellationAndIncompleteFrame(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		io.WriteString(w, "id: 0\nevent: snapshot\ndata: {\"instances\":[]}\n\n")
		w.(http.Flusher).Flush()
		io.WriteString(w, "id: 1\nevent: instance.state\ndata: {")
		w.(http.Flusher).Flush()
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	c, err := New(srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	s, err := c.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Next(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("stream did not reach partial frame")
	}
	done := make(chan error, 1)
	go func() { _, err := s.Next(); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled stream read succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled read did not stop")
	}
}

func TestLogBufferBoundAndOverlap(t *testing.T) {
	initial := bytes.Repeat([]byte("x"), 65536)
	b, err := NewLogBuffer("one", LogSnapshot{Bytes: initial, End: uint64(len(initial))})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Apply(apitypes.LogEvent{ID: "one", Start: 65535, End: 65538, Bytes: []byte("xyz")}); err != nil {
		t.Fatal(err)
	}
	if len(b.Bytes) != 65536 || b.Start != 2 || b.End != 65538 || string(b.Bytes[len(b.Bytes)-2:]) != "yz" {
		t.Fatalf("bounded overlap: start=%d end=%d length=%d", b.Start, b.End, len(b.Bytes))
	}
	if err = b.Apply(apitypes.LogEvent{ID: "one", Start: 65535, End: 65538, Bytes: []byte("xyz")}); err != nil {
		t.Fatal(err)
	}
}

func TestStateRejectsOversizedDirectEvents(t *testing.T) {
	s := NewState()
	large := make([]apitypes.Instance, 33)
	for i := range large {
		large[i] = apitypes.Instance{ID: fmt.Sprintf("id%d", i)}
	}
	if err := s.Apply(Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: large}}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("oversized snapshot: %v", err)
	}
	if err := s.Apply(Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{{ID: "one", Port: 1, State: "stopped"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginLog("one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 0, End: 2049, Bytes: make([]byte, 2049)}}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("oversized log event: %v", err)
	}
}

func TestLogByteOffsetsAndRecoveryBacklog(t *testing.T) {
	b, err := NewLogBuffer("one", LogSnapshot{Bytes: []byte("é"), End: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.Apply(apitypes.LogEvent{ID: "one", Start: 2, End: 6, Bytes: []byte("🙂")}); err != nil {
		t.Fatal(err)
	}
	if b.End != 6 || len(b.Bytes) != 6 || string(b.Bytes) != "é🙂" {
		t.Fatalf("byte offsets: %+v", b)
	}
	s := NewState()
	if err = s.Apply(Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{{ID: "one", Port: 1, State: "stopped"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginLog("one"); err != nil {
		t.Fatal(err)
	}
	event := Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 0, End: 1, Bytes: []byte("x")}}
	for i := 0; i < 128; i++ {
		if err = s.Apply(event); err != nil {
			t.Fatalf("queued %d: %v", i, err)
		}
	}
	if err = s.Apply(event); !errors.Is(err, ErrLogBacklog) {
		t.Fatalf("unbounded recovery queue: %v", err)
	}
}

func TestMutationNetworkFailureDoesNotRetry(t *testing.T) {
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes.Add(1)
			h, ok := w.(http.Hijacker)
			if !ok {
				t.Error("missing hijacker")
				return
			}
			conn, _, err := h.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `[{"id":"one","port":27015,"state":"stopped"}]`)
	}))
	defer srv.Close()
	c, err := New(srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	_, err = c.Register(context.Background(), apitypes.Definition{ID: "one", Port: 27015})
	if err == nil {
		t.Fatal("ambiguous network outcome reported success")
	}
	if writes.Load() != 1 {
		t.Fatalf("mutation retried %d times", writes.Load())
	}
	list, err := c.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != "one" {
		t.Fatalf("authoritative recovery: %+v %v", list, err)
	}
}

func TestClientAllAdministrativeMethods(t *testing.T) {
	var got []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/instances" {
			w.WriteHeader(201)
		}
		io.WriteString(w, `{"id":"one","port":27015,"state":"stopped"}`)
	}))
	defer srv.Close()
	c, err := New(srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	ctx := context.Background()
	if _, err = c.Register(ctx, apitypes.Definition{ID: "one", Port: 27015}); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Start(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Stop(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Restart(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err = c.Remove(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /api/instances", "POST /api/instances/one/start", "POST /api/instances/one/stop", "POST /api/instances/one/restart", "DELETE /api/instances/one"}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("methods: %v", got)
	}
}
