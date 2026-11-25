package manager

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func scriptFixture(t *testing.T, body string) (*Manager, Definition) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "server.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, Definition{ID: "one", Executable: exe, WorkDir: root, Port: 27015}
}
func eventually(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
func add(t *testing.T, m *Manager, d Definition) {
	t.Helper()
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
}
func TestSimultaneousIndependentAndConcurrentStarts(t *testing.T) {
	m, a := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	b := a
	b.ID = "two"
	b.Port++
	b.WorkDir = filepath.Join(a.WorkDir, "two")
	if err := os.Mkdir(b.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	add(t, m, a)
	add(t, m, b)
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.Start("one") }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful starts=%d", success)
	}
	if err := m.Start("two"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "two ready", func() bool { s, _ := m.Logs("two"); return strings.Contains(s, "ready") })
	v, _ := m.Get("two")
	if v.State != "running" {
		t.Fatal(v.State)
	}
	if err := m.Restart("one"); err != nil {
		t.Fatal(err)
	}
	v, _ = m.Get("two")
	if v.State != "running" {
		t.Fatal("restart changed other instance")
	}
	if err := m.Stop("one"); err != nil {
		t.Fatal(err)
	}
	v, _ = m.Get("two")
	if v.State != "running" {
		t.Fatal("stop changed other instance")
	}
}
func TestConcurrentStopsAndConflicts(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "ready", func() bool { s, _ := m.Logs(d.ID); return strings.Contains(s, "ready") })
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.Stop(d.ID) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful stops=%d", success)
	}
	waitState(t, m, d.ID, "stopped")
	if err := m.Restart(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("duplicate start accepted")
	}
	if err := m.Remove(d.ID); err == nil {
		t.Fatal("active removal accepted")
	}
}
func TestExitClassificationAndLaunchRecovery(t *testing.T) {
	for _, tc := range []struct{ name, body, state string }{{"zero", "exit 0", "stopped"}, {"nonzero", "exit 7", "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			m, d := scriptFixture(t, tc.body)
			add(t, m, d)
			if err := m.Start(d.ID); err != nil {
				t.Fatal(err)
			}
			waitState(t, m, d.ID, tc.state)
			v, _ := m.Get(d.ID)
			if tc.state == "failed" && v.Error == "" {
				t.Fatal("missing exit error")
			}
		})
	}
	m, d := scriptFixture(t, "echo ready; while :; do sleep 1; done")
	add(t, m, d)
	if err := os.Remove(d.Executable); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err == nil {
		t.Fatal("missing executable accepted")
	}
	if err := os.WriteFile(d.Executable, []byte("#!/bin/sh\necho ready; while :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(d.ID); err != nil {
		t.Fatal(err)
	}
}
func TestDefinitionOwnershipAndLogs(t *testing.T) {
	m, d := scriptFixture(t, "echo first; while :; do sleep 1; done")
	d.Args = []string{"original"}
	add(t, m, d)
	d.Args[0] = "changed"
	v, _ := m.Get("one")
	if v.Args[0] != "original" {
		t.Fatal("input alias changed definition")
	}
	v.Args[0] = "changed"
	v2, _ := m.Get("one")
	if v2.Args[0] != "original" {
		t.Fatal("output alias changed definition")
	}
	list := m.List()
	list[0].Args[0] = "changed"
	v2, _ = m.Get("one")
	if v2.Args[0] != "original" {
		t.Fatal("list alias changed definition")
	}
	r := newRing()
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = r.Write([]byte(strings.Repeat("x", 20000))) }()
	}
	wg.Wait()
	if len(r.String()) != logLimit {
		t.Fatalf("log length %d", len(r.String()))
	}
	if err := m.Start("one"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "first log", func() bool { s, _ := m.Logs("one"); return strings.Contains(s, "first") })
	if err := m.Restart("one"); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Logs("one")
	if strings.Count(s, "first") < 1 {
		t.Fatal("restart lost log")
	}
}
func TestCloseConcurrentAndRejectsMutations(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); m.Close() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("close did not complete")
	}
	for _, op := range []func() error{func() error { return m.Start(d.ID) }, func() error { return m.Stop(d.ID) }, func() error { return m.Restart(d.ID) }, func() error { return m.Remove(d.ID) }, func() error {
		return m.Register(Definition{ID: "new", Executable: d.Executable, WorkDir: d.WorkDir, Port: 27016})
	}} {
		if err := op(); err == nil {
			t.Fatal("mutation accepted after close")
		}
	}
	v, _ := m.Get(d.ID)
	if v.State != "stopped" {
		t.Fatal(v.State)
	}
}
func TestForcedTerminationAndDescendants(t *testing.T) {
	m, d := scriptFixture(t, "trap '' TERM; sleep 30 & echo $! > child.pid; echo ready; while :; do sleep 1; done")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	pidFile := filepath.Join(d.WorkDir, "child.pid")
	eventually(t, "descendant pid", func() bool { _, err := os.Stat(pidFile); return err == nil })
	start := time.Now()
	err := m.Stop(d.ID)
	if err == nil || !strings.Contains(err.Error(), "forced") {
		t.Fatalf("forced stop error=%v", err)
	}
	if time.Since(start) > gracefulTimeout+forcedTimeout+time.Second {
		t.Fatal("stop exceeded bound")
	}
	v, _ := m.Get(d.ID)
	if v.State != "failed" {
		t.Fatal(v.State)
	}
	b, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(b)))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	eventually(t, "descendant exit", func() bool { return !liveProcess(pid) })
}
func liveProcess(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	parts := strings.SplitN(string(b), ") ", 2)
	return len(parts) < 2 || !strings.HasPrefix(parts[1], "Z ")
}
func TestRepeatedLifecycleAndUnrelatedProcess(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	add(t, m, d)
	unrelated := exec.Command("sleep", "30")
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unrelated.Process.Kill(); _ = unrelated.Wait() }()
	for range 5 {
		if err := m.Start(d.ID); err != nil {
			t.Fatal(err)
		}
		if err := m.Stop(d.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Kill(unrelated.Process.Pid, 0); err != nil {
		t.Fatalf("unrelated process was signaled: %v", err)
	}
}

func TestRestartConflictsAndIndependentInstance(t *testing.T) {
	m, a := scriptFixture(t, "trap '' TERM; echo ready; while :; do sleep 1; done")
	b := a
	b.ID = "two"
	b.Port++
	b.WorkDir = filepath.Join(a.WorkDir, "two")
	if err := os.Mkdir(b.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	add(t, m, a)
	add(t, m, b)
	if err := m.Start("one"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "ready", func() bool { s, _ := m.Logs("one"); return strings.Contains(s, "ready") })
	result := make(chan error, 1)
	go func() { result <- m.Restart("one") }()
	waitState(t, m, "one", "stopping")
	for _, op := range []func() error{func() error { return m.Start("one") }, func() error { return m.Stop("one") }, func() error { return m.Restart("one") }, func() error { return m.Remove("one") }} {
		if err := op(); err == nil {
			t.Fatal("conflicting operation accepted")
		}
	}
	started := time.Now()
	if err := m.Start("two"); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("other instance blocked by restart")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("confirmed forced restart failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restart timed out")
	}
	first, _ := m.Get("one")
	if first.State != "running" || !strings.Contains(first.LastTermination, "forced") {
		t.Fatalf("restart state=%s diagnostic=%q", first.State, first.LastTermination)
	}
	v, _ := m.Get("two")
	if v.State != "running" {
		t.Fatal("other instance changed")
	}
}
func TestExecFailureRecovery(t *testing.T) {
	m, d := scriptFixture(t, "exit 0")
	if err := os.WriteFile(d.Executable, []byte("invalid shell content without shebang\n"), 0700); err != nil {
		t.Fatal(err)
	}
	add(t, m, d)
	if err := m.Start(d.ID); err == nil {
		t.Fatal("exec format accepted")
	}
	v, _ := m.Get(d.ID)
	if v.State != "failed" || v.Error == "" {
		t.Fatalf("launch failure state: %+v", v)
	}
	if err := os.WriteFile(d.Executable, []byte("#!/bin/sh\necho ready; while :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop(d.ID); err != nil {
		t.Fatal(err)
	}
}
func TestStaleCompletionCannotReplaceState(t *testing.T) {
	m, d := scriptFixture(t, "exit 0")
	add(t, m, d)
	cmd := exec.Command("/bin/true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	old := &process{cmd: cmd, done: make(chan struct{})}
	newer := &process{done: make(chan struct{})}
	m.mu.Lock()
	e := m.entries[d.ID]
	e.proc = newer
	e.state = "running"
	m.mu.Unlock()
	m.wait(e, old)
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.proc != newer || e.state != "running" {
		t.Fatal("stale completion changed replacement")
	}
}
func TestCloseParallelBound(t *testing.T) {
	m, a := scriptFixture(t, "trap '' TERM; echo ready; while :; do sleep 1; done")
	for i := 0; i < 3; i++ {
		d := a
		d.ID = fmt.Sprintf("server%d", i)
		d.Port += i
		d.WorkDir = filepath.Join(a.WorkDir, d.ID)
		if err := os.Mkdir(d.WorkDir, 0700); err != nil {
			t.Fatal(err)
		}
		add(t, m, d)
		if err := m.Start(d.ID); err != nil {
			t.Fatal(err)
		}
		eventually(t, "ready", func() bool { s, _ := m.Logs(d.ID); return strings.Contains(s, "ready") })
	}
	start := time.Now()
	m.Close()
	elapsed := time.Since(start)
	if elapsed > gracefulTimeout+forcedTimeout+time.Second {
		t.Fatalf("sequential shutdown: %v", elapsed)
	}
	for _, v := range m.List() {
		if v.State != "failed" {
			t.Fatal(v.State)
		}
	}
}

func TestGracefulStopAndLogIsolation(t *testing.T) {
	m, a := scriptFixture(t, "trap 'echo graceful; exit 0' TERM; echo first-ready; while :; do sleep 1; done")
	b := a
	b.ID = "two"
	b.Port++
	b.WorkDir = filepath.Join(a.WorkDir, "two")
	if err := os.Mkdir(b.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	add(t, m, a)
	add(t, m, b)
	for _, id := range []string{"one", "two"} {
		if err := m.Start(id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"one", "two"} {
		eventually(t, id+" ready", func() bool { s, _ := m.Logs(id); return strings.Contains(s, "first-ready") })
	}
	if err := m.Stop("one"); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Logs("one")
	if !strings.Contains(s, "graceful") {
		t.Fatal("TERM handler did not run")
	}
	v, _ := m.Get("two")
	if v.State != "running" {
		t.Fatal("other instance stopped")
	}
	if err := m.Restart("one"); err != nil {
		t.Fatal(err)
	}
	s, _ = m.Logs("two")
	if strings.Contains(s, "graceful") {
		t.Fatal("logs crossed instances")
	}
}
func TestRepeatedCyclesDoNotAccumulateFDs(t *testing.T) {
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skip("proc fd inventory unavailable")
	}
	m, d := scriptFixture(t, "trap 'exit 0' TERM; echo ready; while :; do sleep 1; done")
	add(t, m, d)
	for range 12 {
		if err := m.Start(d.ID); err != nil {
			t.Fatal(err)
		}
		eventually(t, "ready", func() bool { s, _ := m.Logs(d.ID); return strings.Contains(s, "ready") })
		if err := m.Stop(d.ID); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before)+4 {
		t.Fatalf("file descriptors grew from %d to %d", len(before), len(after))
	}
}

func TestStartStopAndShutdownRaces(t *testing.T) {
	for range 8 {
		m, d := scriptFixture(t, "trap 'exit 0' TERM; echo ready; while :; do sleep 1; done")
		add(t, m, d)
		gate := make(chan struct{})
		startResult := make(chan error, 1)
		stopResult := make(chan error, 1)
		go func() { <-gate; startResult <- m.Start(d.ID) }()
		go func() { <-gate; stopResult <- m.Stop(d.ID) }()
		close(gate)
		startErr, stopErr := <-startResult, <-stopResult
		if startErr != nil {
			t.Fatalf("start failed: %v", startErr)
		}
		if stopErr == nil {
			v, _ := m.Get(d.ID)
			if v.State != "stopped" {
				t.Fatal(v.State)
			}
		}
		m.Close()
		v, _ := m.Get(d.ID)
		if v.State == "running" || v.State == "stopping" {
			t.Fatal("active after close")
		}
	}
}
func TestUnexpectedLeaderExitCleansDescendant(t *testing.T) {
	m, d := scriptFixture(t, "sleep 30 & echo $! > child.pid; exit 0")
	add(t, m, d)
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "leader completion", func() bool { v, _ := m.Get(d.ID); return v.State == "stopped" || v.State == "failed" })
	v, _ := m.Get(d.ID)
	if v.State != "stopped" {
		t.Fatalf("leader completion state=%s error=%s", v.State, v.Error)
	}
	b, err := os.ReadFile(filepath.Join(d.WorkDir, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "orphan descendant exit", func() bool { return !liveProcess(pid) })
}

func TestTerminationTimeoutIsObservable(t *testing.T) {
	m := &Manager{}
	e := &entry{state: "running"}
	p := &process{done: make(chan struct{}), pgid: 2147483647}
	started := time.Now()
	e.proc = p
	result := m.terminate(e, p)
	if result.err == nil || !strings.Contains(result.err.Error(), "timed out") {
		t.Fatalf("timeout error=%v", result.err)
	}
	if e.state != "failed" || !strings.Contains(e.err, "did not exit") {
		t.Fatalf("timeout state=%s error=%s", e.state, e.err)
	}
	if time.Since(started) > gracefulTimeout+forcedTimeout+2*time.Second {
		t.Fatal("timeout wait exceeded bound")
	}
}
