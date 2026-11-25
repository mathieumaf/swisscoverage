package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	checksumA = "f57bc656c5fe7885895f16cb4a7369ae1b8480520d5078b9c253f0122aeda6cc"
	checksumB = "c23c52e27c91e37562e9c8ad190537ef305a7daadb7b5ac72f5a6fe7d9d5663a"
)

// Two lines as the archive writes them: the first original is named after
// its day, the second after its day and time.
const index = `{"published":"2026-11-23T22:01:35.611529Z","sha256":"` + checksumA + `","sites":22777,"release":"raw-2026","file":"mobile-stations_2026-11-23.json.gz"}
{"published":"2026-11-24T22:01:55.936174Z","sha256":"` + checksumB + `","sites":22761,"release":"raw-2026","file":"mobile-stations_2026-11-24T220155Z.json.gz"}
`

func TestParseIndex(t *testing.T) {
	snapshots, err := ParseIndex([]byte(index))
	if err != nil {
		t.Fatal(err)
	}
	want := []Snapshot{
		{
			PublishedAt: time.Date(2026, 11, 23, 22, 1, 35, 611529000, time.UTC),
			SHA256:      checksumA,
			Sites:       22777,
			Release:     "raw-2026",
			File:        "mobile-stations_2026-11-23.json.gz",
		},
		{
			PublishedAt: time.Date(2026, 11, 24, 22, 1, 55, 936174000, time.UTC),
			SHA256:      checksumB,
			Sites:       22761,
			Release:     "raw-2026",
			File:        "mobile-stations_2026-11-24T220155Z.json.gz",
		},
	}
	if !reflect.DeepEqual(snapshots, want) {
		t.Errorf("got %+v, want %+v", snapshots, want)
	}
}

func TestParseIndexOfNothing(t *testing.T) {
	snapshots, err := ParseIndex([]byte("\n"))
	if err != nil || len(snapshots) != 0 {
		t.Errorf("got %v and %v, want no snapshot and no error", snapshots, err)
	}
}

// line returns a line of the index with one field replaced by a JSON value.
func line(field, value string) string {
	fields := map[string]string{
		"published": `"2026-11-23T22:01:35.611529Z"`,
		"sha256":    `"` + checksumA + `"`,
		"sites":     `22777`,
		"release":   `"raw-2026"`,
		"file":      `"mobile-stations_2026-11-23.json.gz"`,
	}
	fields[field] = value
	var parts []string
	for _, name := range []string{"published", "sha256", "sites", "release", "file"} {
		if fields[name] != "" {
			parts = append(parts, `"`+name+`":`+fields[name])
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestParseIndexRefusesWhatItDoesNotExpect(t *testing.T) {
	tests := []struct {
		name, index, want string
	}{
		{"a line that is not JSON", "not json", "line 1: not a JSON object"},
		{"a time that is not one", line("published", `"yesterday"`), `unexpected publication time "yesterday"`},
		{"a checksum in capitals", line("sha256", `"`+strings.ToUpper(checksumA)+`"`), "unexpected checksum"},
		{"a short checksum", line("sha256", `"abc"`), `unexpected checksum "abc"`},
		{"no number of sites", line("sites", ""), "no number of sites"},
		{"a negative number of sites", line("sites", "-1"), "no number of sites"},
		{"a release that is an address", line("release", `"../../evil"`), `unexpected release "../../evil"`},
		{"a file that is a path", line("file", `"../mobile-stations_2026-11-23.json.gz"`), "unexpected file name"},
		{"a file with a query", line("file", `"mobile-stations_2026-11-23.json.gz?x=1"`), "unexpected file name"},
		{"a file on another host", line("file", `"//evil.example/mobile-stations_2026-11-23.json.gz"`), "unexpected file name"},
		{
			"two lines in the wrong order",
			line("published", `"2026-11-24T22:01:55Z"`) + "\n" + line("published", `"2026-11-23T22:01:35Z"`),
			"line 2: published at 2026-11-23T22:01:35Z, which is not after the line before",
		},
		{
			"the same time twice",
			line("sites", "1") + "\n" + line("sites", "2"),
			"line 2: published at",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshots, err := ParseIndex([]byte(test.index))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("got %v and error %v, want an error containing %q", snapshots, err, test.want)
			}
		})
	}
}

func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// serve starts an archive that answers each path with the given body.
func serve(t *testing.T, files map[string][]byte) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	return &Client{
		IndexURL:    server.URL + "/index",
		ReleasesURL: server.URL + "/releases/",
		HTTP:        server.Client(),
	}
}

