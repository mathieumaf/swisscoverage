package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mathieumaf/swisscoverage/backend/internal/testdb"
)

// callWith runs the program with args against the database at address.
func callWith(t *testing.T, address string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	getenv := func(name string) string {
		if name == "DATABASE_URL" {
			return address
		}
		return ""
	}
	var out, errs bytes.Buffer
	status = run(context.Background(), args, strings.NewReader(""), &out, &errs, getenv)
	return out.String(), errs.String(), status
}

// migrated returns the address of an empty database with the schema applied.
func migrated(t *testing.T) string {
	t.Helper()
	address := testdb.New(t)
	if _, stderr, status := callWith(t, address, "migrate"); status != 0 {
		t.Fatalf("migrate failed: %s", stderr)
	}
	return address
}

func TestMigrateCommand(t *testing.T) {
	address := testdb.New(t)

	stdout, stderr, status := callWith(t, address, "migrate")
	if stdout != "applied 0001_stations.sql\napplied 0002_station_position_indexes.sql\n"+
		"applied 0003_current_station.sql\nThe schema is now at version 3.\n" ||
		stderr != "" || status != 0 {
		t.Errorf("first run: got status %d, output %q and stderr %q", status, stdout, stderr)
	}

	stdout, stderr, status = callWith(t, address, "migrate")
	if stdout != "The schema is up to date (version 3).\n" || stderr != "" || status != 0 {
		t.Errorf("second run: got status %d, output %q and stderr %q", status, stdout, stderr)
	}
}

// A program rolled back to an earlier version finds a newer schema, and
// leaves it alone.
func TestMigrateLeavesANewerSchemaAlone(t *testing.T) {
	address := migrated(t)
	testdb.Query(t, address,
		`INSERT INTO schema_migration (version, name) VALUES (9999, '9999_from_the_future.sql')`)

	stdout, stderr, status := callWith(t, address, "migrate")

	const want = "The schema is at version 9999, newer than this program (version 3). Nothing to do.\n"
	if stdout != want || stderr != "" || status != 0 {
		t.Errorf("got status %d, output %q and stderr %q", status, stdout, stderr)
	}
	recorded := testdb.Query(t, address, `SELECT count(*) FROM schema_migration`)
	if recorded[0][0] != "4" {
		t.Errorf("got %s migrations recorded, want the three of this program and the newer one", recorded[0][0])
	}
}

func TestCommandsNeedADatabase(t *testing.T) {
	for _, command := range []string{"migrate", "ingest", "serve"} {
		stdout, stderr, status := call(t, "", command)
		if status != 1 || stdout != "" || !strings.Contains(stderr, "DATABASE_URL is not set") {
			t.Errorf("%s without DATABASE_URL: got status %d, output %q and stderr %q",
				command, status, stdout, stderr)
		}

		_, stderr, status = callWith(t, "postgres://nobody:secret@127.0.0.1:1/nothing", command)
		if status != 1 || !strings.Contains(stderr, "connecting to the database") ||
			strings.Contains(stderr, "secret") {
			t.Errorf("%s with a database that does not answer: got status %d and stderr %q",
				command, status, stderr)
		}

		_, stderr, status = call(t, "", command, "extra")
		if status != 2 || !strings.Contains(stderr, "Usage: swisscoverage "+command) {
			t.Errorf("%s with an argument too many: got status %d and stderr %q", command, status, stderr)
		}
	}
}

// An archive is what the ingest command reads: an index and, for each of its
// lines, the gzipped file. Tests build one from the parser's sample.
type testArchive struct {
	t      *testing.T
	server *httptest.Server
	index  []string
	files  map[string][]byte
}

func newArchive(t *testing.T) *testArchive {
	t.Helper()
	archive := &testArchive{t: t, files: map[string][]byte{}}
	archive.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/snapshots.ndjson" {
			fmt.Fprint(w, strings.Join(archive.index, ""))
			return
		}
		body, ok := archive.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(archive.server.Close)
	return archive
}

