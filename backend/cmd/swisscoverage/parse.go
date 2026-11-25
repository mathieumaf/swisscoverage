package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
)

const parseUsage = `Usage: swisscoverage parse [-json] <file>

Reads a file of OFCOM's mobile stations and prints the number of stations,
the counts per operator, kind and technology, and every value the parser did
not recognise. The file is "-" to read standard input.

Warnings do not change the exit status: the file was read.

Arguments:
`

// runParse is the "parse" command. It reads one file and writes nothing but
// its output.
func runParse(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("parse", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, parseUsage)
		flags.PrintDefaults()
	}
	asJSON := flags.Bool("json", false, "print every station and every warning as JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}

	data, err := readSource(flags.Arg(0), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage parse: %v\n", err)
		return 1
	}
	parsed, err := ofcom.ParseMobileStations(data)
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage parse: %v\n", err)
		return 1
	}

	// The buffered writer keeps the first error of a write and returns it
	// when it is flushed.
	output := bufio.NewWriter(stdout)
	if *asJSON {
		err = writeJSON(output, parsed)
	} else {
		writeSummary(output, parsed)
	}
	if err == nil {
		err = output.Flush()
	}
	if err != nil {
		fmt.Fprintf(stderr, "swisscoverage parse: %v\n", err)
		return 1
	}
	return 0
}

func readSource(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

// writeJSON prints the stations and the warnings as they are, for comparing
// two versions of the parser on the same file.
func writeJSON(output io.Writer, parsed ofcom.MobileStations) error {
	encoder := json.NewEncoder(output)
	// Labels are printed as published, without turning "<", ">" and "&"
	// into escape sequences.
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(parsed)
}

// writeSummary prints what a person needs to judge a file at a glance.
func writeSummary(output io.Writer, parsed ofcom.MobileStations) {
	perOperator := map[ofcom.Operator]int{}
	perKind := map[ofcom.StationKind]int{}
	perTechnology := map[ofcom.Technology]int{}
	for _, station := range parsed.Stations {
		perOperator[station.Operator]++
		perKind[station.Kind]++
		for _, technology := range station.Technologies {
			perTechnology[technology]++
		}
	}

	fmt.Fprintf(output, "%d stations, %d warnings\n", len(parsed.Stations), len(parsed.Warnings))

	writeCounts(output, "Per operator", ofcom.Operators(), perOperator)
	writeCounts(output, "Per kind", ofcom.StationKinds(), perKind)
	writeCounts(output, "Per technology (a station counts once for each one it carries)",
		ofcom.Technologies(), perTechnology)

	if len(parsed.Warnings) > 0 {
		fmt.Fprint(output, "\nWarnings\n")
		for _, warning := range parsed.Warnings {
			fmt.Fprintf(output, "  %s\n", warning)
		}
	}
}

// writeCounts prints one line per value, in the order given, with the names
// aligned on the left and the counts on the right.
func writeCounts[V ~string](output io.Writer, title string, values []V, counts map[V]int) {
	nameWidth, countWidth := 0, 0
	for _, value := range values {
		nameWidth = max(nameWidth, len(value))
		countWidth = max(countWidth, len(strconv.Itoa(counts[value])))
	}
	fmt.Fprintf(output, "\n%s\n", title)
	for _, value := range values {
		fmt.Fprintf(output, "  %-*s  %*d\n", nameWidth, value, countWidth, counts[value])
	}
}
