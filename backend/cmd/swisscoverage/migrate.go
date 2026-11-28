package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

const migrateUsage = `Usage: swisscoverage migrate

Applies the schema migrations the database does not have yet, in order. It
does nothing when the schema is up to date, and several runs at once are safe.

The address of the database is read from DATABASE_URL.
`

// runMigrate is the "migrate" command.
func runMigrate(
	ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
) int {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, migrateUsage) }
	if status, ok := parseArgs(flags, args, 0); !ok {
		return status
	}

	database, err := openStore(ctx, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage migrate: %v\n", err)
		return 1
	}
	defer database.Close()

	result, err := database.Migrate(ctx)
	// What was applied before a failure stays applied: say so either way.
	for _, name := range result.Applied {
		fmt.Fprintf(stdout, "applied %s\n", name)
	}
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage migrate: %v\n", err)
		return 1
	}
	switch {
	case result.Database > result.Program:
		fmt.Fprintf(stdout,
			"The schema is at version %d, newer than this program (version %d). Nothing to do.\n",
			result.Database, result.Program)
	case len(result.Applied) == 0:
		fmt.Fprintf(stdout, "The schema is up to date (version %d).\n", result.Database)
	default:
		fmt.Fprintf(stdout, "The schema is now at version %d.\n", result.Database)
	}
	return 0
}
