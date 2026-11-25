package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/api"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
)

const serveUsage = `Usage: swisscoverage serve [arguments]

Serves the public API, which only reads from the database. It stops on an
interrupt, once the requests in progress are answered.

The address of the database is read from DATABASE_URL. The program's
connections refuse every write, whatever the role may do.

Arguments:
`

// Limits on each request, so that a slow or stuck client holds nothing for
// long.
const (
	// A query that takes longer is stopped by the database.
	statementTimeout = 10 * time.Second
	// Everything a request may wait for: the database and the response.
	requestTimeout = 20 * time.Second
	// How long the requests in progress have to finish after an interrupt.
	shutdownTimeout = 15 * time.Second
)

// runServe is the "serve" command.
func runServe(
	ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, serveUsage)
		flags.PrintDefaults()
	}
	listen := flags.String("listen", "127.0.0.1:8080",
		"`address` to listen on, as host:port; :8080 listens on every interface")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if err := serve(ctx, *listen, nil, stdout, stderr, getenv); err != nil {
		fmt.Fprintf(stderr, "swisscoverage serve: %v\n", err)
		return 1
	}
	return 0
}

// serve answers on address until ctx is cancelled. When ready is not nil, it
// receives the address listened on, which tests use with port 0.
func serve(
	ctx context.Context, address string, ready chan<- string,
	stdout, stderr io.Writer, getenv func(string) string,
) error {
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return errDatabaseURL
	}
	database, err := store.OpenReadOnly(ctx, databaseURL, statementTimeout)
	if err != nil {
		return err
	}
	defer database.Close()
	// The schema may be newer than the program, after a rollback: it stays
	// compatible with it.
	if err := database.CheckSchema(ctx); err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(stderr, nil))
	server := &http.Server{
		Handler: api.New(database, logger, requestTimeout),
		// The API reads no request body: headers are all it receives.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		// The list of every station is a few megabytes, for slow phones too.
		WriteTimeout:   2 * time.Minute,
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 16 << 10,
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		BaseContext:    func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Serving the API on http://%s/v1/\n", listener.Addr())
	if ready != nil {
		ready <- listener.Addr().String()
	}

	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		return fmt.Errorf("stopping: %w", err)
	}
	fmt.Fprintln(stdout, "Stopped.")
	return nil
}
