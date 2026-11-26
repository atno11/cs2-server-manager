package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1
const operationTimeout = 2 * time.Second

// Definition is the persistent subset of a managed instance.
type Definition struct {
	ID         string
	Executable string
	WorkDir    string
	Port       int
	Args       []string
}

type Store struct {
	db       *sql.DB
	lock     *os.File
	once     sync.Once
	closeErr error
}

func privateFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
		return errors.New("database file must be a private regular file")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) {
		return errors.New("database file has wrong owner")
	}
	return nil
}
func sidecars(path string) error {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if err := privateFile(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
func rejectUnexpectedWAL(path string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return errors.New("unexpected WAL sidecar for DELETE-journal database")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
func openPrivate(path string) (*os.File, error) {
	if err := privateFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err := privateFile(path); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func Open(dir string) (_ *Store, err error) {
	lock, err := openPrivate(filepath.Join(dir, "instances.lock"))
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("manager runtime already owned: %w", err)
	}
	s := &Store{lock: lock}
	defer func() {
		if err != nil {
			_ = s.Close()
		}
	}()
	path := filepath.Join(dir, "instances.db")
	if err = sidecars(path); err != nil {
		return nil, err
	}
	if err = rejectUnexpectedWAL(path); err != nil {
		return nil, err
	}
	var f *os.File
	f, err = openPrivate(path)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	params := url.Values{}
	params.Add("_pragma", "busy_timeout(1000)")
	params.Add("_pragma", "journal_mode(DELETE)")
	params.Add("_pragma", "synchronous(FULL)")
	params.Add("_pragma", "foreign_keys(ON)")
	params.Add("_pragma", "temp_store(MEMORY)")
	uri += "?" + params.Encode()
	s.db, err = sql.Open("sqlite", uri)
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	if err = s.db.PingContext(ctx); err != nil {
		return nil, err
	}
	for _, p := range []struct{ read, want string }{
		{"PRAGMA busy_timeout", "1000"},
		{"PRAGMA journal_mode", "delete"},
		{"PRAGMA synchronous", "2"},
		{"PRAGMA foreign_keys", "1"},
		{"PRAGMA temp_store", "2"},
	} {
		var got string
		if err = s.db.QueryRowContext(ctx, p.read).Scan(&got); err != nil {
			return nil, err
		}
		if got != p.want {
			return nil, fmt.Errorf("SQLite setting %s ineffective: %s", p.read, got)
		}
	}
	if err = privateFile(path); err != nil {
		return nil, err
	}
	if err = sidecars(path); err != nil {
		return nil, err
	}
	if err = s.init(ctx); err != nil {
		return nil, err
	}
	var integrity string
	if err = s.db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil {
		return nil, err
	}
	if integrity != "ok" {
		return nil, errors.New("database integrity check failed")
	}
	return s, nil
}
func (s *Store) init(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion || version < 0 {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	if version == 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type IN ('table','view','trigger','index') AND name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("unversioned database has unexpected schema")
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, stmt := range []string{
			`CREATE TABLE instances (id TEXT PRIMARY KEY NOT NULL CHECK(length(id) BETWEEN 1 AND 64), executable TEXT NOT NULL, work_dir TEXT NOT NULL UNIQUE, port INTEGER NOT NULL UNIQUE CHECK(port BETWEEN 1 AND 65535), args_json TEXT NOT NULL CHECK(json_valid(args_json)))`,
			`CREATE TABLE migration_state (id INTEGER PRIMARY KEY CHECK(id=1), legacy_complete INTEGER NOT NULL CHECK(legacy_complete=1))`,
			`PRAGMA user_version=1`,
		} {
			if _, err = tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		if err = sidecars(filepath.Join(filepath.Dir(s.path()), "instances.db")); err != nil {
			return err
		}
		return tx.Commit()
	}
	return s.validateSchema(ctx)
}
func (s *Store) path() string { return filepath.Join(filepath.Dir(s.lock.Name()), "instances.db") }
func (s *Store) MigrationComplete(ctx context.Context) (bool, error) {
	var value int
	err := s.db.QueryRowContext(ctx, "SELECT legacy_complete FROM migration_state WHERE id=1").Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return value == 1, err
}
func (s *Store) Import(ctx context.Context, defs []Definition) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var marker, existing int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM migration_state").Scan(&marker); err != nil {
		return err
	}
	if marker != 0 {
		return errors.New("legacy import already completed")
	}
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM instances").Scan(&existing); err != nil {
		return err
	}
	if existing != 0 {
		return errors.New("uncompleted migration has existing instances")
	}
	for _, d := range defs {
		if err = insert(ctx, tx, d); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO migration_state(id,legacy_complete) VALUES(1,1)"); err != nil {
		return err
	}
	if err = sidecars(s.path()); err != nil {
		return err
	}
	return tx.Commit()
}
func insert(ctx context.Context, tx *sql.Tx, d Definition) error {
	args, err := json.Marshal(d.Args)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO instances(id,executable,work_dir,port,args_json) VALUES(?,?,?,?,?)", d.ID, d.Executable, d.WorkDir, d.Port, string(args))
	return err
}
func (s *Store) Put(d Definition) error {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = insert(ctx, tx, d); err != nil {
		return err
	}
	if err = sidecars(s.path()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM instances WHERE id=?", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if err = sidecars(s.path()); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Load(ctx context.Context) ([]Definition, error) {
	var count, maxID, maxExec, maxWork, maxArgs, invalidTypes int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(max(length(CAST(id AS BLOB))),0),coalesce(max(length(CAST(executable AS BLOB))),0),coalesce(max(length(CAST(work_dir AS BLOB))),0),coalesce(max(length(CAST(args_json AS BLOB))),0),coalesce(sum(CASE WHEN typeof(id)!='text' OR typeof(executable)!='text' OR typeof(work_dir)!='text' OR typeof(port)!='integer' OR typeof(args_json)!='text' THEN 1 ELSE 0 END),0) FROM instances`).Scan(&count, &maxID, &maxExec, &maxWork, &maxArgs, &invalidTypes); err != nil {
		return nil, err
	}
	if count > 32 || maxID > 64 || maxExec > 4096 || maxWork > 4096 || maxArgs > 16*1024 || invalidTypes != 0 {
		return nil, errors.New("stored database definitions exceed limits")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id,executable,work_dir,port,args_json FROM instances ORDER BY id LIMIT 33")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	defs := make([]Definition, 0, count)
	for rows.Next() {
		var d Definition
		var args string
		if err = rows.Scan(&d.ID, &d.Executable, &d.WorkDir, &d.Port, &args); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(args), &d.Args); err != nil {
			return nil, err
		}
		defs = append(defs, d)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(defs) != count {
		return nil, errors.New("stored database changed during load")
	}
	return defs, nil
}
func (s *Store) Close() error {
	s.once.Do(func() {
		var errs []error
		if s.db != nil {
			errs = append(errs, s.db.Close())
		}
		if s.lock != nil {
			errs = append(errs, syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN), s.lock.Close())
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}