// arguments returns what points the ingest command at this archive.
func (a *testArchive) arguments() []string {
	return []string{
		"ingest",
		"-index", a.server.URL + "/snapshots.ndjson",
		"-releases", a.server.URL + "/releases",
	}
}

// add publishes a file on a day of October 2026, safely in the past and lists it in the index.
// It returns the file's checksum.
func (a *testArchive) add(day int, file []byte) string {
	a.t.Helper()
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	writer.Write(file)
	if err := writer.Close(); err != nil {
		a.t.Fatal(err)
	}
	name := fmt.Sprintf("mobile-stations_2026-10-%02d.json.gz", day)
	a.files["/releases/raw-2026/"+name] = compressed.Bytes()

	var document struct {
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(file, &document); err != nil {
		a.t.Fatal(err)
	}
	sum := sha256.Sum256(file)
	checksum := hex.EncodeToString(sum[:])
	a.index = append(a.index, fmt.Sprintf(
		`{"published":"2026-10-%02dT22:01:35.611529Z","sha256":"%s","sites":%d,"release":"raw-2026","file":"%s"}`+"\n",
		day, checksum, len(document.Features), name))
	return checksum
}

// sampleWith returns the parser's sample after change has edited its
// features, each a GeoJSON feature decoded as a map.
func sampleWith(t *testing.T, change func(features []map[string]any) []map[string]any) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(readFile(t, samplePath)), &document); err != nil {
		t.Fatal(err)
	}
	var features []map[string]any
	for _, feature := range document["features"].([]any) {
		features = append(features, feature.(map[string]any))
	}
	document["features"] = change(features)
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func unchanged(features []map[string]any) []map[string]any { return features }

// setProperty replaces one property of the feature at index.
func setProperty(features []map[string]any, index int, key, value string) {
	features[index]["properties"].(map[string]any)[key] = value
}

func TestIngestLoadsTheArchiveInOrder(t *testing.T) {
	address := migrated(t)
	archive := newArchive(t)
	// The sample has nine stations. The first day lacks the last one.
	archive.add(23, sampleWith(t, func(features []map[string]any) []map[string]any {
		return features[:8]
	}))
	// The second day: the first station is gone, the second gains a
	// technology, the ninth arrives.
	archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
		setProperty(features, 1, "techno_en", "Technology 2G,3G,4G,5G")
		return features[1:]
	}))
	// The third day changes nothing.
	archive.add(25, sampleWith(t, func(features []map[string]any) []map[string]any {
		setProperty(features, 1, "techno_en", "Technology 2G,3G,4G,5G")
		// The order of the file does not matter.
		return append(features[2:], features[1])
	}))

	stdout, stderr, status := callWith(t, address, archive.arguments()...)

	const want = `2026-10-23T22:01:35Z: 8 sites, 8 appeared, 0 disappeared, 0 changed, 0 warnings
2026-10-24T22:01:35Z: 8 sites, 1 appeared, 1 disappeared, 1 changed, 0 warnings
2026-10-25T22:01:35Z: 8 sites, 0 appeared, 0 disappeared, 0 changed, 0 warnings
Loaded 3 snapshots. The database holds the 3 snapshots of the archive.
`
	if stdout != want || stderr != "" || status != 0 {
		t.Errorf("got status %d, stderr %q and output:\n%s\nwant:\n%s", status, stderr, stdout, want)
	}
	counts := testdb.Query(t, address, `
		SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station),
		       (SELECT count(*) FROM station_state),
		       (SELECT count(*) FROM station_state WHERE valid_to IS NULL)`)
	if got := strings.Join(counts[0], " "); got != "3 9 10 8" {
		t.Errorf("got %s, want 3 snapshots, 9 stations, 10 states of which 8 current", got)
	}

	// Running it again changes nothing.
	stdout, stderr, status = callWith(t, address, archive.arguments()...)
	if stdout != "Nothing to load: the database holds the 3 snapshots of the archive.\n" ||
		stderr != "" || status != 0 {
		t.Errorf("second run: got status %d, output %q and stderr %q", status, stdout, stderr)
	}
	again := testdb.Query(t, address, `
		SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station),
		       (SELECT count(*) FROM station_state),
		       (SELECT count(*) FROM station_state WHERE valid_to IS NULL)`)
	if got := strings.Join(again[0], " "); got != "3 9 10 8" {
		t.Errorf("after a second run: got %s, want what the first run left", got)
	}

	// A snapshot archived later is the only one loaded.
	archive.add(26, sampleWith(t, unchanged))
	stdout, _, status = callWith(t, address, archive.arguments()...)
	const wantLater = `2026-10-26T22:01:35Z: 9 sites, 1 appeared, 0 disappeared, 1 changed, 0 warnings
Loaded 1 snapshot. The database holds the 4 snapshots of the archive.
`
	if stdout != wantLater || status != 0 {
		t.Errorf("after a new snapshot: got status %d and output:\n%s\nwant:\n%s", status, stdout, wantLater)
	}
}

