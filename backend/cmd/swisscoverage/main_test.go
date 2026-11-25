package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

const (
	samplePath = "../../internal/ofcom/testdata/mobile-stations.sample.json"
	goldenPath = "../../internal/ofcom/testdata/mobile-stations.golden.json"
)

// noEnvironment stands for an environment where nothing is set.
func noEnvironment(string) string { return "" }

// call runs the program with args and returns what it printed and its exit
// status.
func call(t *testing.T, stdin string, args ...string) (stdout, stderr string, status int) {
	t.Helper()
	var out, errs bytes.Buffer
	status = run(context.Background(), args, strings.NewReader(stdin), &out, &errs, noEnvironment)
	return out.String(), errs.String(), status
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseSummary(t *testing.T) {
	stdout, stderr, status := call(t, "", "parse", samplePath)

	const want = `9 stations, 0 warnings

Per operator
  swisscom  4
  sunrise   1
  salt      2
  sbb       1
  other     1

Per kind
  outdoor            3
  outdoor_low_power  1
  indoor             1
  tunnel             1
  temporary          1
  unknown            2

Per technology (a station counts once for each one it carries)
  2G  2
  3G  2
  4G  7
  5G  5
`
	if stdout != want {
		t.Errorf("got:\n%s\nwant:\n%s", stdout, want)
	}
	if stderr != "" || status != 0 {
		t.Errorf("got status %d and stderr %q, want 0 and nothing", status, stderr)
	}
}

func TestParseListsWarningsAndStillSucceeds(t *testing.T) {
	changed := strings.Replace(readFile(t, samplePath),
		`"techno_en": "Technology 4G,5G"`, `"techno_en": "Technology 4G,6G"`, 1)

	stdout, stderr, status := call(t, changed, "parse", "-")

	if !strings.HasPrefix(stdout, "9 stations, 1 warnings\n") {
		t.Errorf("got:\n%s\nwant it to start with the number of warnings", stdout)
	}
	const want = "\nWarnings\n  Swisscom BIFO: unrecognised technology \"Technology 4G,6G\"\n"
	if !strings.HasSuffix(stdout, want) {
		t.Errorf("got:\n%s\nwant it to end with:%s", stdout, want)
	}
	if stderr != "" || status != 0 {
		t.Errorf("got status %d and stderr %q, want 0 and nothing", status, stderr)
	}
}

func TestParseJSON(t *testing.T) {
	want := readFile(t, goldenPath)

	fromFile, _, status := call(t, "", "parse", "-json", samplePath)
	if fromFile != want || status != 0 {
		t.Errorf("from a file: got status %d and output that differs from %s:\n%s",
			status, goldenPath, fromFile)
	}

	fromStdin, _, status := call(t, readFile(t, samplePath), "parse", "-json", "-")
	if fromStdin != want || status != 0 {
		t.Errorf("from standard input: got status %d and output that differs from %s:\n%s",
			status, goldenPath, fromStdin)
	}
}

func TestParseFailures(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		args       []string
		wantStatus int
		wantStderr string
	}{
		{
			name:       "a file that does not exist",
			args:       []string{"parse", "testdata/missing.json"},
			wantStatus: 1,
			wantStderr: "swisscoverage parse: open testdata/missing.json",
		},
		{
			name:       "a file that is not the dataset",
			stdin:      `{"type": "Feature"}`,
			args:       []string{"parse", "-"},
			wantStatus: 1,
			wantStderr: "swisscoverage parse: expected a GeoJSON FeatureCollection\n",
		},
		{
			name:       "no file",
			args:       []string{"parse"},
			wantStatus: 2,
			wantStderr: "Usage: swisscoverage parse",
		},
		{
			name:       "two files",
			args:       []string{"parse", samplePath, samplePath},
			wantStatus: 2,
			wantStderr: "Usage: swisscoverage parse",
		},
		{
			name:       "an unknown flag",
			args:       []string{"parse", "-yaml", samplePath},
			wantStatus: 2,
			wantStderr: "flag provided but not defined: -yaml",
		},
		{
			name:       "no command",
			wantStatus: 2,
			wantStderr: "Usage: swisscoverage <command>",
		},
		{
			name:       "an unknown command",
			args:       []string{"serve"},
			wantStatus: 2,
			wantStderr: `swisscoverage: unknown command "serve"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, status := call(t, test.stdin, test.args...)

			if status != test.wantStatus {
				t.Errorf("got status %d, want %d", status, test.wantStatus)
			}
			if !strings.Contains(stderr, test.wantStderr) {
				t.Errorf("got stderr %q, want it to contain %q", stderr, test.wantStderr)
			}
			if stdout != "" {
				t.Errorf("got output %q, want none", stdout)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	stdout, stderr, status := call(t, "", "help")
	if !strings.Contains(stdout, "Usage: swisscoverage <command>") || stderr != "" || status != 0 {
		t.Errorf("help: got status %d, output %q and stderr %q", status, stdout, stderr)
	}

	stdout, stderr, status = call(t, "", "parse", "-h")
	if !strings.Contains(stderr, "Usage: swisscoverage parse") || stdout != "" || status != 0 {
		t.Errorf("parse -h: got status %d, output %q and stderr %q", status, stdout, stderr)
	}
}

// failingWriter refuses every write, like a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("the pipe is closed") }

func TestParseReportsAFailedWrite(t *testing.T) {
	for _, args := range [][]string{{"parse", samplePath}, {"parse", "-json", samplePath}} {
		var stderr bytes.Buffer
		status := run(context.Background(), args, strings.NewReader(""), failingWriter{}, &stderr, noEnvironment)

		if status != 1 || !strings.Contains(stderr.String(), "the pipe is closed") {
			t.Errorf("%v: got status %d and stderr %q, want 1 and the write error",
				args, status, stderr.String())
		}
	}
}
