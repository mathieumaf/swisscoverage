package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/mathieumaf/swisscoverage/backend/internal/api"
)

const openapiUsage = `Usage: swisscoverage openapi

Prints the contract of the API, as OpenAPI 3.1 in JSON. The repository keeps
it as openapi.json, and CI fails when the two differ. From backend/:

  go run ./cmd/swisscoverage openapi > ../openapi.json

It needs no database.
`

// runOpenAPI is the "openapi" command.
func runOpenAPI(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("openapi", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, openapiUsage) }
	if status, ok := parseArgs(flags, args, 0); !ok {
		return status
	}
	// The routes are only described: nothing is read, nothing is served.
	contract, err := api.New(nil, slog.New(slog.DiscardHandler), 0).OpenAPI()
	if err == nil {
		_, err = stdout.Write(contract)
	}
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage openapi: %v\n", err)
		return 1
	}
	return 0
}