func TestIngestReportsWarningsAndStillLoads(t *testing.T) {
	address := migrated(t)
	archive := newArchive(t)
	archive.add(23, sampleWith(t, func(features []map[string]any) []map[string]any {
		setProperty(features, 0, "typ_en", "Balloon")
		// The same site twice.
		return append(features, features[1])
	}))

	stdout, stderr, status := callWith(t, address, archive.arguments()...)

	if !strings.HasPrefix(stdout,
		"2026-10-23T22:01:35Z: 10 sites, 9 appeared, 0 disappeared, 0 changed, 2 warnings\n") || status != 0 {
		t.Errorf("got status %d and output %q", status, stdout)
	}
	const want = `2026-10-23T22:01:35Z: warning: Swisscom BIFO: unrecognised type "Balloon"
2026-10-23T22:01:35Z: warning: Salt TI_3787A at 2705499, 1132611: published more than once, the first is kept
`
	if stderr != want {
		t.Errorf("got stderr:\n%s\nwant:\n%s", stderr, want)
	}
	kind := testdb.Query(t, address, `
		SELECT st.kind FROM station s JOIN station_state st ON st.station_id = s.id
		WHERE s.name = 'Swisscom BIFO'`)
	if kind[0][0] != "unknown" {
		t.Errorf("got kind %q, want the station kept as unknown", kind[0][0])
	}
}

// Two runs at once: the one that finds a snapshot loaded by the other while
// it downloaded says so, and goes on.
func TestIngestBesideAnotherRun(t *testing.T) {
	address := migrated(t)
	archive := newArchive(t)
	archive.add(23, sampleWith(t, unchanged))
	archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
		return features[1:]
	}))

	// The first download of the first file starts another run, which loads
	// the whole archive before the file reaches this run.
	first := "/releases/raw-2026/mobile-stations_2026-10-23.json.gz"
	serve := archive.server.Config.Handler
	var started atomic.Bool
	type result struct {
		stdout, stderr string
		status         int
	}
	others := make(chan result, 1)
	archive.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == first && started.CompareAndSwap(false, true) {
			stdout, stderr, status := callWith(t, address, archive.arguments()...)
			others <- result{stdout, stderr, status}
		}
		serve.ServeHTTP(w, r)
	})

	stdout, stderr, status := callWith(t, address, archive.arguments()...)

	var other result
	select {
	case other = <-others:
	default:
		t.Fatalf("the other run never started: got status %d, stderr %q and output %q", status, stderr, stdout)
	}
	if other.status != 0 || other.stderr != "" || !strings.HasSuffix(other.stdout,
		"Loaded 2 snapshots. The database holds the 2 snapshots of the archive.\n") {
		t.Fatalf("the other run: got status %d, output %q and stderr %q", other.status, other.stdout, other.stderr)
	}
	const want = `2026-10-23T22:01:35Z: already loaded by another run
2026-10-24T22:01:35Z: already loaded by another run
Loaded 0 snapshots. The database holds the 2 snapshots of the archive.
`
	if stdout != want || stderr != "" || status != 0 {
		t.Errorf("got status %d, stderr %q and output:\n%s\nwant:\n%s", status, stderr, stdout, want)
	}
	counts := testdb.Query(t, address, `SELECT count(*) FROM snapshot`)
	if counts[0][0] != "2" {
		t.Errorf("got %s snapshots, want each loaded once", counts[0][0])
	}
}

