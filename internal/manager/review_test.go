package manager

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestCompletedProcessStopDoesNotMutateState(t *testing.T) {
	m := &Manager{}
	e := &entry{state: "stopped"}
	p := &process{done: make(chan struct{}), pgid: 0}
	close(p.done)
	if result := m.terminate(e, p); !result.confirmed || result.err != nil {
		t.Fatalf("completed process result=%+v", result)
	}
	if e.state != "stopped" {
		t.Fatalf("stale state %q", e.state)
	}
}

func TestCloseNaturalExitRace(t *testing.T) {
	for range 12 {
		m, d := scriptFixture(t, "exit 0")
		add(t, m, d)
		if err := m.Start(d.ID); err != nil {
			t.Fatal(err)
		}
		results := make(chan error, 2)
		go func() { results <- m.Close() }()
		go func() { results <- m.Close() }()
		for range 2 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		v, _ := m.Get(d.ID)
		if v.State != "stopped" {
			t.Fatalf("state %s", v.State)
		}
	}
}
func TestCloseReportsSameForcedOutcome(t *testing.T) {
	m, d := scriptFixture(t, "trap '' TERM; echo ready; while :; do sleep 1; done")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "ready", func() bool { s, _ := m.Logs(d.ID); return strings.Contains(s, "ready") })
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.Close() }()
	}
	wg.Wait()
	close(results)
	var first string
	for err := range results {
		if err == nil {
			t.Fatal("forced shutdown error lost")
		}
		if first == "" {
			first = err.Error()
		} else if err.Error() != first {
			t.Fatalf("inconsistent close errors: %q and %q", first, err)
		}
	}
}
func TestNaturalLeaderExitGivesDescendantGrace(t *testing.T) {
	m, d := scriptFixture(t, "sh -c 'trap \"echo graceful > child-term; exit 0\" TERM; echo ready > child-ready; while :; do sleep 1; done' & while [ ! -f child-ready ]; do sleep 0.01; done; exit 0")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, d.ID, "stopped")
	b, err := os.ReadFile(filepath.Join(d.WorkDir, "child-term"))
	if err != nil || !strings.Contains(string(b), "graceful") {
		t.Fatalf("descendant TERM marker: %q %v", b, err)
	}
}
func TestUnconfirmedCleanupRetainsOwnership(t *testing.T) {
	m, d := scriptFixture(t, "exit 0")
	add(t, m, d)
	p := &process{done: make(chan struct{}), exited: true}
	close(p.done)
	m.mu.Lock()
	e := m.entries[d.ID]
	e.proc = p
	e.state = "failed"
	e.err = "process-group cleanup failed"
	m.mu.Unlock()
	if err := m.Restart(d.ID); err == nil {
		t.Fatal("replacement started despite unconfirmed cleanup")
	}
	m.mu.Lock()
	same := e.proc == p
	m.mu.Unlock()
	if !same {
		t.Fatal("ownership released")
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("start accepted while ownership retained")
	}
}
func TestClosedPidfdCannotSignalGroup(t *testing.T) {
	if err := pidfdSignalGroup(-1, syscall.SIGTERM); err == nil {
		t.Fatal("invalid pidfd accepted")
	}
}

func TestDisappearingProcEntryIsIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1", "2"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	found, err := scanGroupMembers(entries, func(path string) ([]byte, error) {
		if strings.Contains(path, "/1/stat") {
			return nil, &os.PathError{Op: "read", Path: path, Err: syscall.ESRCH}
		}
		return []byte("2 (owned-child) S 1 42 0 0 0 0"), nil
	}, 42, 99)
	if err != nil {
		t.Fatalf("transient disappearance rejected: %v", err)
	}
	if !found {
		t.Fatal("live owned member not found after disappearance")
	}
}
