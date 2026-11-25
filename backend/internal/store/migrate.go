package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// The schema is a list of SQL files, applied in order and never edited once
// merged: a change to the schema is a new file. There is no way back. A new
// version of the schema must keep working with the previous version of the
// program, so that rolling the program back needs no change to the database.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

var migrationName = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.sql$`)

// Any number works, as long as nothing else in the database uses it.
const migrationLock = 7765190041

type migration struct {
	version int
	name    string
	sql     string
}

// Migration reports what Migrate found and did.
type Migration struct {
	// Applied names the files that were applied, in order. It is empty when
	// the schema was already up to date.
	Applied []string
	// Database is the schema version of the database afterwards, and
	// Program the one this program was built with. A database ahead of the
	// program is not an error: the program was rolled back, and the schema
	// stays compatible with it.
	Database, Program int
}

// Migrate brings the schema up to the version this program was built with.
// Each file is applied in its own transaction. Several programs may call it
// at once: one works, the others wait and then find nothing to do.
func (s *Store) Migrate(ctx context.Context) (Migration, error) {
	migrations, err := loadMigrations()
	if err != nil {
		return Migration{}, err
	}
	result := Migration{Program: len(migrations)}

	// The lock belongs to the connection, so everything runs on one.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return result, fmt.Errorf("connecting to the database: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		return result, fmt.Errorf("waiting for other migrations: %w", err)
	}
	// Released even when ctx was cancelled meanwhile. Should this fail, the
	// lock goes with the connection.
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLock)

	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migration (
			version    integer     PRIMARY KEY,
			name       text        NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return result, fmt.Errorf("preparing the list of migrations: %w", err)
	}
	result.Database, err = schemaVersion(ctx, conn)
	if err != nil {
		return result, err
	}

	for _, m := range migrations {
		if m.version <= result.Database {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return result, fmt.Errorf("applying %s: %w", m.name, err)
		}
		// Without arguments, a file of several statements runs as one.
		_, err = tx.Exec(ctx, m.sql)
		if err == nil {
			_, err = tx.Exec(ctx,
				`INSERT INTO schema_migration (version, name) VALUES ($1, $2)`, m.version, m.name)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			tx.Rollback(context.WithoutCancel(ctx))
			return result, fmt.Errorf("applying %s: %w", m.name, err)
		}
		result.Applied = append(result.Applied, m.name)
		result.Database = m.version
	}
	return result, nil
}

// CheckSchema returns an error when the database is behind the schema this
// program was built with, which Migrate fixes.
func (s *Store) CheckSchema(ctx context.Context) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	var exists bool
	err = s.pool.QueryRow(ctx, `SELECT to_regclass('schema_migration') IS NOT NULL`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("reading the schema version: %w", err)
	}
	version := 0
	if exists {
		if version, err = schemaVersion(ctx, s.pool); err != nil {
			return err
		}
	}
	if version < len(migrations) {
		return fmt.Errorf(
			"the database schema is at version %d and this program needs version %d: run `swisscoverage migrate`",
			version, len(migrations))
	}
	return nil
}

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func schemaVersion(ctx context.Context, db queryer) (int, error) {
	var version int
	err := db.QueryRow(ctx, `SELECT coalesce(max(version), 0) FROM schema_migration`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}
	return version, nil
}

// loadMigrations returns the embedded files in order. Their numbers must be
// 1, 2, 3 and so on, so that a version says exactly which files were applied.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var migrations []migration
	// ReadDir returns the entries sorted by name.
	for _, entry := range entries {
		match := migrationName.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("migration %q is not named like 0001_what_it_does.sql", entry.Name())
		}
		version, _ := strconv.Atoi(match[1])
		if version != len(migrations)+1 {
			return nil, fmt.Errorf("migration %q should be number %04d", entry.Name(), len(migrations)+1)
		}
		sql, err := fs.ReadFile(migrationFiles, "migrations/"+entry.Name())
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(sql)})
	}
	return migrations, nil
}
