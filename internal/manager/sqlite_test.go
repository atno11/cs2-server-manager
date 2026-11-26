package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/atno11/cs2-server-manager/internal/storage"
)

type failingStore struct {
	base     definitionStore
	put, del error
	closeErr error
}
type blockingStore struct {
	base        definitionStore
	entered     chan struct{}
	release     chan struct{}
	blockDelete bool
}

func (b *blockingStore) Put(d storage.Definition) error {
	if !b.blockDelete {
		close(b.entered)
		<-b.release
	}
	return b.base.Put(d)
}
func (b *blockingStore) Delete(id string) error {
	if b.blockDelete {
		close(b.entered)
		<-b.release
	}
	return b.base.Delete(id)
}
func (b *blockingStore) Close() error { return b.base.Close() }

func (f *failingStore) Put(d storage.Definition) error {
	if f.put != nil {
		return f.put
	}
	return f.base.Put(d)
}
func (f *failingStore) Delete(id string) error {
	if f.del != nil {
		return f.del
	}
	return f.base.Delete(id)
}
func (f *failingStore) Close() error { return errors.Join(f.base.Close(), f.closeErr) }

func TestSQLiteRegisterRemoveFailureAtomicity(t *testing.T) {
	m, d, _ := policyFixture(t)
	failure := errors.New("injected persistence failure")
	wrapper := &failingStore{base: m.store, put: failure}
	m.store = wrapper
	if err := m.Register(d); !errors.Is(err, failure) {
		t.Fatalf("put error=%v", err)
	}
	if len(m.List()) != 0 {
		t.Fatal("failed registration published")
	}
	wrapper.put = nil
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	wrapper.del = failure
	if err := m.Remove(d.ID); !errors.Is(err, failure) {
		t.Fatalf("delete error=%v", err)
	}
	if len(m.List()) != 1 {
		t.Fatal("failed removal erased instance")
	}
	wrapper.del = nil
	if err := m.Remove(d.ID); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("removal not published")
	}
}
func TestSQLiteLegacyMigrationAndReplay(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "server.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho ready; while :; do sleep 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	workA := filepath.Join(root, "a")
	workB := filepath.Join(root, "b")
	for _, work := range []string{workA, workB} {
		if err := os.Mkdir(work, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	defs := []Definition{{ID: "a", Executable: exe, WorkDir: workA, Port: 27015, Args: []string{"first", "second"}}, {ID: "b", Executable: exe, WorkDir: workB, Port: 27016, Args: []string{"third"}}}
	original, err := json.MarshalIndent(defs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(data, "instances.json")
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	m, err := New(data, policy)
	if err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) != 2 || !slices.Equal(list[0].Args, defs[0].Args) || !slices.Equal(list[1].Args, defs[1].Args) {
		t.Fatal("migration changed definitions")
	}
	if err := m.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("legacy file changed")
	}
	m2, err := New(data, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	if len(m2.List()) != 1 || m2.List()[0].ID != "b" {
		t.Fatal("removed instance reimported")
	}
	if err := m2.Remove("b"); err != nil {
		t.Fatal(err)
	}
	if err := m2.Close(); err != nil {
		t.Fatal(err)
	}
	m3, err := New(data, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer m3.Close()
	if len(m3.List()) != 0 {
		t.Fatal("completed empty database reimported JSON")
	}
}
func TestSQLiteInvalidLegacyRetryAndNoFallback(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(data, "instances.json")
	if err := os.WriteFile(legacy, []byte(`[{"id":"x","ID":"y"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	if _, err := New(data, policy); err == nil {
		t.Fatal("invalid legacy accepted")
	}
	original := []byte(`[]`)
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(data, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("invalid stale JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	m2, err := New(data, policy)
	if err != nil {
		t.Fatal("completed SQLite fell back to stale JSON", err)
	}
	defer m2.Close()
	if len(m2.List()) != 0 {
		t.Fatal("stale JSON loaded")
	}
}
func TestSQLiteSingleManagerOwnershipAndConcurrentRegistration(t *testing.T) {
	m, d, root := policyFixture(t)
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	if _, err := New(filepath.Join(root, "data"), policy); err == nil {
		t.Fatal("second manager opened same runtime")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- m.Register(d) }()
	}
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 || len(m.List()) != 1 {
		t.Fatalf("successes=%d instances=%d", successes, len(m.List()))
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(filepath.Join(root, "data"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.List()) != 1 {
		t.Fatal("definition missing after reopen")
	}
}
func TestSQLiteStoredDataRemainsValidated(t *testing.T) {
	m, d, root := policyFixture(t)
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(d.ID); err != nil {
		t.Fatal(err)
	}
	bad := storage.Definition{ID: "bad", Executable: d.Executable, WorkDir: d.WorkDir, Port: 27015, Args: []string{"-port"}}
	if err := s.Put(bad); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	if _, err := New(filepath.Join(root, "data"), policy); err == nil {
		t.Fatal("unsafe stored definition accepted")
	}
}
func TestSQLiteRuntimeStateRestoresStopped(t *testing.T) {
	m, d := scriptFixture(t, "echo ready; trap 'exit 0' TERM; while :; do sleep 1; done")
	if err := m.Register(d); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(d.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, d.ID, "running")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{d.WorkDir}, WorkRoots: []string{d.WorkDir}}
	reopened, err := New(filepath.Join(d.WorkDir, "data"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	v, err := reopened.Get(d.ID)
	if err != nil || v.State != "stopped" {
		t.Fatalf("restored state=%q err=%v", v.State, err)
	}
}
func TestSQLiteLegacyDuplicateDefinitionsRejected(t *testing.T) {
	_, d, root := policyFixture(t)
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	for _, tc := range []struct {
		name   string
		modify func(*Definition)
	}{
		{"id", func(x *Definition) { x.ID = d.ID; x.Port++ }},
		{"port", func(x *Definition) { x.ID = "two" }},
		{"directory", func(x *Definition) { x.ID = "two"; x.Port++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := d
			tc.modify(&other)
			data := filepath.Join(t.TempDir(), "data")
			if err := os.Mkdir(data, 0700); err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal([]Definition{d, other})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(data, "instances.json"), b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := newWithPreflight(data, policy, func() error { return nil }); err == nil {
				t.Fatal("duplicate legacy definitions accepted")
			}
		})
	}
}
func TestSQLiteLegacyOversizeAndPermission(t *testing.T) {
	_, d, root := policyFixture(t)
	data := filepath.Join(root, "migration-data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(data, "instances.json")
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	if err := os.WriteFile(legacy, []byte(strings.Repeat("x", 512*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newWithPreflight(data, policy, func() error { return nil }); err == nil {
		t.Fatal("oversized legacy accepted")
	}
	if err := os.Chmod(legacy, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := newWithPreflight(data, policy, func() error { return nil }); err == nil {
		t.Fatal("unsafe legacy file accepted")
	}
}
func TestSQLiteCloseErrorSharedAndLockReleased(t *testing.T) {
	m, d, root := policyFixture(t)
	injected := errors.New("storage close failure")
	m.store = &failingStore{base: m.store, closeErr: injected}
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- m.Close() }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if !errors.Is(err, injected) {
			t.Fatalf("close result=%v", err)
		}
	}
	policy := Policy{Executables: []string{d.Executable}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	reopened, err := New(filepath.Join(root, "data"), policy)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	defer reopened.Close()
}
func TestSQLiteCorruptDatabaseDoesNotFallBack(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "fake.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(data, "instances.json")
	if err := os.WriteFile(legacy, []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	policy := Policy{Executables: []string{exe}, ExecutableRoots: []string{root}, WorkRoots: []string{root}}
	m, err := New(data, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("not a SQLite database")
	if err := os.WriteFile(filepath.Join(data, "instances.db"), corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(data, policy); err == nil {
		t.Fatal("corrupt SQLite fell back to valid JSON")
	}
	after, err := os.ReadFile(filepath.Join(data, "instances.db"))
	if err != nil || !bytes.Equal(after, corrupt) {
		t.Fatal("corrupt database was rewritten")
	}
}
func TestSQLiteMutationRacingClose(t *testing.T) {
	for _, blockDelete := range []bool{false, true} {
		name := "register"
		if blockDelete {
			name = "remove"
		}
		t.Run(name, func(t *testing.T) {
			m, d, _ := policyFixture(t)
			if blockDelete {
				if err := m.Register(d); err != nil {
					t.Fatal(err)
				}
			}
			b := &blockingStore{base: m.store, entered: make(chan struct{}), release: make(chan struct{}), blockDelete: blockDelete}
			m.store = b
			mutation := make(chan error, 1)
			go func() {
				if blockDelete {
					mutation <- m.Remove(d.ID)
				} else {
					mutation <- m.Register(d)
				}
			}()
			select {
			case <-b.entered:
			case <-time.After(2 * time.Second):
				t.Fatal("mutation did not enter storage")
			}
			closed := make(chan error, 1)
			go func() { closed <- m.Close() }()
			close(b.release)
			select {
			case err := <-mutation:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("mutation blocked")
			}
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("close blocked")
			}
			if err := m.Register(d); err == nil {
				t.Fatal("mutation accepted after close")
			}
		})
	}
}
