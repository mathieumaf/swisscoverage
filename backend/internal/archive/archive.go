// Package archive reads the daily archive of OFCOM's files, which lives in a
// repository of its own: an index with one line per snapshot, and each file
// as OFCOM published it, gzipped, attached to a release.
//
// Nothing the archive says is trusted. Every value of the index is checked
// before it is used, downloads are bounded, and a file is returned only when
// its checksum is the one the index gives.
package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Where the archive of the public instance lives.
const (
	DefaultIndexURL    = "https://raw.githubusercontent.com/mathieumaf/swisscoverage-data/main/ofcom/mobile-stations/snapshots.ndjson"
	DefaultReleasesURL = "https://github.com/mathieumaf/swisscoverage-data/releases/download"
)

// The most that is read. OFCOM's file is about 28 MB, a little over 1 MB
// once gzipped, and an index line about 200 bytes.
const (
	maxIndexBytes      = 16 << 20
	maxCompressedBytes = 128 << 20
	maxFileBytes       = 512 << 20
)

var (
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	releasePattern = regexp.MustCompile(`^raw-\d{4}$`)
	filePattern    = regexp.MustCompile(`^mobile-stations_\d{4}-\d{2}-\d{2}(T\d{6}Z)?\.json\.gz$`)
)

// Snapshot is one line of the index: a file OFCOM published, and where the
// archive keeps it.
type Snapshot struct {
	// PublishedAt is when OFCOM published the file, to the microsecond.
	PublishedAt time.Time
	// SHA256 is the checksum of the file as published, in hexadecimal.
	SHA256 string
	// Sites is the number of sites in the file.
	Sites int
	// Release and File name the gzipped original: the release it is attached
	// to and its name there.
	Release, File string
}

// Client reads an archive over HTTP.
type Client struct {
	// IndexURL is the address of the index. ReleasesURL is the address under
	// which a file is found at <release>/<file>.
	IndexURL, ReleasesURL string
	// HTTP is the client to use. Its timeout bounds each download.
	HTTP *http.Client
}

// Index returns the snapshots of the archive, oldest first.
func (c *Client) Index(ctx context.Context) ([]Snapshot, error) {
	data, err := c.get(ctx, c.IndexURL, maxIndexBytes)
	if err != nil {
		return nil, fmt.Errorf("reading the archive's index: %w", err)
	}
	snapshots, err := ParseIndex(data)
	if err != nil {
		return nil, fmt.Errorf("reading the archive's index: %w", err)
	}
	return snapshots, nil
}

// Download returns a snapshot's file as OFCOM published it. It fails when the
// file is not the one the index describes.
func (c *Client) Download(ctx context.Context, snapshot Snapshot) ([]byte, error) {
	// Checked again here: these two values become part of an address.
	if !releasePattern.MatchString(snapshot.Release) || !filePattern.MatchString(snapshot.File) {
		return nil, fmt.Errorf("unexpected file name %q in release %q", snapshot.File, snapshot.Release)
	}
	url := strings.TrimSuffix(c.ReleasesURL, "/") + "/" + snapshot.Release + "/" + snapshot.File
	compressed, err := c.get(ctx, url, maxCompressedBytes)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", snapshot.File, err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("%s is not a gzip file: %w", snapshot.File, err)
	}
	data, err := readAtMost(reader, maxFileBytes)
	if err != nil {
		return nil, fmt.Errorf("decompressing %s: %w", snapshot.File, err)
	}
	sum := sha256.Sum256(data)
	if actual := hex.EncodeToString(sum[:]); actual != snapshot.SHA256 {
		return nil, fmt.Errorf("%s has checksum %s, and the index says %s",
			snapshot.File, actual, snapshot.SHA256)
	}
	return data, nil
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, response.Status)
	}
	return readAtMost(response.Body, limit)
}

func readAtMost(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("more than %d bytes", limit)
	}
	return data, nil
}

// ParseIndex reads the index: one JSON object per line, oldest snapshot
// first. It checks every value, and that the publication times increase.
func ParseIndex(data []byte) ([]Snapshot, error) {
	var snapshots []Snapshot
	for number, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		snapshot, err := parseIndexLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", number+1, err)
		}
		if n := len(snapshots); n > 0 && !snapshot.PublishedAt.After(snapshots[n-1].PublishedAt) {
			return nil, fmt.Errorf("line %d: published at %s, which is not after the line before",
				number+1, snapshot.PublishedAt.Format(time.RFC3339Nano))
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func parseIndexLine(line string) (Snapshot, error) {
	var entry struct {
		Published string `json:"published"`
		SHA256    string `json:"sha256"`
		Sites     *int   `json:"sites"`
		Release   string `json:"release"`
		File      string `json:"file"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return Snapshot{}, fmt.Errorf("not a JSON object: %w", err)
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, entry.Published)
	if err != nil {
		return Snapshot{}, fmt.Errorf("unexpected publication time %q", entry.Published)
	}
	if !sha256Pattern.MatchString(entry.SHA256) {
		return Snapshot{}, fmt.Errorf("unexpected checksum %q", entry.SHA256)
	}
	if entry.Sites == nil || *entry.Sites < 0 {
		return Snapshot{}, fmt.Errorf("no number of sites")
	}
	if !releasePattern.MatchString(entry.Release) {
		return Snapshot{}, fmt.Errorf("unexpected release %q", entry.Release)
	}
	if !filePattern.MatchString(entry.File) {
		return Snapshot{}, fmt.Errorf("unexpected file name %q", entry.File)
	}
	return Snapshot{
		// The database keeps microseconds, and so does the archive.
		PublishedAt: publishedAt.UTC().Truncate(time.Microsecond),
		SHA256:      entry.SHA256,
		Sites:       *entry.Sites,
		Release:     entry.Release,
		File:        entry.File,
	}, nil
}
