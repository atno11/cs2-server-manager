package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqlite "modernc.org/sqlite"
)

type schemaColumn struct {
	name, kind       string
	notNull, primary int
}

func (s *Store) validateSchema(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT type,name,tbl_name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return err
	}
	objects := map[string]bool{}
	for rows.Next() {
		var kind, name, table string
		if err = rows.Scan(&kind, &name, &table); err != nil {
			break
		}
		if kind != "table" || table != name || (name != "instances" && name != "migration_state") {
			err = fmt.Errorf("unexpected database schema object %q", name)
			break
		}
		objects[name] = true
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if !objects["instances"] || !objects["migration_state"] {
		return fmt.Errorf("database schema incomplete")
	}
	if err := s.checkColumns(ctx, "instances", []schemaColumn{{"id", "TEXT", 1, 1}, {"executable", "TEXT", 1, 0}, {"work_dir", "TEXT", 1, 0}, {"port", "INTEGER", 1, 0}, {"args_json", "TEXT", 1, 0}}); err != nil {
		return err
	}
	if err := s.checkColumns(ctx, "migration_state", []schemaColumn{{"id", "INTEGER", 0, 1}, {"legacy_complete", "INTEGER", 1, 0}}); err != nil {
		return err
	}
	for _, column := range []string{"work_dir", "port"} {
		if err := s.checkUniqueIndex(ctx, column); err != nil {
			return err
		}
	}
	var instanceIndexes, migrationIndexes int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_index_list('instances')`).Scan(&instanceIndexes); err != nil {
		return err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_index_list('migration_state')`).Scan(&migrationIndexes); err != nil {
		return err
	}
	if instanceIndexes != 3 || migrationIndexes != 0 {
		return fmt.Errorf("unexpected database indexes")
	}
	for _, table := range []string{"instances", "migration_state"} {
		var foreignKeys int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_list('`+table+`')`).Scan(&foreignKeys); err != nil {
			return err
		}
		if foreignKeys != 0 {
			return fmt.Errorf("unexpected foreign key in %s", table)
		}
	}
	var bad int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM migration_state WHERE id!=1 OR legacy_complete!=1`).Scan(&bad); err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("invalid migration metadata")
	}
	return s.checkConstraints(ctx)
}

func (s *Store) checkColumns(ctx context.Context, table string, want []schemaColumn) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_xinfo("+table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	got := []schemaColumn{}
	for rows.Next() {
		var cid, notNull, primary, hidden int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primary, &hidden); err != nil {
			return err
		}
		if cid != len(got) || hidden != 0 || defaultValue.Valid {
			return fmt.Errorf("incompatible %s column %s", table, name)
		}
		got = append(got, schemaColumn{name, strings.ToUpper(kind), notNull, primary})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(got) != len(want) {
		return fmt.Errorf("incompatible %s columns", table)
	}
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("incompatible %s column %s", table, got[i].name)
		}
	}
	return nil
}

func (s *Store) checkUniqueIndex(ctx context.Context, column string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA index_list(instances)`)
	if err != nil {
		return err
	}
	var indexes []string
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err = rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			break
		}
		if unique == 1 && partial == 0 {
			indexes = append(indexes, name)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, name := range indexes {
		var count int
		var indexed string
		// Index names come from SQLite's schema, so quote them as identifiers.
		query := `SELECT count(*),max(name) FROM pragma_index_info('` + strings.ReplaceAll(name, "'", "''") + `')`
		if err := s.db.QueryRowContext(ctx, query).Scan(&count, &indexed); err != nil {
			return err
		}
		if count == 1 && indexed == column {
			return nil
		}
	}
	return fmt.Errorf("missing unique index on %s", column)
}

func (s *Store) checkConstraints(ctx context.Context) error {
	// Probe CHECK constraints inside a transaction. No probe data is committed.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var freePort int
	if err := tx.QueryRowContext(ctx, `WITH RECURSIVE ports(p) AS (SELECT 1 UNION ALL SELECT p+1 FROM ports WHERE p<33) SELECT p FROM ports WHERE p NOT IN (SELECT port FROM instances) LIMIT 1`).Scan(&freePort); err != nil {
		return err
	}
	valid := fmt.Sprint(freePort)
	probes := []string{
		`INSERT INTO instances VALUES('', '/x', '/probe-'||hex(randomblob(16)), ` + valid + `, '[]')`,
		`INSERT INTO instances VALUES('` + strings.Repeat("a", 65) + `', '/x', '/probe-'||hex(randomblob(16)), ` + valid + `, '[]')`,
		`INSERT INTO instances VALUES('probe-'||hex(randomblob(16)), '/x', '/probe-'||hex(randomblob(16)), 0, '[]')`,
		`INSERT INTO instances VALUES('probe-'||hex(randomblob(16)), '/x', '/probe-'||hex(randomblob(16)), 65536, '[]')`,
		`INSERT INTO instances VALUES('probe-'||hex(randomblob(16)), '/x', '/probe-'||hex(randomblob(16)), ` + valid + `, 'invalid')`,
		`INSERT INTO migration_state VALUES(2, 1)`,
	}
	for _, probe := range probes {
		if _, err := tx.ExecContext(ctx, probe); err == nil {
			return fmt.Errorf("database schema constraint missing")
		} else if !isConstraintError(err) {
			return err
		}
	}
	var markerCount int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM migration_state").Scan(&markerCount); err != nil {
		return err
	}
	probe := `INSERT INTO migration_state VALUES(1, 2)`
	if markerCount != 0 {
		probe = `UPDATE migration_state SET legacy_complete=2 WHERE id=1`
	}
	if _, err := tx.ExecContext(ctx, probe); err == nil {
		return fmt.Errorf("migration constraint missing")
	} else if !isConstraintError(err) {
		return err
	}
	return nil
}

func isConstraintError(err error) bool {
	var sqlErr *sqlite.Error
	return errors.As(err, &sqlErr) && sqlErr.Code()&0xff == 19
}
