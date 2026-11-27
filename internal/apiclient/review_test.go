package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/atno11/cs2-server-manager/internal/apitypes"
)

type fixedRoundTripper struct{}

func (fixedRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}

// The child isolates the global replacement from other tests and packages.
func TestClientOwnsTransportWithoutDefaultTransport(t *testing.T) {
	if os.Getenv("CS2M_REVIEW_TRANSPORT_CHILD") == "1" {
		http.DefaultTransport = fixedRoundTripper{}
		c, err := New("http://127.0.0.1:8080", testToken)
		if err != nil {
			t.Fatal(err)
		}
		c.CloseIdleConnections()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestClientOwnsTransportWithoutDefaultTransport$")
	cmd.Env = append(os.Environ(), "CS2M_REVIEW_TRANSPORT_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("client depended on global transport: %v\n%s", err, out)
	}
}

func TestClientRejectsOversizedResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Fill", strings.Repeat("x", 20*1024))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer srv.Close()
	c, err := New(srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	if _, err = c.List(context.Background()); err == nil {
		t.Fatal("oversized response headers accepted")
	}
}

func TestClientTransportConfiguration(t *testing.T) {
	c, err := New("http://127.0.0.1:8080", testToken)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	if c.transport.Proxy != nil || c.transport.MaxResponseHeaderBytes != 16*1024 || c.transport.MaxConnsPerHost != 16 || c.transport.MaxIdleConnsPerHost != 4 {
		t.Fatalf("transport not independently bounded: header=%d connections=%d idle=%d", c.transport.MaxResponseHeaderBytes, c.transport.MaxConnsPerHost, c.transport.MaxIdleConnsPerHost)
	}
}

func TestSSERejectsInvalidRemovalAndLogIDs(t *testing.T) {
	for _, kind := range []string{"instance.removed", "instance.log"} {
		for _, id := range []string{"", strings.Repeat("x", 65), "bad/id", "bad id", "é"} {
			t.Run(kind+"/"+fmt.Sprintf("%q", id), func(t *testing.T) {
				var payload []byte
				if kind == "instance.removed" {
					payload, _ = json.Marshal(apitypes.Removed{ID: id})
				} else {
					payload, _ = json.Marshal(apitypes.LogEvent{ID: id, Start: 0, End: 1, Bytes: []byte("x")})
				}
				input := "id: 0\nevent: snapshot\ndata: {\"instances\":[]}\n\n" + "id: 1\nevent: " + kind + "\ndata: " + string(payload) + "\n\n"
				s := testStream(input)
				defer s.Close()
				if _, err := s.Next(); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Next(); !errors.Is(err, ErrInvalidEvent) {
					t.Fatalf("invalid ID %q accepted: %v", id, err)
				}
			})
		}
	}
}

func TestSSELogRequiresContractFields(t *testing.T) {
	for _, payload := range []string{`{"id":"one"}`, `{"id":"one","start":0,"end":0}`, `{"id":"one","start":null,"end":0,"bytes":""}`, `{"id":"one","start":0,"end":null,"bytes":""}`, `{"id":"one","start":0,"end":0,"bytes":null}`} {
		s := testStream("id: 0\nevent: snapshot\ndata: {\"instances\":[]}\n\n" + "id: 1\nevent: instance.log\ndata: " + payload + "\n\n")
		if _, err := s.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Next(); !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("missing/null log field accepted: %s (%v)", payload, err)
		}
		s.Close()
	}
	// Additive unknown fields remain compatible.
	s := testStream("id: 0\nevent: snapshot\ndata: {\"instances\":[]}\n\n" + "id: 1\nevent: instance.log\ndata: {\"id\":\"one\",\"start\":0,\"end\":1,\"bytes\":\"eA==\",\"future\":true}\n\n")
	defer s.Close()
	if _, err := s.Next(); err != nil {
		t.Fatal(err)
	}
	if e, err := s.Next(); err != nil || e.Log == nil || string(e.Log.Bytes) != "x" {
		t.Fatalf("compatible event rejected: %+v %v", e, err)
	}
}

func TestStateCallerRecoveryWorkflow(t *testing.T) {
	s := NewState()
	instance := apitypes.Instance{ID: "one", Port: 27015, State: "running"}
	snapshot := Event{Type: "snapshot", Snapshot: &apitypes.Snapshot{Instances: []apitypes.Instance{instance}}}
	if err := s.Apply(snapshot); err != nil {
		t.Fatal(err)
	}
	oldTicket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(oldTicket, LogSnapshot{Bytes: []byte("old"), End: 3}); err != nil {
		t.Fatal(err)
	}
	// A reconnect snapshot invalidates old logs. The event consumer begins
	// recovery before it advances to the next stream event.
	if err = s.Apply(snapshot); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	allowEvent := make(chan struct{})
	eventDone := make(chan error, 1)
	go func() {
		<-allowEvent
		eventDone <- s.Apply(Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 3, End: 4, Bytes: []byte("!")}})
	}()
	close(allowEvent)
	if err = <-eventDone; err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(oldTicket, LogSnapshot{Bytes: []byte("stale"), End: 5}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("delayed old response: %v", err)
	}
	if err = s.CompleteLog(ticket, LogSnapshot{Bytes: []byte("new"), End: 3}); err != nil {
		t.Fatal(err)
	}
	if log, ok := s.Log(); !ok || string(log.Bytes) != "new!" {
		t.Fatalf("reconnect log: %+v", log)
	}
	// Failed/cancelled fetches are abandoned by starting a new ticket.
	failed, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(failed, LogSnapshot{Bytes: []byte("old"), End: 3}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("failed fetch ticket: %v", err)
	}
	if err = s.CompleteLog(retry, LogSnapshot{Bytes: []byte("fresh"), End: 5}); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 7, End: 8, Bytes: []byte("x")}}); !errors.Is(err, ErrLogGap) {
		t.Fatalf("gap: %v", err)
	}
	gapTicket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(gapTicket, LogSnapshot{Bytes: []byte("recovered"), End: 9}); err != nil {
		t.Fatal(err)
	}
	backlogTicket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	queued := Event{Type: "instance.log", Log: &apitypes.LogEvent{ID: "one", Start: 0, End: 1, Bytes: []byte("r")}}
	for i := 0; i < 128; i++ {
		if err = s.Apply(queued); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Apply(queued); !errors.Is(err, ErrLogBacklog) {
		t.Fatalf("backlog: %v", err)
	}
	recovery, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(backlogTicket, LogSnapshot{Bytes: []byte("old"), End: 3}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("old backlog ticket: %v", err)
	}
	if err = s.CompleteLog(recovery, LogSnapshot{Bytes: []byte("complete"), End: 8}); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "instance.removed", RemovedID: "one"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(Event{Type: "instance.registered", Instance: &instance}); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(recovery, LogSnapshot{Bytes: []byte("old"), End: 3}); !errors.Is(err, ErrStaleLog) {
		t.Fatalf("same-ID recreation: %v", err)
	}
	newTicket, err := s.BeginLog("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteLog(newTicket, LogSnapshot{Bytes: []byte("new instance"), End: 12}); err != nil {
		t.Fatal(err)
	}
}
