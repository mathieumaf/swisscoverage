package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/archive"
	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
)

const ingestUsage = `Usage: swisscoverage ingest [arguments]

Loads into the database the snapshots of the archive that it does not hold
yet, oldest first. For each one it downloads the file OFCOM published, checks
it against the checksum of the archive's index, parses it, and records what
changed since the snapshot before. It does nothing when the database is up to
date, and it can be run again after a failure.

The address of the database is read from DATABASE_URL.

Arguments:
`

// How many warnings of one snapshot are printed in full.
const printedWarnings = 20

// A snapshot dated later than this from now is refused. Once loaded, it would
// make every real snapshot look older than the latest one, and stop them all.
const clockMargin = 24 * time.Hour

// runIngest is the "ingest" command.
func runIngest(
	ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string,
) int {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprint(stderr, ingestUsage)
		flags.PrintDefaults()
	}
	indexURL := flags.String("index", archive.DefaultIndexURL,
		"address of the archive's index")
	releasesURL := flags.String("releases", archive.DefaultReleasesURL,
		"address under which the archive's files are found, as <release>/<file>")
	allowDrop := flags.Bool("allow-drop", false,
		"load one snapshot even though it has less than half the sites of the one before")
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

	source := &archive.Client{
		IndexURL:    *indexURL,
		ReleasesURL: *releasesURL,
		HTTP:        &http.Client{Timeout: 5 * time.Minute},
	}
	if err := ingest(ctx, source, *allowDrop, stdout, stderr, getenv); err != nil {
		fmt.Fprintf(stderr, "swisscoverage ingest: %v\n", err)
		return 1
	}
	return 0
}

func ingest(
	ctx context.Context, source *archive.Client, allowDrop bool,
	stdout, stderr io.Writer, getenv func(string) string,
) error {
	database, err := openStore(ctx, getenv)
	if err != nil {
		return err
	}
	defer database.Close()
	if err := database.CheckSchema(ctx); err != nil {
		return err
	}

	index, err := source.Index(ctx)
	if err != nil {
		return err
	}
	loaded, err := database.Snapshots(ctx)
	if err != nil {
		return err
	}
	pending, err := pendingSnapshots(index, loaded)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Fprintf(stdout, "Nothing to load: the database holds the %s of the archive.\n",
			count(len(index), "snapshot"))
		return nil
	}

	previousSites := 0
	if len(loaded) > 0 {
		previousSites = loaded[len(loaded)-1].Sites
	}
	for _, snapshot := range pending {
		published := snapshot.PublishedAt.Format(time.RFC3339)
		if snapshot.PublishedAt.After(time.Now().Add(clockMargin)) {
			return fmt.Errorf("the snapshot published at %s is dated in the future", published)
		}
		data, err := source.Download(ctx, snapshot)
		if err != nil {
			return err
		}
		parsed, err := ofcom.ParseMobileStations(data)
		if err != nil {
			return fmt.Errorf("the snapshot published at %s: %w", published, err)
		}
		sites := len(parsed.Stations)
		if sites != snapshot.Sites {
			return fmt.Errorf("the snapshot published at %s has %d sites, and the index says %d",
				published, sites, snapshot.Sites)
		}
		// A file cut short, or emptied, would end every site it lacks. That
		// is recorded for good, so a person decides, one file at a time.
		if sites*2 < previousSites {
			if !allowDrop {
				return fmt.Errorf("the snapshot published at %s has %d sites, less than half the %d "+
					"of the one before: check the file, then run again with -allow-drop to load it",
					published, sites, previousSites)
			}
			allowDrop = false
		}

		result, err := database.LoadSnapshot(ctx, store.Snapshot{
			PublishedAt: snapshot.PublishedAt,
			SHA256:      snapshot.SHA256,
			Sites:       sites,
		}, parsed.Stations)
		if err != nil {
			return fmt.Errorf("the snapshot published at %s: %w", published, err)
		}
		previousSites = sites

		fmt.Fprintf(stdout, "%s: %d sites, %d appeared, %d disappeared, %d changed, %d warnings\n",
			published, sites, result.Appeared, result.Disappeared, result.Changed,
			len(parsed.Warnings)+len(result.Duplicates))
		printWarnings(stderr, published, parsed.Warnings, result.Duplicates)
	}
	fmt.Fprintf(stdout, "Loaded %s. The database holds the %s of the archive.\n",
		count(len(pending), "snapshot"), count(len(index), "snapshot"))
	return nil
}

// count returns a number with its noun, in the plural unless there is one.
func count(number int, noun string) string {
	if number == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", number, noun)
}

// pendingSnapshots returns the snapshots of the index that are not loaded,
// oldest first. It fails when the index and the database tell two stories:
// a loaded snapshot with another checksum, or a snapshot that is missing from
// the database while later ones are loaded.
func pendingSnapshots(index []archive.Snapshot, loaded []store.Snapshot) ([]archive.Snapshot, error) {
	checksums := make(map[int64]string, len(loaded))
	for _, snapshot := range loaded {
		checksums[snapshot.PublishedAt.UnixMicro()] = snapshot.SHA256
	}
	var pending []archive.Snapshot
	for _, snapshot := range index {
		published := snapshot.PublishedAt.Format(time.RFC3339Nano)
		checksum, isLoaded := checksums[snapshot.PublishedAt.UnixMicro()]
		switch {
		case isLoaded && checksum != snapshot.SHA256:
			return nil, fmt.Errorf("the snapshot published at %s is loaded with checksum %s, "+
				"and the archive's index now says %s", published, checksum, snapshot.SHA256)
		case isLoaded:
			continue
		case len(loaded) > 0 && snapshot.PublishedAt.Before(loaded[len(loaded)-1].PublishedAt):
			return nil, fmt.Errorf("the snapshot published at %s is in the archive's index but not "+
				"in the database, which holds later ones. Snapshots are loaded in order: "+
				"to include it, load the archive into an empty database", published)
		}
		pending = append(pending, snapshot)
	}
	return pending, nil
}

// printWarnings reports what the parser did not recognise in a snapshot, and
// the sites the file contains twice.
func printWarnings(stderr io.Writer, published string, warnings, duplicates []string) {
	all := make([]string, 0, len(warnings)+len(duplicates))
	all = append(all, warnings...)
	for _, duplicate := range duplicates {
		all = append(all, duplicate+": published more than once, the first is kept")
	}
	for number, warning := range all {
		if number == printedWarnings {
			fmt.Fprintf(stderr, "%s: and %d more warnings\n", published, len(all)-printedWarnings)
			break
		}
		fmt.Fprintf(stderr, "%s: warning: %s\n", published, warning)
	}
}
