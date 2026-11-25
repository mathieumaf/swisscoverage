// Package store keeps the stations in PostgreSQL with PostGIS: the snapshots
// that were loaded, the sites, and how each site changed from one snapshot to
// the next. docs/architecture.md describes the model.
//
// Queries are written in queries.sql and turned into the typed code of the
// query package. The schema is the migrations directory.
package store

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is a connection to the database.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to the database at databaseURL, a PostgreSQL connection
// string, and checks that it answers.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("reading the database address: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connecting to the database: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the connections.
func (s *Store) Close() {
	s.pool.Close()
}
