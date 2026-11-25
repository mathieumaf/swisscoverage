// Command swisscoverage is the project's backend: one program with one
// command per job. docs/architecture.md lists the commands and says which of
// them exist.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `Usage: swisscoverage <command> [arguments]

Commands:
  parse   Read a file of OFCOM's mobile stations and print what it contains.

Run "swisscoverage <command> -h" for the arguments of a command.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes one command and returns the exit status: 0 when it succeeded,
// 1 when it failed, 2 when it was called the wrong way.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
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