func TestIngestNeedsTheSchema(t *testing.T) {
	address := testdb.New(t)
	archive := newArchive(t)
	archive.add(23, sampleWith(t, unchanged))

	stdout, stderr, status := callWith(t, address, archive.arguments()...)

	if status != 1 || stdout != "" || !strings.Contains(stderr, "run `swisscoverage migrate`") {
		t.Errorf("got status %d, output %q and stderr %q", status, stdout, stderr)
	}
}

// What the archive says is not trusted. Each of these stops the import, and
// what was loaded before stays.
func TestIngestRefusesAnArchiveItCannotTrust(t *testing.T) {
	tests := []struct {
		name string
		// spoil changes the archive after its first day is in place.
		spoil func(t *testing.T, archive *testArchive)
		want  string
	}{
		{
			name: "a file that is not the one listed",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, unchanged))
				other := newArchive(t)
				other.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
					return features[:5]
				}))
				const path = "/releases/raw-2026/mobile-stations_2026-10-24.json.gz"
				archive.files[path] = other.files[path]
			},
			want: "mobile-stations_2026-10-24.json.gz has checksum",
		},
		{
			name: "a number of sites that is not the file's",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, unchanged))
				archive.index[1] = strings.Replace(archive.index[1], `"sites":9`, `"sites":7`, 1)
			},
			want: "the snapshot published at 2026-10-24T22:01:35Z has 9 sites, and the index says 7",
		},
		{
			name: "a file that is not the dataset",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, []byte(`{"type": "FeatureCollection", "features": []}`))
			},
			want: "the snapshot published at 2026-10-24T22:01:35Z: expected coordinates in EPSG:2056, found no CRS",
		},
		{
			name: "a file with less than half the sites",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
					return features[:4]
				}))
			},
			want: "has 4 sites, less than half the 9 of the one before",
		},
		{
			name: "a loaded snapshot whose checksum changed",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.index[0] = strings.Replace(archive.index[0], `"sha256":"`, `"sha256":"`+strings.Repeat("0", 64)+`","was":"`, 1)
			},
			want: "and the archive's index now says " + strings.Repeat("0", 64),
		},
		{
			name: "a snapshot older than what is loaded",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(22, sampleWith(t, unchanged))
				// The index is in order: the new line goes first.
				archive.index[0], archive.index[1] = archive.index[1], archive.index[0]
			},
			want: "the snapshot published at 2026-10-22T22:01:35.611529Z is in the archive's index but not in the database",
		},
		{
			name: "a file that repeats one site to look as large",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
					for index := range features {
						features[index] = features[0]
					}
					return features
				}))
			},
			want: "has 1 sites, less than half the 9 of the one before",
		},
		{
			name: "an index that no longer lists what is loaded",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, unchanged))
				archive.index = archive.index[1:]
			},
			want: "the database holds the snapshot published at 2026-10-23T22:01:35.611529Z, " +
				"and the archive's index does not list it",
		},
		{
			name: "an empty index",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.index = nil
			},
			want: "the archive's index, which lists 0 snapshots, does not",
		},
		{
			name: "a snapshot dated in the future",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.add(24, sampleWith(t, unchanged))
				archive.index[1] = strings.Replace(archive.index[1], `"published":"2026-`, `"published":"2999-`, 1)
			},
			want: "the snapshot published at 2999-10-24T22:01:35Z is dated more than a day in the future",
		},
		{
			name: "an index that is not one",
			spoil: func(t *testing.T, archive *testArchive) {
				archive.index = append(archive.index, "not json\n")
			},
			want: "reading the archive's index: line 2: not a JSON object",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			address := migrated(t)
			archive := newArchive(t)
			archive.add(23, sampleWith(t, unchanged))
			if _, stderr, status := callWith(t, address, archive.arguments()...); status != 0 {
				t.Fatalf("loading the first day failed: %s", stderr)
			}
			test.spoil(t, archive)

			stdout, stderr, status := callWith(t, address, archive.arguments()...)

			if status != 1 || !strings.Contains(stderr, test.want) {
				t.Errorf("got status %d, output %q and stderr %q, want status 1 and %q",
					status, stdout, stderr, test.want)
			}
			counts := testdb.Query(t, address, `
				SELECT (SELECT count(*) FROM snapshot), (SELECT count(*) FROM station_state)`)
			if got := strings.Join(counts[0], " "); got != "1 9" {
				t.Errorf("got %s, want the first day's snapshot and its 9 states untouched", got)
			}
		})
	}
}

