package store_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
	"github.com/mathieumaf/swisscoverage/backend/internal/testdb"
)

var (
	day1 = time.Date(2026, 11, 23, 22, 1, 35, 611529000, time.UTC)
	day2 = time.Date(2026, 11, 24, 22, 1, 55, 936174000, time.UTC)
	day3 = time.Date(2026, 11, 25, 22, 2, 7, 0, time.UTC)
)

// open returns a store on an empty database with the schema applied, and the
// address of that database for assertions.
func open(t *testing.T) (*store.Store, string) {
	t.Helper()
	address := testdb.New(t)
	database, err := store.Open(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return database, address
}

func pointer[T any](value T) *T { return &value }

// station returns a site with ordinary values, which a test then changes.
func station(name string, east, north float64) ofcom.MobileStation {
	operator, code, _ := strings.Cut(name, " ")
	return ofcom.MobileStation{
		Name:                   name,
		Operator:               ofcom.Operator(strings.ToLower(operator)),
		Code:                   code,
		Kind:                   ofcom.KindOutdoor,
		PowerClass:             ofcom.PowerMedium,
		Technologies:           []ofcom.Technology{ofcom.Technology4G, ofcom.Technology5G},
		Adaptive:               pointer(true),
		Permit:                 &ofcom.Permit{Type: ofcom.PermitSiteDataSheet, Date: "2021-06-03"},
		InstallationLimitValue: pointer(5.0),
		LV95:                   ofcom.LV95{East: east, North: north},
	}
}

// with returns a copy of a station after change has edited it.
func with(original ofcom.MobileStation, change func(*ofcom.MobileStation)) ofcom.MobileStation {
	change(&original)
	return original
}

// load loads a snapshot made of stations and fails the test on an error.
func load(
	t *testing.T, database *store.Store, publishedAt time.Time, stations ...ofcom.MobileStation,
) store.Loaded {
	t.Helper()
	loaded, err := database.LoadSnapshot(context.Background(), snapshotAt(publishedAt, len(stations)), stations, false)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// snapshotAt returns a snapshot whose checksum depends on its time only.
func snapshotAt(publishedAt time.Time, sites int) store.Snapshot {
	checksum := strings.Repeat("0", 60) + publishedAt.Format("0102")
	return store.Snapshot{PublishedAt: publishedAt, SHA256: checksum, Sites: sites}
}

func wantEqual[T any](t *testing.T, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// changes returns what differs between two moments as "kind name" lines.
func changes(t *testing.T, database *store.Store, from, to time.Time) []string {
	t.Helper()
	list, err := database.Changes(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{}
	for _, change := range list {
		lines = append(lines, string(change.Kind)+" "+change.Name)
	}
	return lines
}

var (
	bifo = station("Swisscom BIFO", 2676878, 1246069)
	dibh = station("Swisscom DIBH", 2692356, 1233045)
	salt = station("Salt TI_3787A", 2717696, 1102456)
	sbb  = station("SBB AU-WAE-02202", 2600000, 1200000)
)

// The first migration runs on a database nobody prepared: it creates PostGIS
// itself, where the role is allowed to.
func TestMigrate(t *testing.T) {
	address := testdb.NewBare(t)
	database, err := store.Open(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()

	if err := database.CheckSchema(ctx); err == nil ||
		!strings.Contains(err.Error(), "run `swisscoverage migrate`") {
		t.Errorf("before migrating: got %v, want an error that names the command to run", err)
	}

	first, err := database.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Applied) == 0 || first.Applied[0] != "0001_stations.sql" {
		t.Errorf("got %v applied, want the files in order from 0001_stations.sql", first.Applied)
	}
	if first.Database != first.Program || first.Database != len(first.Applied) {
		t.Errorf("got %+v, want the database at the program's version", first)
	}
	if err := database.CheckSchema(ctx); err != nil {
		t.Errorf("after migrating: %v", err)
	}

	second, err := database.Migrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Applied) != 0 || second.Database != first.Database {
		t.Errorf("a second run: got %+v, want nothing applied", second)
	}

	recorded := testdb.Query(t, address, `SELECT version, name FROM schema_migration ORDER BY version`)
	if len(recorded) != first.Program || recorded[0][1] != "0001_stations.sql" {
		t.Errorf("got %v recorded, want one row per file", recorded)
	}
}

// A program rolled back to an earlier version finds a schema it does not
// know. That is expected: the schema stays compatible with it.
func TestMigrateLeavesANewerSchemaAlone(t *testing.T) {
	database, address := open(t)
	testdb.Query(t, address,
		`INSERT INTO schema_migration (version, name) VALUES (9999, '9999_from_the_future.sql')`)

	result, err := database.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Applied) != 0 || result.Database != 9999 || result.Database <= result.Program {
		t.Errorf("got %+v, want nothing applied and the database ahead", result)
	}
	if err := database.CheckSchema(context.Background()); err != nil {
		t.Errorf("a newer schema is not an error: %v", err)
	}
}

func TestMigrateFromSeveralProgramsAtOnce(t *testing.T) {
	address := testdb.New(t)
	results := make(chan error, 4)
	for range cap(results) {
		go func() {
			database, err := store.Open(context.Background(), address)
			if err != nil {
				results <- err
				return
			}
			defer database.Close()
			_, err = database.Migrate(context.Background())
			results <- err
		}()
	}
	for range cap(results) {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if recorded := testdb.Query(t, address, `SELECT count(*) FROM schema_migration`); recorded[0][0] == "0" {
		t.Error("no migration recorded")
	}
}

func TestLoadFirstSnapshot(t *testing.T) {
	database, address := open(t)

	loaded := load(t, database, day1, bifo, dibh)

	wantEqual(t, loaded, store.Loaded{Appeared: 2})
	snapshots, err := database.Snapshots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantEqual(t, snapshots, []store.Snapshot{snapshotAt(day1, 2)})

	wantEqual(t, testdb.Query(t, address, `
		SELECT s.name, s.operator, s.code, s.east, s.north, s.first_seen = $1,
		       st.valid_from = $1, st.valid_to, st.kind, st.power_class, st.technologies,
		       st.adaptive, st.permit_type, st.permit_date, st.installation_limit_value
		FROM station s JOIN station_state st ON st.station_id = s.id
		ORDER BY s.name`, day1),
		[][]string{
			{"Swisscom BIFO", "swisscom", "BIFO", "2676878", "1246069", "t",
				"t", "NULL", "outdoor", "medium", "{4G,5G}", "t", "site_data_sheet", "2021-06-03", "5"},
			{"Swisscom DIBH", "swisscom", "DIBH", "2692356", "1233045", "t",
				"t", "NULL", "outdoor", "medium", "{4G,5G}", "t", "site_data_sheet", "2021-06-03", "5"},
		})
	wantEqual(t, testdb.Query(t, address, `SELECT count(*) FROM incoming_station`), [][]string{{"0"}})
}

func TestLoadKeepsWhatTheFileLeavesBlank(t *testing.T) {
	database, address := open(t)
	blank := with(bifo, func(s *ofcom.MobileStation) {
		s.Technologies = []ofcom.Technology{}
		s.Adaptive = nil
		s.Permit = nil
		s.InstallationLimitValue = nil
	})
	exempt := with(dibh, func(s *ofcom.MobileStation) {
		s.Adaptive = pointer(false)
		s.Permit = &ofcom.Permit{Type: ofcom.PermitExempt}
	})
	// A data sheet whose published date the parser could not keep.
	undated := with(salt, func(s *ofcom.MobileStation) {
		s.Permit = &ofcom.Permit{Type: ofcom.PermitSiteDataSheet}
	})

	load(t, database, day1, blank, exempt, undated)

	wantEqual(t, testdb.Query(t, address, `
		SELECT s.name, st.technologies, st.adaptive, st.permit_type, st.permit_date,
		       st.installation_limit_value
		FROM station s JOIN station_state st ON st.station_id = s.id
		ORDER BY s.name`),
		[][]string{
			{"Salt TI_3787A", "{4G,5G}", "t", "site_data_sheet", "NULL", "5"},
			{"Swisscom BIFO", "{}", "NULL", "NULL", "NULL", "NULL"},
			{"Swisscom DIBH", "{4G,5G}", "f", "exempt", "NULL", "5"},
		})
}

// Every value the parser can return must be one the schema accepts: a value
// added to the parser without a migration would stop the import.
func TestLoadAcceptsEveryValueOfTheParser(t *testing.T) {
	database, _ := open(t)
	var stations []ofcom.MobileStation
	add := func(change func(*ofcom.MobileStation)) {
		site := with(bifo, change)
		site.LV95.East += float64(len(stations))
		stations = append(stations, site)
	}
	for _, operator := range ofcom.Operators() {
		add(func(s *ofcom.MobileStation) { s.Operator = operator })
	}
	for _, kind := range ofcom.StationKinds() {
		add(func(s *ofcom.MobileStation) { s.Kind = kind })
	}
	for _, powerClass := range ofcom.PowerClasses() {
		add(func(s *ofcom.MobileStation) { s.PowerClass = powerClass })
	}
	add(func(s *ofcom.MobileStation) { s.Technologies = ofcom.Technologies() })
	for _, permit := range []ofcom.PermitType{ofcom.PermitSiteDataSheet, ofcom.PermitExempt} {
		add(func(s *ofcom.MobileStation) { s.Permit = &ofcom.Permit{Type: permit} })
	}

	loaded := load(t, database, day1, stations...)

	wantEqual(t, loaded.Appeared, len(stations))
}

// PostGIS computes the WGS84 position. LV95 2600000 / 1200000 is the origin
// of the Swiss grid, the old observatory of Bern: 46.95108 N, 7.43864 E.
func TestLoadConvertsThePositionToWGS84(t *testing.T) {
	database, address := open(t)

	load(t, database, day1, sbb)

	position := testdb.Query(t, address, `
		SELECT abs(ST_Y(position) - 46.95108) < 0.00001, abs(ST_X(position) - 7.43864) < 0.00001,
		       ST_SRID(position)
		FROM station`)
	wantEqual(t, position, [][]string{{"t", "t", "4326"}})
}

func TestLoadRecordsWhatChanged(t *testing.T) {
	database, address := open(t)
	load(t, database, day1, bifo, dibh, salt)

	// BIFO is unchanged, DIBH gains a technology, Salt leaves, SBB arrives.
	upgraded := with(dibh, func(s *ofcom.MobileStation) {
		s.Technologies = []ofcom.Technology{ofcom.Technology3G, ofcom.Technology4G, ofcom.Technology5G}
	})
	loaded := load(t, database, day2, bifo, upgraded, sbb)

	wantEqual(t, loaded, store.Loaded{Appeared: 1, Disappeared: 1, Changed: 1})
	wantEqual(t, changes(t, database, day1, day2), []string{
		"disappeared Salt TI_3787A",
		"appeared SBB AU-WAE-02202",
		"changed Swisscom DIBH",
	})

	wantEqual(t, testdb.Query(t, address, `
		SELECT s.name, st.valid_from = $1, st.valid_to = $2, st.valid_to IS NULL, st.technologies
		FROM station s JOIN station_state st ON st.station_id = s.id
		ORDER BY s.name, st.valid_from`, day1, day2),
		[][]string{
			// Left the file: its only state has an end.
			{"Salt TI_3787A", "t", "t", "f", "{4G,5G}"},
			// Arrived: one state, from the second snapshot.
			{"SBB AU-WAE-02202", "f", "NULL", "t", "{4G,5G}"},
			// Unchanged: still its first state.
			{"Swisscom BIFO", "t", "NULL", "t", "{4G,5G}"},
			// Changed: the old state ends where the new one starts.
			{"Swisscom DIBH", "t", "t", "f", "{4G,5G}"},
			{"Swisscom DIBH", "f", "NULL", "t", "{3G,4G,5G}"},
		})
	wantEqual(t, testdb.Query(t, address,
		`SELECT name, first_seen = $1 FROM station ORDER BY name`, day1),
		[][]string{
			{"Salt TI_3787A", "t"}, {"SBB AU-WAE-02202", "f"}, {"Swisscom BIFO", "t"}, {"Swisscom DIBH", "t"},
		})
}

// Each published value is compared, including one that goes from a value to
// nothing.
func TestLoadSeesAChangeInEachValue(t *testing.T) {
	edits := map[string]func(*ofcom.MobileStation){
		"kind":         func(s *ofcom.MobileStation) { s.Kind = ofcom.KindIndoor },
		"power class":  func(s *ofcom.MobileStation) { s.PowerClass = ofcom.PowerHigh },
		"technologies": func(s *ofcom.MobileStation) { s.Technologies = []ofcom.Technology{ofcom.Technology5G} },
		"adaptive":     func(s *ofcom.MobileStation) { s.Adaptive = nil },
		"permit type":  func(s *ofcom.MobileStation) { s.Permit = &ofcom.Permit{Type: ofcom.PermitExempt} },
		"permit date": func(s *ofcom.MobileStation) {
			s.Permit = &ofcom.Permit{Type: ofcom.PermitSiteDataSheet, Date: "2024-08-29"}
		},
		"no permit":   func(s *ofcom.MobileStation) { s.Permit = nil },
		"limit value": func(s *ofcom.MobileStation) { s.InstallationLimitValue = pointer(6.0) },
		"no limit":    func(s *ofcom.MobileStation) { s.InstallationLimitValue = nil },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			database, _ := open(t)
			load(t, database, day1, bifo, dibh)

			loaded := load(t, database, day2, with(bifo, edit), dibh)

			wantEqual(t, loaded, store.Loaded{Changed: 1})
		})
	}
}

// The same label at another position is another site.
func TestLoadTreatsAMovedSiteAsAnotherSite(t *testing.T) {
	database, _ := open(t)
	load(t, database, day1, bifo)

	moved := with(bifo, func(s *ofcom.MobileStation) { s.LV95.East += 3 })
	loaded := load(t, database, day2, moved)

	wantEqual(t, loaded, store.Loaded{Appeared: 1, Disappeared: 1})
}

func TestLoadAnIdenticalSnapshotAddsNothing(t *testing.T) {
	database, address := open(t)
	load(t, database, day1, bifo, dibh)

	loaded := load(t, database, day2, dibh, bifo)

	wantEqual(t, loaded, store.Loaded{})
	wantEqual(t, changes(t, database, day1, day2), []string{})
	wantEqual(t, testdb.Query(t, address, `SELECT count(*) FROM station_state`), [][]string{{"2"}})
}

func TestLoadASiteThatComesBack(t *testing.T) {
	database, address := open(t)
	load(t, database, day1, bifo, dibh)
	load(t, database, day2, bifo)

	loaded := load(t, database, day3, bifo, dibh)

	wantEqual(t, loaded, store.Loaded{Appeared: 1})
	// Published at the first and the third moment with the same values.
	wantEqual(t, changes(t, database, day1, day3), []string{})
	wantEqual(t, testdb.Query(t, address, `
		SELECT count(DISTINCT s.id), count(*), count(*) FILTER (WHERE st.valid_to IS NULL)
		FROM station s JOIN station_state st ON st.station_id = s.id
		WHERE s.name = 'Swisscom DIBH'`),
		[][]string{{"1", "2", "1"}})
}

func TestChangesBetweenAnyTwoMoments(t *testing.T) {
	database, _ := open(t)
	high := with(bifo, func(s *ofcom.MobileStation) { s.PowerClass = ofcom.PowerHigh })
	load(t, database, day1, bifo, dibh)
	load(t, database, day2, high, dibh)
	load(t, database, day3, bifo, dibh, salt)

	// Changed and changed back: the same at both ends.
	wantEqual(t, changes(t, database, day1, day3), []string{"appeared Salt TI_3787A"})
	wantEqual(t, changes(t, database, day2, day3), []string{
		"appeared Salt TI_3787A", "changed Swisscom BIFO",
	})
	// A moment between two snapshots shows the earlier one.
	wantEqual(t, changes(t, database, day1.Add(time.Hour), day2.Add(time.Hour)),
		[]string{"changed Swisscom BIFO"})
	// Before the first snapshot, nothing was published.
	wantEqual(t, changes(t, database, day1.Add(-time.Hour), day1), []string{
		"appeared Swisscom BIFO", "appeared Swisscom DIBH",
	})
	wantEqual(t, changes(t, database, day3, day1), []string{"disappeared Salt TI_3787A"})
}

func TestLoadKeepsTheFirstOfASitePublishedTwice(t *testing.T) {
	database, address := open(t)
	again := with(bifo, func(s *ofcom.MobileStation) { s.PowerClass = ofcom.PowerHigh })

	loaded := load(t, database, day1, bifo, again, dibh)

	wantEqual(t, loaded, store.Loaded{
		Appeared:   2,
		Duplicates: []string{"Swisscom BIFO at 2676878, 1246069"},
	})
	wantEqual(t, testdb.Query(t, address, `
		SELECT st.power_class FROM station s JOIN station_state st ON st.station_id = s.id
		WHERE s.name = 'Swisscom BIFO'`),
		[][]string{{"medium"}})
	// The snapshot records what the file contains.
	wantEqual(t, testdb.Query(t, address, `SELECT sites FROM snapshot`), [][]string{{"3"}})
}

func TestLoadRefusesWhatWouldRewriteHistory(t *testing.T) {
	database, address := open(t)
	ctx := context.Background()
	load(t, database, day2, bifo)

	_, err := database.LoadSnapshot(ctx, snapshotAt(day2, 1), []ofcom.MobileStation{bifo}, false)
	if !errors.Is(err, store.ErrAlreadyLoaded) {
		t.Errorf("the same snapshot again: got %v, want ErrAlreadyLoaded", err)
	}

	other := snapshotAt(day2, 1)
	other.SHA256 = strings.Repeat("f", 64)
	_, err = database.LoadSnapshot(ctx, other, []ofcom.MobileStation{bifo, dibh}, false)
	if err == nil || !strings.Contains(err.Error(), "already loaded with another checksum") {
		t.Errorf("the same time with another checksum: got %v", err)
	}

	_, err = database.LoadSnapshot(ctx, snapshotAt(day1, 1), []ofcom.MobileStation{dibh}, false)
	if err == nil || !strings.Contains(err.Error(), "older than the latest one loaded") {
		t.Errorf("an older snapshot: got %v", err)
	}

	wantEqual(t, testdb.Query(t, address,
		`SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station)`),
		[][]string{{"1", "1"}})
}

// Sites are counted as the database sees them, so a file cannot hide that it
// lost most of them by repeating the ones it kept.
func TestLoadRefusesASnapshotThatLostMostSites(t *testing.T) {
	database, address := open(t)
	ctx := context.Background()
	load(t, database, day1, bifo, dibh, salt, sbb)
	padded := []ofcom.MobileStation{bifo, bifo, bifo, bifo}

	_, err := database.LoadSnapshot(ctx, snapshotAt(day2, 4), padded, false)

	var tooFew *store.TooFewSitesError
	if !errors.As(err, &tooFew) || tooFew.Sites != 1 || tooFew.Current != 4 {
		t.Fatalf("got %v, want a TooFewSitesError for 1 site out of 4", err)
	}
	wantEqual(t, testdb.Query(t, address, `SELECT count(*) FROM snapshot`), [][]string{{"1"}})

	// Exactly half is not a drop.
	wantEqual(t, load(t, database, day2, bifo, dibh), store.Loaded{Disappeared: 2})

	// And a person can allow one.
	loaded, err := database.LoadSnapshot(ctx, snapshotAt(day3, 0), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	wantEqual(t, loaded, store.Loaded{Disappeared: 2})
}

// Two imports of the same snapshot at once: one loads it, the other waits and
// then finds it loaded.
func TestLoadFromSeveralProgramsAtOnce(t *testing.T) {
	_, address := open(t)
	results := make(chan error, 4)
	for range cap(results) {
		go func() {
			database, err := store.Open(context.Background(), address)
			if err != nil {
				results <- err
				return
			}
			defer database.Close()
			_, err = database.LoadSnapshot(context.Background(), snapshotAt(day1, 2),
				[]ofcom.MobileStation{bifo, dibh}, false)
			results <- err
		}()
	}
	loaded := 0
	for range cap(results) {
		switch err := <-results; {
		case err == nil:
			loaded++
		case !errors.Is(err, store.ErrAlreadyLoaded):
			t.Errorf("got %v, want the snapshot loaded or ErrAlreadyLoaded", err)
		}
	}
	wantEqual(t, loaded, 1)
	wantEqual(t, testdb.Query(t, address,
		`SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station_state)`),
		[][]string{{"1", "2"}})
}

// A failure leaves no trace: the snapshot, its sites and their states are
// written together or not at all.
func TestLoadIsAllOrNothing(t *testing.T) {
	database, address := open(t)
	load(t, database, day1, bifo)

	broken := with(dibh, func(s *ofcom.MobileStation) { s.Kind = "balloon" })
	_, err := database.LoadSnapshot(context.Background(), snapshotAt(day2, 2),
		[]ofcom.MobileStation{broken, salt}, false)
	if err == nil {
		t.Fatal("a value the schema does not know was accepted")
	}

	wantEqual(t, testdb.Query(t, address, `
		SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station),
		       (SELECT count(*) FROM station_state), (SELECT count(*) FROM incoming_station)`),
		[][]string{{"1", "1", "1", "0"}})
	// And the next import works.
	wantEqual(t, load(t, database, day2, bifo, salt), store.Loaded{Appeared: 1})
}

func TestLoadRefusesAPermitDateThatIsNotADate(t *testing.T) {
	database, _ := open(t)
	wrong := with(bifo, func(s *ofcom.MobileStation) {
		s.Permit = &ofcom.Permit{Type: ofcom.PermitSiteDataSheet, Date: "2021-02-30"}
	})

	_, err := database.LoadSnapshot(context.Background(), snapshotAt(day1, 1), []ofcom.MobileStation{wrong}, false)

	if err == nil || !strings.Contains(err.Error(), `the permit date "2021-02-30" is not a date`) {
		t.Errorf("got %v", err)
	}
}

// The address holds a password, which no error may show.
func TestOpenDoesNotShowThePassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tests := map[string]string{
		"postgres://nobody:secret@127.0.0.1:1/nothing":       "connecting to the database",
		"postgres://nobody:secret@127.0.0.1:no-port/nothing": "reading the database address",
	}
	for address, want := range tests {
		_, err := store.Open(ctx, address)

		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want an error containing %q", address, err, want)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("the error shows the password: %v", err)
		}
	}
}
