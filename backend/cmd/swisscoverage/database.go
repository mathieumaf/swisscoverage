package main

import (
	"context"
	"errors"

	"github.com/mathieumaf/swisscoverage/backend/internal/store"
)

// openStore connects to the database named by DATABASE_URL.
func openStore(ctx context.Context, getenv func(string) string) (*store.Store, error) {
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL is not set: it holds the address of the database, " +
			"such as postgres://user:password@host:5432/name")
	}
	return store.Open(ctx, databaseURL)
}