// A person allows one snapshot, the one they looked at, by its publication
// time. The allowance does not carry over to another one.
func TestIngestLoadsASmallerFileWhenTold(t *testing.T) {
	address := migrated(t)
	archive := newArchive(t)
	archive.add(23, sampleWith(t, unchanged))
	archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
		return features[:4]
	}))
	archive.add(25, sampleWith(t, func(features []map[string]any) []map[string]any {
		return features[:1]
	}))

	_, stderr, status := callWith(t, address, archive.arguments()...)
	if status != 1 || !strings.Contains(stderr, "then run again with -allow-drop 2026-10-24T22:01:35.611529Z to load it") {
		t.Fatalf("without the flag: got status %d and stderr %q", status, stderr)
	}

	// Allowing another snapshot allows nothing here, and neither does the
	// same second without its fraction.
	for _, other := range []string{"2026-10-25T22:01:35.611529Z", "2026-10-24T22:01:35Z"} {
		_, stderr, status = callWith(t, address, append(archive.arguments(), "-allow-drop", other)...)
		if status != 1 || !strings.Contains(stderr, "has 4 sites, less than half the 9 of the one before") {
			t.Errorf("allowing %s: got status %d and stderr %q", other, status, stderr)
		}
	}
	_, stderr, status = callWith(t, address, append(archive.arguments(), "-allow-drop", "yes")...)
	if status != 2 || !strings.Contains(stderr, "-allow-drop takes a publication time") {
		t.Errorf("a value that is not a time: got status %d and stderr %q", status, stderr)
	}

	stdout, stderr, status := callWith(t, address,
		append(archive.arguments(), "-allow-drop", "2026-10-24T22:01:35.611529Z")...)
	if !strings.Contains(stdout,
		"2026-10-24T22:01:35Z: 4 sites, 0 appeared, 5 disappeared, 0 changed, 0 warnings\n") {
		t.Errorf("the allowed file was not loaded: output %q and stderr %q", stdout, stderr)
	}
	if status != 1 || !strings.Contains(stderr, "has 1 sites, less than half the 4 of the one before") {
		t.Errorf("the next one: got status %d and stderr %q, want it refused", status, stderr)
	}
}

// After a failure, the next run picks up where the import stopped.
func TestIngestResumesAfterAFailure(t *testing.T) {
	address := migrated(t)
	archive := newArchive(t)
	archive.add(23, sampleWith(t, unchanged))
	archive.add(24, sampleWith(t, func(features []map[string]any) []map[string]any {
		return features[1:]
	}))
	const path = "/releases/raw-2026/mobile-stations_2026-10-24.json.gz"
	file := archive.files[path]
	delete(archive.files, path)

	stdout, stderr, status := callWith(t, address, archive.arguments()...)
	if status != 1 || !strings.Contains(stderr, "answered 404 Not Found") ||
		!strings.HasPrefix(stdout, "2026-10-23T22:01:35Z: 9 sites, 9 appeared") {
		t.Errorf("with a missing file: got status %d, output %q and stderr %q", status, stdout, stderr)
	}

	archive.files[path] = file
	stdout, stderr, status = callWith(t, address, archive.arguments()...)
	const want = `2026-10-24T22:01:35Z: 8 sites, 0 appeared, 1 disappeared, 0 changed, 0 warnings
Loaded 1 snapshot. The database holds the 2 snapshots of the archive.
`
	if stdout != want || stderr != "" || status != 0 {
		t.Errorf("once the file is back: got status %d, stderr %q and output:\n%s", status, stderr, stdout)
	}
}