func TestClientReadsTheIndexAndAFile(t *testing.T) {
	original := []byte(`{"type":"FeatureCollection"}`)
	snapshot := Snapshot{
		SHA256:  checksum(original),
		Release: "raw-2026",
		File:    "mobile-stations_2026-11-23.json.gz",
	}
	client := serve(t, map[string][]byte{
		"/index": []byte(index),
		"/releases/raw-2026/mobile-stations_2026-11-23.json.gz": gzipped(t, original),
	})

	snapshots, err := client.Index(context.Background())
	if err != nil || len(snapshots) != 2 {
		t.Fatalf("got %d snapshots and error %v, want 2", len(snapshots), err)
	}
	data, err := client.Download(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Errorf("got %q, want the original", data)
	}
}

func TestClientRefusesAFileThatIsNotTheOneListed(t *testing.T) {
	original := []byte(`{"type":"FeatureCollection"}`)
	listed := Snapshot{
		SHA256:  checksum(original),
		Release: "raw-2026",
		File:    "mobile-stations_2026-11-23.json.gz",
	}
	path := "/releases/raw-2026/mobile-stations_2026-11-23.json.gz"
	tests := []struct {
		name     string
		files    map[string][]byte
		snapshot Snapshot
		want     string
	}{
		{
			name:     "other content",
			files:    map[string][]byte{path: gzipped(t, []byte(`{"type":"Feature"}`))},
			snapshot: listed,
			want:     "has checksum " + checksum([]byte(`{"type":"Feature"}`)) + ", and the index says " + listed.SHA256,
		},
		{
			name:     "not gzipped",
			files:    map[string][]byte{path: original},
			snapshot: listed,
			want:     "is not a gzip file",
		},
		{
			name:     "cut short",
			files:    map[string][]byte{path: gzipped(t, original)[:20]},
			snapshot: listed,
			want:     "decompressing mobile-stations_2026-11-23.json.gz",
		},
		{
			name:     "missing",
			files:    map[string][]byte{},
			snapshot: listed,
			want:     "answered 404 Not Found",
		},
		{
			name:  "a name that would leave the releases",
			files: map[string][]byte{path: gzipped(t, original)},
			snapshot: Snapshot{
				SHA256:  listed.SHA256,
				Release: "raw-2026/../raw-2026",
				File:    listed.File,
			},
			want: "unexpected file name",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := serve(t, test.files)

			data, err := client.Download(context.Background(), test.snapshot)

			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("got %d bytes and error %v, want an error containing %q", len(data), err, test.want)
			}
		})
	}
}

func TestClientReportsAnIndexItCannotRead(t *testing.T) {
	client := serve(t, map[string][]byte{"/index": []byte("not json\n")})
	if _, err := client.Index(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "reading the archive's index: line 1") {
		t.Errorf("an index that is not one: got %v", err)
	}

	client = serve(t, map[string][]byte{})
	if _, err := client.Index(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "answered 404 Not Found") {
		t.Errorf("a missing index: got %v", err)
	}
}

func TestReadAtMost(t *testing.T) {
	data, err := readAtMost(strings.NewReader("12345"), 5)
	if err != nil || string(data) != "12345" {
		t.Errorf("at the limit: got %q and %v", data, err)
	}
	if _, err := readAtMost(strings.NewReader("123456"), 5); err == nil ||
		!strings.Contains(err.Error(), "more than 5 bytes") {
		t.Errorf("over the limit: got %v", err)
	}
}
