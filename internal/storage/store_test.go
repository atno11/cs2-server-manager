package storage

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "private?#dir")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}
func testDef(id string, port int) Definition {
	return Definition{ID: id, Executable: "/trusted/cs2", WorkDir: "/work/" + id, Port: port, Args: []string{"first", "second"}}
}
func TestSchemaAndSettings(t *testing.T) {
	s, dir := openTestStore(t)
	ctx := context.Background()
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatalf("version=%d err=%v", version, err)
	}
	for pragma, want := range map[string]string{"journal_mode": "delete", "synchronous": "2", "foreign_keys": "1", "busy_timeout": "1000"} {
		var got string
		if err := s.db.QueryRowContext(ctx, "PRAGMA "+pragma).Scan(&got); err != nil || got != want {
			t.Fatalf("%s=%s want %s err=%v", pragma, got, want, err)
		}
	}
	fi, err := os.Stat(filepath.Join(dir, "instances.db"))
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("database not private", err)
	}
	if err := s.Import(ctx, nil); err != nil {
		t.Fatal(err)
	}
	complete, err := s.MigrationComplete(ctx)
	if err != nil || !complete {
		t.Fatal("missing completed migration", err)
	}
	if err := s.Put(testDef("one", 27015)); err != nil {
		t.Fatal(err)
	}
	for _, d := range []Definition{testDef("one", 27016), testDef("two", 27015), {ID: "three", Executable: "/trusted/cs2", WorkDir: "/work/one", Port: 27017}} {
		if err := s.Put(d); err == nil {
			t.Fatalf("constraint accepted %+v", d)
		}
	}
	for _, d := range []Definition{testDef("badzero", 0), testDef("badlarge", 65536), testDef("", 27018)} {
		if err := s.Put(d); err == nil {
			t.Fatalf("invalid row accepted: %+v", d)
		}
	}
}
func TestTransactionalImportRetryAndReplay(t *testing.T) {
	s, dir := openTestStore(t)
	ctx := context.Background()
	if err := s.Import(ctx, []Definition{testDef("one", 27015), testDef("one", 27016)}); err == nil {
		t.Fatal("duplicate import accepted")
	}
	defs, err := s.Load(ctx)
	if err != nil || len(defs) != 0 {
		t.Fatal("partial import", err, defs)
	}
	done, err := s.MigrationComplete(ctx)
	if err != nil || done {
		t.Fatal("partial migration marker", err)
	}
	original := []Definition{testDef("one", 27015), testDef("two", 27016)}
	if err := s.Import(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := s.Import(ctx, nil); err == nil {
		t.Fatal("completed migration replayed")
	}
	if err := s.Delete("one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("two"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	done, err = reopened.MigrationComplete(ctx)
	if err != nil || !done {
		t.Fatal("completion forgotten", err)
	}
	defs, err = reopened.Load(ctx)
	if err != nil || len(defs) != 0 {
		t.Fatal("deleted definitions reappeared", err)
	}
}
func TestInvalidDatabaseAndPaths(t *testing.T) {
	s, dir := openTestStore(t)
	path := filepath.Join(dir, "instances.db")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("unsafe db accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	journal := path + "-journal"
	if err := os.Symlink(filepath.Join(dir, "other"), journal); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("journal symlink accepted")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal, []byte("unsafe"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("unsafe journal accepted")
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	wal := path + "-wal"
	if err := os.WriteFile(wal, []byte("unexpected"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("unexpected WAL sidecar accepted")
	}
	if err := os.Remove(wal); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("newer schema accepted")
	}
}
func TestSingleManagerLockAndClose(t *testing.T) {
	s, dir := openTestStore(t)
	if _, err := Open(dir); err == nil {
		t.Fatal("second manager acquired lock")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
}
func TestBusyDatabaseIsBounded(t *testing.T) {
	s, dir := openTestStore(t)
	if err := s.Import(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.Join(dir, "instances.db")}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO instances(id,executable,work_dir,port,args_json) VALUES('holder','/x','/holder',27015,'[]')"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = s.Put(testDef("blocked", 27016))
	if err == nil {
		t.Fatal("locked write succeeded")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("locked write exceeded bound: %v", time.Since(start))
	}
}
func TestParameterValuesAndInvalidSchema(t *testing.T) {
	s, dir := openTestStore(t)
	if err := s.Import(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	d := testDef("quoted'--", 27015)
	if err := s.Put(d); err != nil {
		t.Fatal(err)
	}
	defs, err := s.Load(context.Background())
	if err != nil || len(defs) != 1 || defs[0].ID != d.ID {
		t.Fatal("parameterized value lost", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "instances.db"), []byte(strings.Repeat("corrupt", 100)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("corrupt database accepted")
	}
}
func TestUnversionedUnexpectedSchemaRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instances.db")
	raw, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec("CREATE TABLE unexpected_data(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("unversioned unexpected schema accepted")
	}
}
func TestJournalAndLockPermissions(t *testing.T) {
	s, dir := openTestStore(t)
	if err := s.Import(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	lockInfo, err := os.Stat(filepath.Join(dir, "instances.lock"))
	if err != nil || lockInfo.Mode().Perm() != 0600 {
		t.Fatal("lock file not private", err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO instances(id,executable,work_dir,port,args_json) VALUES('journal','/x','/journal',27015,'[]')"); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(dir, "instances.db-journal")
	fi, err := os.Lstat(journal)
	if err != nil {
		t.Fatal("DELETE journal missing during write", err)
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		t.Fatalf("journal mode=%v", fi.Mode())
	}
}
