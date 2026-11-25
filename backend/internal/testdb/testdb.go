// Package testdb gives each test a database of its own in a real PostgreSQL
// with PostGIS, so that queries are tested against what runs them.
//
// The server is the one of deploy/compose.yaml. Another one can be named with
// SWISSCOVERAGE_TEST_DATABASE_URL: its role must be allowed to create
// databases. Tests never touch the database that address names. They create
// databases named test_* beside it, and drop them when they end.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// The server of deploy/compose.yaml.
	composeURL = "postgres://swisscoverage:swisscoverage@127.0.0.1:54320/swisscoverage"
	// An empty database with PostGIS, which each test's database is a copy
	// of. Creating the extension takes a second; copying takes far less.
	template = "swisscoverage_test_template"
	// Any number works, as long as nothing else on the server uses it.
	setupLock = 7765190040
)

// New returns the address of an empty database with PostGIS, created for
// this test and dropped when it ends.
//
// Without a server, the test is skipped on a developer's machine, and fails
// in CI or when SWISSCOVERAGE_TEST_DATABASE_URL names one.
func New(t *testing.T) string {
	t.Helper()
	return create(t, template)
}

// NewBare is New for a database without PostGIS, as a server gives one
// before anyone prepared it.
func NewBare(t *testing.T) string {
	t.Helper()
	return create(t, "template0")
}

func create(t *testing.T, from string) string {
	t.Helper()
	address, named := os.LookupEnv("SWISSCOVERAGE_TEST_DATABASE_URL")
	if !named {
		address = composeURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgx.Connect(ctx, address)
	if err != nil {
		if named || os.Getenv("CI") != "" {
			t.Fatalf("no test database: %v", err)
		}
		t.Skipf("no test database, start it with "+
			"`docker compose -f deploy/compose.yaml up -d --wait`: %v", err)
	}
	defer admin.Close(ctx)

	if from == template {
		if err := ensureTemplate(ctx, admin, address); err != nil {
			t.Fatalf("preparing the template of the test databases: %v", err)
		}
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "test_" + hex.EncodeToString(suffix)
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name+` TEMPLATE `+from); err != nil {
		t.Fatalf("creating the test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		admin, err := pgx.Connect(ctx, address)
		if err != nil {
			t.Errorf("dropping the test database: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, `DROP DATABASE `+name+` WITH (FORCE)`); err != nil {
			t.Errorf("dropping the test database: %v", err)
		}
	})
	created, err := withDatabase(address, name)
	if err != nil {
		t.Fatalf("the test database's address is not a URL: %v", err)
	}
	return created
}

// ensureTemplate creates the template once per server. Test binaries run
// side by side, hence the lock. The template is built under another name and
// renamed when complete, so that one left half-built is never used.
func ensureTemplate(ctx context.Context, admin *pgx.Conn, address string) error {
	if _, err := admin.Exec(ctx, `SELECT pg_advisory_lock($1)`, setupLock); err != nil {
		return err
	}
	defer admin.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, setupLock)

	var exists bool
	err := admin.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, template).Scan(&exists)
	if err != nil || exists {
		return err
	}
	building := template + "_building"
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS `+building+` WITH (FORCE)`); err != nil {
		return err
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+building); err != nil {
		return err
	}
	buildingAddress, err := withDatabase(address, building)
	if err != nil {
		return err
	}
	inside, err := pgx.Connect(ctx, buildingAddress)
	if err != nil {
		return err
	}
	_, err = inside.Exec(ctx, `CREATE EXTENSION postgis`)
	inside.Close(ctx)
	if err != nil {
		return err
	}
	_, err = admin.Exec(ctx, `ALTER DATABASE `+building+` RENAME TO `+template)
	return err
}

// withDatabase returns address pointed at another database of the same
// server. An address can also name its database in a parameter, which would
// win over the path: it is removed, so that nothing is written elsewhere.
func withDatabase(address, name string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + name
	values := parsed.Query()
	values.Del("dbname")
	values.Del("database")
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

// Query runs one statement in the database at address and returns its rows,
// each value as text. It is for assertions on what the code under test wrote.
func Query(t *testing.T, address, sql string, args ...any) [][]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// The simple protocol returns every value as text.
	args = append([]any{pgx.QueryExecModeSimpleProtocol}, args...)
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result [][]string
	for rows.Next() {
		raw := rows.RawValues()
		row := make([]string, len(raw))
		for index, value := range raw {
			if value == nil {
				row[index] = "NULL"
			} else {
				row[index] = string(value)
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
