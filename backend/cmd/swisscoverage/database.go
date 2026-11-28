package main

import (
	"context"
	"errors"

	"github.com/mathieumaf/swisscoverage/backend/internal/store"
)

// openStore connects to the database named by DATABASE_URL.
func openStore(ctx context.Context, getenv func(string) string) (*store.Store, error) {
	address, err := databaseURL(getenv)
	if err != nil {
		return nil, err
	}
	return store.Open(ctx, address)
}

// databaseURL returns the address of the database, which DATABASE_URL holds.
func databaseURL(getenv func(string) string) (string, error) {
	address := getenv("DATABASE_URL")
	if address == "" {
		return "", errDatabaseURL
	}
	return address, nil
}

var errDatabaseURL = errors.New("DATABASE_URL is not set: it holds the address of the database, " +
	"such as postgres://user:password@host:5432/name, with the password percent-encoded")
