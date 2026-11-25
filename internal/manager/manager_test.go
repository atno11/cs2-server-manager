package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Manager, string, string) {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho started\nwhile true; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, exe, root
}
func waitState(t *testing.T, m *Manager, id, state string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, _ := m.Get(id)
		if v.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	v, _ := m.Get(id)
	t.Fatalf("state %s, want %s", v.State, state)
}
func TestLifecyclePersistenceAndValidation(t *testing.T) {
	m, exe, root := fixture(t)
	a := Definition{ID: "one", Executable: exe, WorkDir: root, Port: 27015}
	b := Definition{ID: "two", Executable: exe, WorkDir: filepath.Join(root, "two"), Port: 27016}
	if err := os.Mkdir(b.WorkDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(a); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(b); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 2 {
		t.Fatal("missing definitions")
	}
	bad := b
	bad.ID = "three"
	bad.Port = a.Port
	if m.Register(bad) == nil {
		t.Fatal("duplicate port accepted")
	}
	bad.Port = 0
	if m.Register(bad) == nil {
		t.Fatal("invalid port accepted")
	}
	if err := m.Start("one"); err != nil {
		t.Fatal(err)
	}
	v, _ := m.Get("two")
	if v.State != "stopped" {
		t.Fatal("other instance changed")
	}
	if err := m.Restart("one"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "one", "running")
	if err := m.Stop("one"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "one", "stopped")
	m2, err := New(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.List()) != 2 {
		t.Fatal("definitions lost")
	}
	if err := m.Remove("one"); err != nil {
		t.Fatal(err)
	}
}
func TestUnexpectedExitAndLogs(t *testing.T) {
	m, _, root := fixture(t)
	exe := filepath.Join(root, "exit.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho tokenless-output\nexit 2\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Register(Definition{ID: "exit", Executable: exe, WorkDir: root, Port: 27017}); err != nil {
		t.Fatal(err)
	}
	if err := m.Start("exit"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, "exit", "failed")
	s, _ := m.Logs("exit")
	if !strings.Contains(s, "tokenless-output") {
		t.Fatal("output not captured")
	}
	r := newRing()
	_, _ = r.Write([]byte(strings.Repeat("x", logLimit+100)))
	if len(r.String()) != logLimit {
		t.Fatal("log not bounded")
	}
}
