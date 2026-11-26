package manager

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/events"
)

func nextEvent(t *testing.T, sub *events.Subscription) events.Event {
	t.Helper()
	select {
	case e := <-sub.Events():
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("event timeout")
		return events.Event{}
	}
}

func TestEventsSnapshotDuringCommittedRegistration(t *testing.T) {
	m, d, _ := policyFixture(t)
	wrapper := &blockingStore{base: m.store, entered: make(chan struct{}), release: make(chan struct{})}
	m.store = wrapper
	registered := make(chan error, 1)
	go func() { registered <- m.Register(d) }()
	<-wrapper.entered
	var wg sync.WaitGroup
	wg.Add(1)
	snapshots := make(chan events.Event, 1)
	errCh := make(chan error, 1)
	go func() {
		defer wg.Done()
		snapshot, sub, err := m.Subscribe()
		if err == nil {
			sub.Cancel()
			snapshots <- snapshot
		}
		errCh <- err
	}()
	close(wrapper.release)
	if err := <-registered; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	snapshot := <-snapshots
	if snapshot.Sequence != 1 || !strings.Contains(snapshot.Data, `"id":"one"`) {
		t.Fatalf("lost concurrent registration: %+v", snapshot)
	}
}

func TestEventsSnapshotFailureReleasesSubscriber(t *testing.T) {
	m, _, _ := policyFixture(t)
	m.mu.Lock()
	for i := 0; i < 32; i++ {
		id := strings.Repeat("x", 600) + string(rune('A'+i))
		m.entries[id] = &entry{def: Definition{ID: id, Port: 27015 + i}, state: "stopped"}
	}
	m.mu.Unlock()
	if _, _, err := m.Subscribe(); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
	if m.EventSubscriberCount() != 0 {
		t.Fatal("failed snapshot retained subscription")
	}
}

func TestEventsUnexpectedExitAndRestart(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	_, sub, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, sub); e.Type != "instance.state" || !strings.Contains(e.Data, `"state":"running"`) {
		t.Fatal(e)
	}
	if err := m.Restart(d.ID); err != nil {
		t.Fatal(err)
	}
	seen := []string{}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-sub.Events():
			if e.Type != "instance.state" {
				continue
			}
			var body struct {
				Instance EventInstance `json:"instance"`
			}
			if err := json.Unmarshal([]byte(e.Data), &body); err != nil {
				t.Fatal(err)
			}
			seen = append(seen, body.Instance.State)
			if body.Instance.State == "running" {
				if len(seen) < 3 || seen[0] != "stopping" || seen[1] != "stopped" {
					t.Fatalf("restart states: %v", seen)
				}
				goto restarted
			}
		case <-deadline:
			t.Fatalf("restart event timeout: %v", seen)
		}
	}
restarted:
	other, def := scriptFixture(t, "exit 7")
	if err := other.Register(def); err != nil {
		t.Fatal(err)
	}
	_, exitSub, err := other.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer exitSub.Cancel()
	if err := other.Start(def.ID); err != nil {
		t.Fatal(err)
	}
	states := []string{}
	deadline = time.After(3 * time.Second)
	for {
		select {
		case e := <-exitSub.Events():
			if e.Type != "instance.state" {
				continue
			}
			var body struct {
				Instance EventInstance `json:"instance"`
			}
			if err := json.Unmarshal([]byte(e.Data), &body); err != nil {
				t.Fatal(err)
			}
			states = append(states, body.Instance.State)
			if body.Instance.State == "failed" {
				if len(states) < 2 || states[0] != "running" || strings.Contains(e.Data, "exit status") {
					t.Fatalf("unexpected exit: %v %+v", states, e)
				}
				return
			}
		case <-deadline:
			t.Fatalf("missing unexpected-exit event: %v", states)
		}
	}
}
func noEvent(t *testing.T, sub *events.Subscription) {
	t.Helper()
	select {
	case e := <-sub.Events():
		t.Fatalf("unexpected event: %+v", e)
	default:
	}
}

func TestEventsCommittedMutationsAndSnapshot(t *testing.T) {
	m, d, _ := policyFixture(t)
	snapshot, sub, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	if snapshot.Sequence != 0 || snapshot.Type != "snapshot" || !strings.Contains(snapshot.Data, `"instances":[]`) {
		t.Fatalf("initial snapshot: %+v", snapshot)
	}
	failure := errors.New("storage failure")
	wrapper := &failingStore{base: m.store, put: failure}
	m.store = wrapper
	if err := m.Register(d); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	noEvent(t, sub)
	wrapper.put = nil
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	registered := nextEvent(t, sub)
	if registered.Type != "instance.registered" || registered.Sequence != 1 || strings.Contains(registered.Data, d.Executable) {
		t.Fatalf("registration event: %+v", registered)
	}
	fresh, boundary, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer boundary.Cancel()
	if fresh.Sequence != 1 || !strings.Contains(fresh.Data, `"id":"one"`) {
		t.Fatalf("fresh snapshot: %+v", fresh)
	}
	wrapper.del = failure
	if err := m.Remove(d.ID); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	noEvent(t, sub)
	wrapper.del = nil
	if err := m.Remove(d.ID); err != nil {
		t.Fatal(err)
	}
	removed := nextEvent(t, sub)
	if removed.Type != "instance.removed" || removed.Sequence != 2 {
		t.Fatalf("removal event: %+v", removed)
	}
	if nextEvent(t, boundary).Type != "instance.removed" {
		t.Fatal("snapshot subscriber lost later change")
	}
}

