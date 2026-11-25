package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/testdb"
)

// syncBuffer is a buffer that the server and the test may use at once.
type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestServeCommand(t *testing.T) {
	address := migrated(t)
	getenv := func(name string) string {
		if name == "DATABASE_URL" {
			return address
		}
		return ""
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	var stdout, stderr syncBuffer
	go func() { done <- serve(ctx, "127.0.0.1:0", ready, &stdout, &stderr, getenv) }()

	var listening string
	select {
	case listening = <-ready:
	case err := <-done:
		t.Fatalf("serve stopped before listening: %v, %s", err, stderr.String())
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not start")
	}
	for path, status := range map[string]int{
		"/v1/snapshots": http.StatusOK,
		// Nothing is loaded yet.
		"/v1/stations": http.StatusServiceUnavailable,
	} {
		response, err := http.Get("http://" + listening + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != status {
			t.Errorf("%s: got status %d, want %d", path, response.StatusCode, status)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not stop")
	}
	want := "Serving the API on http://" + listening + "/v1/\nStopped.\n"
	if stdout.String() != want {
		t.Errorf("got output %q, want %q", stdout.String(), want)
	}
}

func TestServeNeedsTheSchema(t *testing.T) {
	_, stderr, status := callWith(t, testdb.New(t), "serve", "-listen", "127.0.0.1:0")
	if status != 1 || !strings.Contains(stderr, "run `swisscoverage migrate`") {
		t.Errorf("got status %d and stderr %q", status, stderr)
	}
}

// The contract committed at the root of the repository is the one the code
// prints. To update it, run in backend/:
//
//	go run ./cmd/swisscoverage openapi > ../openapi.json
func TestOpenAPIIsCommitted(t *testing.T) {
	stdout, stderr, status := call(t, "", "openapi")
	if status != 0 || stderr != "" {
		t.Fatalf("got status %d and stderr %q", status, stderr)
	}
	committed, err := os.ReadFile("../../../openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != string(committed) {
		t.Error("openapi.json is not what `swisscoverage openapi` prints: run " +
			"`go run ./cmd/swisscoverage openapi > ../openapi.json` in backend/ and commit the result")
	}

	_, stderr, status = call(t, "", "openapi", "extra")
	if status != 2 || !strings.Contains(stderr, "Usage: swisscoverage openapi") {
		t.Errorf("with an argument too many: got status %d and stderr %q", status, stderr)
	}
}
