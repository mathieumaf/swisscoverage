package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store/query"
)

// What the API reads. Each method runs its queries in one read-only
// transaction that sees a single moment, so that an import committing
// meanwhile never shows half of one snapshot and half of the next.

// Site is what identifies a site: its label and its position, as published.
type Site struct {
	Name string
	LV95 ofcom.LV95
}

// WGS84 is a position in WGS84 (EPSG:4326), in degrees. PostGIS computes it
// from the published LV95 position.
type WGS84 struct {
	Latitude, Longitude float64
}

// State is what a snapshot publishes about a site, apart from its identity.
type State struct {
	Kind         ofcom.StationKind
	PowerClass   ofcom.PowerClass
	Technologies []ofcom.Technology
	// Adaptive, Permit and InstallationLimitValue are nil when the file did
	// not say, as in ofcom.MobileStation.
	Adaptive               *bool
	Permit                 *ofcom.Permit
	InstallationLimitValue *float64
}

// Station is what never changes about a site.
type Station struct {
	Site
	Operator ofcom.Operator
	Code     string
	Position WGS84
	// FirstSeen is the publication time of the first snapshot that showed
	// the site.
	FirstSeen time.Time
}

// PublishedStation is a site as the latest snapshot shows it.
type PublishedStation struct {
	Station
	State
	// Distance is in metres, from the point of a search around a point. It
	// is zero otherwise.
	Distance float64
}

// Period is what was published of a site from one snapshot to another.
type Period struct {
	// From is the publication time of the snapshot that first showed these
	// values, and To that of the first snapshot that no longer did. To is
	// nil while they are still published.
	From time.Time
	To   *time.Time
	State
}

// Published is every site the latest snapshot shows.
type Published struct {
	// Snapshot is nil before the first import, and Stations empty then.
	Snapshot *Snapshot
	Stations []PublishedStation
}

// Page says which part of a list to return: at most Limit items, after
// skipping Offset of them.
type Page struct {
	Limit, Offset int
}

// Box is a rectangle in WGS84, in degrees.
type Box struct {
	West, South, East, North float64
}

// Around is a circle in WGS84: a point and a radius in metres.
type Around struct {
	Center WGS84
	Radius float64
}

// Search says which of the published sites to return. Exactly one of Box and
// Around is set. A filter left empty matches every site; a site matches
// Technologies when it has one of them.
type Search struct {
	Box          *Box
	Around       *Around
	Operators    []ofcom.Operator
	Kinds        []ofcom.StationKind
	PowerClasses []ofcom.PowerClass
	Technologies []ofcom.Technology
	Page
}

// Found is a page of the sites a search matched.
type Found struct {
	// Snapshot is nil before the first import.
	Snapshot *Snapshot
	// Stations is ordered by label and position, or nearest first around
	// a point.
	Stations []PublishedStation
	// Total counts every site the search matched, on every page.
	Total int
}

// History is a site and a page of its periods.
type History struct {
	Station
	// Published says whether the latest snapshot shows the site.
	Published bool
	// Periods is the latest first.
	Periods []Period
	// Total counts every period of the site, on every page.
	Total int
}

// Counts says how many sites the latest snapshot shows, in all and for each
// value of each vocabulary. Every value is present, with zero if no site has
// it. A site counts once for each of its technologies.
type Counts struct {
	// Snapshot is nil before the first import.
	Snapshot     *Snapshot
	Stations     int
	Operators    map[ofcom.Operator]int
	Kinds        map[ofcom.StationKind]int
	PowerClasses map[ofcom.PowerClass]int
	Technologies map[ofcom.Technology]int
}

// SnapshotPage is a page of the snapshots that were loaded.
type SnapshotPage struct {
	// Snapshots is the newest first.
	Snapshots []Snapshot
	// Total counts every snapshot that was loaded.
	Total int
}

