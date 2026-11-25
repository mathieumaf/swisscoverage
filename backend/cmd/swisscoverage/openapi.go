package main

import (
	"errors"
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
	// The routes are only described: nothing is read.
	contract, err := api.New(nil, slog.New(slog.DiscardHandler)).OpenAPI()
	if err == nil {
		_, err = stdout.Write(contract)
	}
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage openapi: %v\n", err)
		return 1
	}
	return 0
}
