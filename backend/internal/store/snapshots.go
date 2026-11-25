package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store/query"
)

// Snapshot is one file published by OFCOM, as the database records it.
type Snapshot struct {
	// PublishedAt is when OFCOM published the file. It identifies the
	// snapshot. The database keeps it to the microsecond.
	PublishedAt time.Time
	// SHA256 is the checksum of the file as published, in hexadecimal.
	SHA256 string
	// Sites is the number of sites in the file.
	Sites int
}

// Loaded says what a snapshot changed compared with the one before it.
type Loaded struct {
	// Appeared counts the sites that were not in the previous snapshot,
	// Disappeared those that are no longer in this one, and Changed those
	// in both whose published values differ.
	Appeared, Disappeared, Changed int
	// Duplicates lists the sites the file contains more than once, each as
	// its label and position. The first occurrence is the one kept.
	Duplicates []string
}

// TooFewSitesError is returned by LoadSnapshot for a snapshot with less than
// half the sites that were published before it. A file cut short, or emptied,
// would end every site it lacks, and that is recorded for good: a person
// decides.
type TooFewSitesError struct {
	// Sites is the number of distinct sites in the snapshot, and Current the
	// number of sites published before it.
	Sites, Current int
}

func (e *TooFewSitesError) Error() string {
	return fmt.Sprintf("%d sites, less than half the %d of the one before", e.Sites, e.Current)
}

// ErrAlreadyLoaded is returned by LoadSnapshot for a snapshot the database
// already holds, with the same checksum.
var ErrAlreadyLoaded = errors.New("the snapshot is already loaded")

// Snapshots returns the snapshots that were loaded, oldest first.
func (s *Store) Snapshots(ctx context.Context) ([]Snapshot, error) {
	return listSnapshots(ctx, query.New(s.pool))
}

func listSnapshots(ctx context.Context, queries *query.Queries) ([]Snapshot, error) {
	rows, err := queries.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing the snapshots: %w", err)
	}
	snapshots := make([]Snapshot, 0, len(rows))
	for _, row := range rows {
		snapshots = append(snapshots, Snapshot{
			PublishedAt: row.PublishedAt.UTC(),
			SHA256:      row.Sha256,
			Sites:       int(row.Sites),
		})
	}
	return snapshots, nil
}

