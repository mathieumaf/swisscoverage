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
	allowDropAt := flags.String("allow-drop", "",
		"publication `time` of the one snapshot to load even though it has less than half\n"+
			"the sites of the one before, as the refusal printed it")
	if status, ok := parseArgs(flags, args, 0); !ok {
		return status
	}
	// The zero time, which no snapshot has, allows nothing.
	var allowDrop time.Time
	if *allowDropAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, *allowDropAt)
		if err != nil {
			fmt.Fprintf(stderr, "swisscoverage ingest: -allow-drop takes a publication time such as "+
				"2026-11-24T22:01:55.936174Z, not %q\n", *allowDropAt)
			return 2
		}
		allowDrop = parsed
	}

	source := &archive.Client{
		IndexURL:    *indexURL,
		ReleasesURL: *releasesURL,
		HTTP:        &http.Client{Timeout: 5 * time.Minute},
	}
	if err := ingest(ctx, source, allowDrop, stdout, stderr, getenv); err != nil {
		fmt.Fprintf(stderr, "swisscoverage ingest: %v\n", err)
		return 1
	}
	return 0
}

// ingest loads what is pending. allowDrop is the publication time of the one
// snapshot that may have less than half the sites of the one before.
func ingest(
	ctx context.Context, source *archive.Client, allowDrop time.Time,
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

	loadedNow := 0
	for _, snapshot := range pending {
		published := snapshot.PublishedAt.Format(time.RFC3339)
		if snapshot.PublishedAt.After(time.Now().Add(clockMargin)) {
			return fmt.Errorf("the snapshot published at %s is dated more than a day in the future",
				published)
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

		result, err := database.LoadSnapshot(ctx, store.Snapshot{
			PublishedAt: snapshot.PublishedAt,
			SHA256:      snapshot.SHA256,
			Sites:       sites,
		}, parsed.Stations, snapshot.PublishedAt.Equal(allowDrop))
		var tooFew *store.TooFewSitesError
		switch {
		case errors.Is(err, store.ErrAlreadyLoaded):
			// Another run loaded it since this one read the database.
			fmt.Fprintf(stdout, "%s: already loaded by another run\n", published)
			continue
		case errors.As(err, &tooFew):
			// The whole time, to the microsecond: it names one snapshot.
			return fmt.Errorf("the snapshot published at %s has %w: check the file, "+
				"then run again with -allow-drop %s to load it", published, err,
				snapshot.PublishedAt.Format(time.RFC3339Nano))
		case err != nil:
			return fmt.Errorf("the snapshot published at %s: %w", published, err)
		}
		loadedNow++

		fmt.Fprintf(stdout, "%s: %d sites, %d appeared, %d disappeared, %d changed, %d warnings\n",
			published, sites, result.Appeared, result.Disappeared, result.Changed,
			len(parsed.Warnings)+len(result.Duplicates))
		printWarnings(stderr, published, parsed.Warnings, result.Duplicates)
	}
	fmt.Fprintf(stdout, "Loaded %s. The database holds the %s of the archive.\n",
		count(loadedNow, "snapshot"), count(len(index), "snapshot"))
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
// oldest first. What is loaded must be the beginning of the index, snapshot
// for snapshot: it fails when the index lists a snapshot the database lacks
// before one it holds, when it no longer lists a loaded one, and when a
// loaded one has another checksum there.
func pendingSnapshots(index []archive.Snapshot, loaded []store.Snapshot) ([]archive.Snapshot, error) {
	for position, held := range loaded {
		heldAt := held.PublishedAt.Format(time.RFC3339Nano)
		if position == len(index) {
			return nil, fmt.Errorf("the database holds the snapshot published at %s, and the "+
				"archive's index, which lists %s, does not", heldAt, count(len(index), "snapshot"))
		}
		listed := index[position]
		listedAt := listed.PublishedAt.Format(time.RFC3339Nano)
		switch {
		case listed.PublishedAt.Before(held.PublishedAt):
			return nil, fmt.Errorf("the snapshot published at %s is in the archive's index but not "+
				"in the database, which holds later ones. Snapshots are loaded in order: "+
				"to include it, load the archive into an empty database", listedAt)
		case listed.PublishedAt.After(held.PublishedAt):
			return nil, fmt.Errorf("the database holds the snapshot published at %s, and the "+
				"archive's index does not list it", heldAt)
		case listed.SHA256 != held.SHA256:
			return nil, fmt.Errorf("the snapshot published at %s is loaded with checksum %s, "+
				"and the archive's index now says %s", heldAt, held.SHA256, listed.SHA256)
		}
	}
	return index[len(loaded):], nil
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