// read runs fn in one read-only transaction.
func (s *Store) read(ctx context.Context, fn func(*query.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("starting to read: %w", err)
	}
	defer func() {
		// Ended even once ctx is done, but within a bound: a stalled
		// connection must not hold back the answer. pgx closes a connection
		// whose rollback fails.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		tx.Rollback(cleanup)
	}()
	if err := fn(query.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// How long ending a read may take once the request is over.
const rollbackTimeout = time.Second

func latestSnapshot(ctx context.Context, queries *query.Queries) (*Snapshot, error) {
	rows, err := queries.LatestSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the latest snapshot: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &Snapshot{PublishedAt: rows[0].PublishedAt.UTC(), SHA256: rows[0].Sha256, Sites: int(rows[0].Sites)}, nil
}

// Published returns every site the latest snapshot shows, ordered by label
// and position.
func (s *Store) Published(ctx context.Context) (Published, error) {
	var result Published
	err := s.read(ctx, func(queries *query.Queries) error {
		var err error
		if result.Snapshot, err = latestSnapshot(ctx, queries); err != nil {
			return err
		}
		rows, err := queries.ListCurrentStations(ctx)
		if err != nil {
			return fmt.Errorf("listing the published sites: %w", err)
		}
		result.Stations = make([]PublishedStation, 0, len(rows))
		for _, row := range rows {
			result.Stations = append(result.Stations, PublishedStation{
				Station: station(row.Name, row.Operator, row.Code, row.East, row.North,
					row.Latitude, row.Longitude, row.FirstSeen),
				State: state(row.Kind, row.PowerClass, row.Technologies, row.Adaptive,
					row.PermitType, row.PermitDate, row.InstallationLimitValue),
			})
		}
		return nil
	})
	return result, err
}

// Search returns a page of the published sites that match search.
func (s *Store) Search(ctx context.Context, search Search) (Found, error) {
	if (search.Box == nil) == (search.Around == nil) {
		return Found{}, fmt.Errorf("a search needs a rectangle or a circle, and not both")
	}
	var result Found
	err := s.read(ctx, func(queries *query.Queries) error {
		var err error
		if result.Snapshot, err = latestSnapshot(ctx, queries); err != nil {
			return err
		}
		result.Stations, result.Total, err = paged(search.Page,
			func(page Page) ([]PublishedStation, int, error) {
				return searchPage(ctx, queries, search, page)
			})
		return err
	})
	return result, err
}

// History returns the site of the given label and position with a page of
// its periods. It returns false when no snapshot ever showed that site.
func (s *Store) History(ctx context.Context, site Site, page Page) (History, bool, error) {
	var result History
	found := false
	err := s.read(ctx, func(queries *query.Queries) error {
		rows, err := queries.GetStation(ctx, query.GetStationParams{
			Name: site.Name, East: site.LV95.East, North: site.LV95.North,
		})
		if err != nil {
			return fmt.Errorf("reading the site: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		found = true
		row := rows[0]
		result.Station = station(row.Name, row.Operator, row.Code, row.East, row.North,
			row.Latitude, row.Longitude, row.FirstSeen)
		result.Published = row.Published

		result.Periods, result.Total, err = paged(page, func(page Page) ([]Period, int, error) {
			states, err := queries.ListStationStates(ctx, query.ListStationStatesParams{
				StationID: row.ID, RowLimit: int32(page.Limit), RowOffset: int32(page.Offset),
			})
			if err != nil {
				return nil, 0, fmt.Errorf("reading the history of the site: %w", err)
			}
			periods := make([]Period, 0, len(states))
			total := 0
			for _, st := range states {
				total = int(st.Total)
				period := Period{
					From: st.ValidFrom.UTC(),
					State: state(st.Kind, st.PowerClass, st.Technologies, st.Adaptive,
						st.PermitType, st.PermitDate, st.InstallationLimitValue),
				}
				if st.ValidTo.Valid {
					to := st.ValidTo.Time.UTC()
					period.To = &to
				}
				periods = append(periods, period)
			}
			return periods, total, nil
		})
		return err
	})
	return result, found, err
}

// Counts returns how many sites the latest snapshot shows, in all and for
// each value of each vocabulary.
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	result := Counts{
		Operators:    map[ofcom.Operator]int{},
		Kinds:        map[ofcom.StationKind]int{},
		PowerClasses: map[ofcom.PowerClass]int{},
		Technologies: map[ofcom.Technology]int{},
	}
	for _, value := range ofcom.Operators() {
		result.Operators[value] = 0
	}
	for _, value := range ofcom.StationKinds() {
		result.Kinds[value] = 0
	}
	for _, value := range ofcom.PowerClasses() {
		result.PowerClasses[value] = 0
	}
	for _, value := range ofcom.Technologies() {
		result.Technologies[value] = 0
	}
	err := s.read(ctx, func(queries *query.Queries) error {
		var err error
		if result.Snapshot, err = latestSnapshot(ctx, queries); err != nil {
			return err
		}
		total, err := queries.CountCurrentStations(ctx)
		if err != nil {
			return fmt.Errorf("counting the published sites: %w", err)
		}
		result.Stations = int(total)
		rows, err := queries.CountCurrentStationsBy(ctx)
		if err != nil {
			return fmt.Errorf("counting the published sites: %w", err)
		}
		for _, row := range rows {
			count := int(row.Stations)
			switch row.Dimension {
			case "operator":
				result.Operators[ofcom.Operator(row.Value)] = count
			case "kind":
				result.Kinds[ofcom.StationKind(row.Value)] = count
			case "power_class":
				result.PowerClasses[ofcom.PowerClass(row.Value)] = count
			case "technology":
				result.Technologies[ofcom.Technology(row.Value)] = count
			}
		}
		return nil
	})
	return result, err
}

// SnapshotPage returns a page of the snapshots that were loaded, the newest
// first.
func (s *Store) SnapshotPage(ctx context.Context, page Page) (SnapshotPage, error) {
	var result SnapshotPage
	err := s.read(ctx, func(queries *query.Queries) error {
		var err error
		result.Snapshots, result.Total, err = paged(page, func(page Page) ([]Snapshot, int, error) {
			rows, err := queries.ListSnapshotsPage(ctx, query.ListSnapshotsPageParams{
				RowLimit: int32(page.Limit), RowOffset: int32(page.Offset),
			})
			if err != nil {
				return nil, 0, fmt.Errorf("listing the snapshots: %w", err)
			}
			snapshots := make([]Snapshot, 0, len(rows))
			total := 0
			for _, row := range rows {
				total = int(row.Total)
				snapshots = append(snapshots, Snapshot{
					PublishedAt: row.PublishedAt.UTC(), SHA256: row.Sha256, Sites: int(row.Sites),
				})
			}
			return snapshots, total, nil
		})
		return err
	})
	return result, err
}

// paged returns the items of a page and how many there are on every page,
// as list reads them. Each row carries the total, so a page past the end has
// none to carry it: the first row is then read for it.
func paged[T any](page Page, list func(Page) ([]T, int, error)) ([]T, int, error) {
	items, total, err := list(page)
	if err != nil || len(items) > 0 || page.Offset == 0 {
		return items, total, err
	}
	_, total, err = list(Page{Limit: 1})
	return items, total, err
}

// searchPage reads one page of a search.
func searchPage(ctx context.Context, queries *query.Queries, search Search, page Page) (
	[]PublishedStation, int, error,
) {
	operators := texts(search.Operators)
	kinds := texts(search.Kinds)
	powerClasses := texts(search.PowerClasses)
	technologies := texts(search.Technologies)
	stations := []PublishedStation{}
	total := 0
	if search.Box != nil {
		rows, err := queries.SearchStationsInBox(ctx, query.SearchStationsInBoxParams{
			West: search.Box.West, South: search.Box.South,
			East: search.Box.East, North: search.Box.North,
			Operators: operators, Kinds: kinds, PowerClasses: powerClasses, Technologies: technologies,
			RowLimit: int32(page.Limit), RowOffset: int32(page.Offset),
		})
		if err != nil {
			return nil, 0, fmt.Errorf("searching the sites: %w", err)
		}
		for _, row := range rows {
			total = int(row.Total)
			stations = append(stations, PublishedStation{
				Station: station(row.Name, row.Operator, row.Code, row.East, row.North,
					row.Latitude, row.Longitude, row.FirstSeen),
				State: state(row.Kind, row.PowerClass, row.Technologies, row.Adaptive,
					row.PermitType, row.PermitDate, row.InstallationLimitValue),
			})
		}
		return stations, total, nil
	}
	rows, err := queries.SearchStationsAround(ctx, query.SearchStationsAroundParams{
		Latitude: search.Around.Center.Latitude, Longitude: search.Around.Center.Longitude,
		Radius:    search.Around.Radius,
		Operators: operators, Kinds: kinds, PowerClasses: powerClasses, Technologies: technologies,
		RowLimit: int32(page.Limit), RowOffset: int32(page.Offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("searching the sites: %w", err)
	}
	for _, row := range rows {
		total = int(row.Total)
		stations = append(stations, PublishedStation{
			Station: station(row.Name, row.Operator, row.Code, row.East, row.North,
				row.Latitude, row.Longitude, row.FirstSeen),
			State: state(row.Kind, row.PowerClass, row.Technologies, row.Adaptive,
				row.PermitType, row.PermitDate, row.InstallationLimitValue),
			Distance: row.Distance,
		})
	}
	return stations, total, nil
}

func station(
	name, operator, code string, east, north, latitude, longitude float64, firstSeen time.Time,
) Station {
	return Station{
		Site:      Site{Name: name, LV95: ofcom.LV95{East: east, North: north}},
		Operator:  ofcom.Operator(operator),
		Code:      code,
		Position:  WGS84{Latitude: latitude, Longitude: longitude},
		FirstSeen: firstSeen.UTC(),
	}
}

func state(
	kind, powerClass string, technologies []string, adaptive pgtype.Bool,
	permitType pgtype.Text, permitDate pgtype.Date, limitValue pgtype.Float8,
) State {
	result := State{
		Kind:         ofcom.StationKind(kind),
		PowerClass:   ofcom.PowerClass(powerClass),
		Technologies: make([]ofcom.Technology, 0, len(technologies)),
	}
	for _, technology := range technologies {
		result.Technologies = append(result.Technologies, ofcom.Technology(technology))
	}
	if adaptive.Valid {
		result.Adaptive = &adaptive.Bool
	}
	if permitType.Valid {
		result.Permit = &ofcom.Permit{Type: ofcom.PermitType(permitType.String)}
		if permitDate.Valid {
			result.Permit.Date = permitDate.Time.Format(time.DateOnly)
		}
	}
	if limitValue.Valid {
		result.InstallationLimitValue = &limitValue.Float64
	}
	return result
}

// texts turns values of a vocabulary into the strings the database holds.
func texts[T ~string](values []T) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}