// LoadSnapshot records a snapshot and what it changes: the sites it shows for
// the first time, and for every site the period during which its published
// values stayed the same. Everything happens in one transaction, so readers
// see the previous snapshot or this one, never a mix.
//
// Snapshots are loaded in the order they were published. One that is older
// than the latest loaded is refused, because the periods already recorded
// would have to be rewritten.
//
// A snapshot with less than half the sites published before it is refused
// with a *TooFewSitesError, unless allowDrop is true.
func (s *Store) LoadSnapshot(
	ctx context.Context, snapshot Snapshot, stations []ofcom.MobileStation, allowDrop bool,
) (Loaded, error) {
	// PostgreSQL keeps microseconds: the same precision here, so that a time
	// read back equals the one that was written.
	publishedAt := snapshot.PublishedAt.Truncate(time.Microsecond)
	rows, duplicates, err := incomingRows(stations)
	if err != nil {
		return Loaded{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Loaded{}, fmt.Errorf("starting the import: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	queries := query.New(tx)

	if err := queries.LockImport(ctx); err != nil {
		return Loaded{}, fmt.Errorf("waiting for another import: %w", err)
	}
	known, err := listSnapshots(ctx, queries)
	if err != nil {
		return Loaded{}, err
	}
	var previous *Snapshot
	if len(known) > 0 {
		previous = &known[len(known)-1]
	}
	for _, loaded := range known {
		if !loaded.PublishedAt.Equal(publishedAt) {
			continue
		}
		if loaded.SHA256 != snapshot.SHA256 {
			return Loaded{}, fmt.Errorf(
				"the snapshot published at %s is already loaded with another checksum: %s, not %s",
				formatTime(publishedAt), loaded.SHA256, snapshot.SHA256)
		}
		return Loaded{}, ErrAlreadyLoaded
	}
	if previous != nil && previous.PublishedAt.After(publishedAt) {
		return Loaded{}, fmt.Errorf(
			"the snapshot published at %s is older than the latest one loaded, published at %s",
			formatTime(publishedAt), formatTime(previous.PublishedAt))
	}

	// Sites are counted as the database will see them: a file that repeats
	// one site a thousand times holds one.
	current, err := queries.CountCurrentStations(ctx)
	if err != nil {
		return Loaded{}, fmt.Errorf("counting the published sites: %w", err)
	}
	if int64(len(rows))*2 < current && !allowDrop {
		return Loaded{}, &TooFewSitesError{Sites: len(rows), Current: int(current)}
	}

	err = queries.InsertSnapshot(ctx, query.InsertSnapshotParams{
		PublishedAt: publishedAt,
		Sha256:      snapshot.SHA256,
		Sites:       int32(snapshot.Sites),
	})
	if err != nil {
		return Loaded{}, fmt.Errorf("recording the snapshot: %w", err)
	}
	if err := queries.ClearIncomingStations(ctx); err != nil {
		return Loaded{}, fmt.Errorf("preparing the comparison: %w", err)
	}
	if _, err := queries.CopyIncomingStations(ctx, rows); err != nil {
		return Loaded{}, fmt.Errorf("copying the stations: %w", err)
	}
	if _, err := queries.InsertNewStations(ctx, publishedAt); err != nil {
		return Loaded{}, fmt.Errorf("adding the new sites: %w", err)
	}
	_, err = queries.CloseOutdatedStates(ctx, pgtype.Timestamptz{Time: publishedAt, Valid: true})
	if err != nil {
		return Loaded{}, fmt.Errorf("ending what is no longer published: %w", err)
	}
	opened, err := queries.OpenNewStates(ctx, publishedAt)
	if err != nil {
		return Loaded{}, fmt.Errorf("recording what is published: %w", err)
	}
	if err := queries.ClearIncomingStations(ctx); err != nil {
		return Loaded{}, fmt.Errorf("clearing the comparison: %w", err)
	}

	// In a first snapshot, every site appears.
	loaded := Loaded{Appeared: int(opened), Duplicates: duplicates}
	if previous != nil {
		changes, err := listChanges(ctx, queries, previous.PublishedAt, publishedAt)
		if err != nil {
			return Loaded{}, err
		}
		loaded.Appeared = 0
		for _, change := range changes {
			switch change.Kind {
			case Appeared:
				loaded.Appeared++
			case Disappeared:
				loaded.Disappeared++
			case Changed:
				loaded.Changed++
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Loaded{}, fmt.Errorf("saving the snapshot: %w", err)
	}
	return loaded, nil
}

// incomingRows turns the parser's stations into rows to copy, one per site.
// A site is its label and its position: when the file contains one twice,
// the first is kept and the site is reported.
func incomingRows(stations []ofcom.MobileStation) ([]query.CopyIncomingStationsParams, []string, error) {
	type site struct {
		name        string
		east, north float64
	}
	seen := make(map[site]bool, len(stations))
	rows := make([]query.CopyIncomingStationsParams, 0, len(stations))
	var duplicates []string
	for _, station := range stations {
		key := site{station.Name, station.LV95.East, station.LV95.North}
		if seen[key] {
			duplicates = append(duplicates, fmt.Sprintf("%s at %s, %s", station.Name,
				strconv.FormatFloat(station.LV95.East, 'f', -1, 64),
				strconv.FormatFloat(station.LV95.North, 'f', -1, 64)))
			continue
		}
		seen[key] = true

		row := query.CopyIncomingStationsParams{
			Name:         station.Name,
			Operator:     string(station.Operator),
			Code:         station.Code,
			East:         station.LV95.East,
			North:        station.LV95.North,
			Kind:         string(station.Kind),
			PowerClass:   string(station.PowerClass),
			Technologies: make([]string, 0, len(station.Technologies)),
		}
		for _, technology := range station.Technologies {
			row.Technologies = append(row.Technologies, string(technology))
		}
		if station.Adaptive != nil {
			row.Adaptive = pgtype.Bool{Bool: *station.Adaptive, Valid: true}
		}
		if station.Permit != nil {
			row.PermitType = pgtype.Text{String: string(station.Permit.Type), Valid: true}
			if station.Permit.Date != "" {
				date, err := time.Parse(time.DateOnly, station.Permit.Date)
				if err != nil {
					return nil, nil, fmt.Errorf("%s: the permit date %q is not a date",
						station.Name, station.Permit.Date)
				}
				row.PermitDate = pgtype.Date{Time: date, Valid: true}
			}
		}
		if station.InstallationLimitValue != nil {
			row.InstallationLimitValue = pgtype.Float8{Float64: *station.InstallationLimitValue, Valid: true}
		}
		rows = append(rows, row)
	}
	return rows, duplicates, nil
}

// ChangeKind says how a site differs between two moments.
type ChangeKind string

const (
	// Appeared: published at the later moment and not at the earlier one.
	Appeared ChangeKind = "appeared"
	// Disappeared: published at the earlier moment and not at the later one.
	Disappeared ChangeKind = "disappeared"
	// Changed: published at both, with different values.
	Changed ChangeKind = "changed"
)

// Change is one site that is not the same at two moments.
type Change struct {
	Name     string
	Operator ofcom.Operator
	Code     string
	LV95     ofcom.LV95
	Kind     ChangeKind
}

// Changes returns the sites that differ between what was published at from
// and what was published at to, ordered by label and position. What was
// published at a moment is the latest snapshot loaded up to then. A site that
// changed and changed back in between is not listed.
func (s *Store) Changes(ctx context.Context, from, to time.Time) ([]Change, error) {
	return listChanges(ctx, query.New(s.pool), from, to)
}

func listChanges(ctx context.Context, queries *query.Queries, from, to time.Time) ([]Change, error) {
	rows, err := queries.ListChanges(ctx, query.ListChangesParams{FromTime: from, ToTime: to})
	if err != nil {
		return nil, fmt.Errorf("comparing the two moments: %w", err)
	}
	changes := make([]Change, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, Change{
			Name:     row.Name,
			Operator: ofcom.Operator(row.Operator),
			Code:     row.Code,
			LV95:     ofcom.LV95{East: row.East, North: row.North},
			Kind:     ChangeKind(row.Change),
		})
	}
	return changes, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
