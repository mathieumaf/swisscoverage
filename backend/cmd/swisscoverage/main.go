// Command swisscoverage is the project's backend: one program with one
// command per job. docs/architecture.md lists the commands and says which of
// them exist.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `Usage: swisscoverage <command> [arguments]

Commands:
  serve    Serve the public API, which only reads.
  migrate  Bring the database schema up to date.
  ingest   Load the snapshots of the archive that the database does not hold yet.
  parse    Read a file of OFCOM's mobile stations and print what it contains.
  openapi  Print the contract of the API, as openapi.json holds it.

serve, migrate and ingest read the address of the database from DATABASE_URL.
Run "swisscoverage <command> -h" for the arguments of a command.
`

func main() {
	// An interruption cancels what is running, which rolls back the
	// transaction in progress.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	status := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(status)
}

// run executes one command and returns the exit status: 0 when it succeeded,
// 1 when it failed, 2 when it was called the wrong way.
func run(
	ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer,
	getenv func(string) string,
) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr, getenv)
	case "openapi":
		return runOpenAPI(args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(ctx, args[1:], stdout, stderr, getenv)
	case "ingest":
		return runIngest(ctx, args[1:], stdout, stderr, getenv)
	case "parse":
		return runParse(args[1:], stdin, stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "swisscoverage: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

// parseArgs parses the arguments of a command, which takes operands
// arguments after its flags. When the command must stop there, it returns
// false with the exit status: 0 after -h, which printed the usage, and 2 when
// the command was called the wrong way.
func parseArgs(flags *flag.FlagSet, args []string, operands int) (int, bool) {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	if flags.NArg() != operands {
		flags.Usage()
		return 2, false
	}
	return 0, true
}