func TestEventsStateTransitionsAndLaunchFailure(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	_, sub, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if e := nextEvent(t, sub); e.Type != "instance.state" || !strings.Contains(e.Data, `"state":"running"`) {
		t.Fatalf("running event: %+v", e)
	}
	if err := m.Stop(d.ID); err != nil {
		t.Fatal(err)
	}
	states := []string{}
	deadline := time.After(3 * time.Second)
	for len(states) < 2 {
		select {
		case e := <-sub.Events():
			if e.Type == "instance.state" {
				var body struct {
					Instance EventInstance `json:"instance"`
				}
				if err := json.Unmarshal([]byte(e.Data), &body); err != nil {
					t.Fatal(err)
				}
				states = append(states, body.Instance.State)
			}
		case <-deadline:
			t.Fatalf("missing stop states: %v", states)
		}
	}
	if states[0] != "stopping" || states[1] != "stopped" {
		t.Fatalf("stop states: %v", states)
	}

	root := t.TempDir()
	exe := filepath.Join(root, "broken.sh")
	if err := os.WriteFile(exe, []byte("#!/does/not/exist\n"), 0700); err != nil {
		t.Fatal(err)
	}
	broken, err := New(filepath.Join(root, "data"), Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	def := Definition{ID: "broken", Executable: exe, WorkDir: root, Port: 27016}
	if err := broken.Register(def); err != nil {
		t.Fatal(err)
	}
	_, failedSub, err := broken.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer failedSub.Cancel()
	if err := broken.Start(def.ID); err == nil {
		t.Fatal("broken launch succeeded")
	}
	if e := nextEvent(t, failedSub); e.Type != "instance.state" || !strings.Contains(e.Data, `"state":"failed"`) || strings.Contains(e.Data, "does/not/exist") {
		t.Fatalf("failed launch event: %+v", e)
	}
}

func TestEventsSanitizedLogOffsetsAndTruncation(t *testing.T) {
	m, d, _ := policyFixture(t)
	m.policy.secrets = []string{"abcdsecret", "secret"}
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	_, sub, err := m.Subscribe()
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Cancel()
	m.mu.Lock()
	ring := m.entries[d.ID].log
	m.mu.Unlock()
	_, _ = ring.Write([]byte("prefix abcd"))
	_, _ = ring.Write([]byte("secret suffix"))
	ring.Flush()
	logEvents := []events.Event{}
	for {
		select {
		case e := <-sub.Events():
			logEvents = append(logEvents, e)
		default:
			goto drained
		}
	}
drained:
	if len(logEvents) == 0 {
		t.Fatal("no incremental log events")
	}
	var previous uint64
	var delivered strings.Builder
	for _, e := range logEvents {
		if e.Type != "instance.log" || strings.Contains(e.Data, "abcdsecret") || strings.Contains(e.Data, "secret suffix") || len(e.Data) > events.MaxPayload {
			t.Fatalf("unsafe log event: %+v", e)
		}
		var body struct {
			Start, End uint64
			Bytes      []byte
		}
		if err := json.Unmarshal([]byte(e.Data), &body); err != nil {
			t.Fatal(err)
		}
		if body.Start != previous || body.End-body.Start != uint64(len(body.Bytes)) || len(body.Bytes) > maxLogEventBytes {
			t.Fatalf("log offset gap: %+v", body)
		}
		delivered.Write(body.Bytes)
		previous = body.End
	}
	if strings.Contains(delivered.String(), "abcdsecret") || !strings.Contains(delivered.String(), "[REDACTED]") {
		t.Fatalf("incremental redaction: %q", delivered.String())
	}
	text, start, end, err := m.LogsSnapshot(d.ID)
	if err != nil || start != 0 || end != previous || !strings.Contains(text, "[REDACTED]") || strings.Contains(text, "abcdsecret") {
		t.Fatalf("log snapshot: %q %d %d %v", text, start, end, err)
	}
	_, _ = ring.Write([]byte(strings.Repeat("x", 70*1024)))
	_, start, end, err = m.LogsSnapshot(d.ID)
	if err != nil || end-start != logLimit || start == 0 {
		t.Fatalf("retained bound: %d %d %v", start, end, err)
	}
}

func TestEventsPartialSecretWaitsForFinalization(t *testing.T) {
	r := newRing("abcdsecret")
	var published strings.Builder
	r.onOutput = func(_ uint64, p []byte) { published.Write(p) }
	_, _ = r.Write([]byte("abcd"))
	if published.Len() != 0 {
		t.Fatalf("partial secret emitted early: %q", published.String())
	}
	_, _ = r.Write([]byte("secret"))
	r.Flush()
	if published.String() != "[REDACTED]" {
		t.Fatalf("split secret: %q", published.String())
	}
	r2 := newRing("abcdsecret")
	r2.onOutput = func(_ uint64, p []byte) { published.Write(p) }
	_, _ = r2.Write([]byte("abc"))
	if published.String() != "[REDACTED]" {
		t.Fatal("partial prefix emitted early")
	}
	r2.Flush()
	if published.String() != "[REDACTED]abc" {
		t.Fatalf("termination flush: %q", published.String())
	}
}
