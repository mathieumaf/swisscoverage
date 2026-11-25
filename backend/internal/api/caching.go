package api

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// How long a client or a cache may reuse a response without asking again.
// The data changes once a day, so a few minutes of delay cost nothing, and
// spare the database most of the traffic.
const cacheControl = "public, max-age=300"

// Responses smaller than this are sent as they are: compressing them saves
// less than it costs.
const minCompressed = 1024

// withCaching answers every request with the headers that let it be cached,
// and the conditional requests that a cache sends to revalidate.
//
// A successful response gets an ETag computed from its body, so it changes
// exactly when the response does: when a snapshot is loaded, and when a new
// version of the program writes the response differently. A request that
// already holds that ETag gets 304 Not Modified and no body. Bodies are
// compressed with gzip for clients that accept it. Any origin may read the
// API from a browser: the data is public, and no request carries
// credentials.
//
// Responses are held in memory to be hashed. Each is bounded, by the limits
// on the pages and by the size of the dataset.
func withCaching(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Access-Control-Allow-Origin", "*")
		header.Set("Access-Control-Expose-Headers", "ETag")
		header.Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodOptions {
			// The preflight of a browser that sends If-None-Match itself.
			header.Set("Access-Control-Allow-Methods", "GET, HEAD")
			header.Set("Access-Control-Allow-Headers", "If-None-Match")
			header.Set("Access-Control-Max-Age", "86400")
			header.Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			header.Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
			return
		}

		recorded := &recorder{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(recorded, r)
		for key, values := range recorded.header {
			header[key] = values
		}
		header.Add("Vary", "Accept-Encoding")
		if recorded.status != http.StatusOK {
			// Errors are not cached: what is missing now may be loaded at
			// the next import.
			header.Set("Cache-Control", "no-store")
			w.WriteHeader(recorded.status)
			w.Write(recorded.body.Bytes())
			return
		}

		body := recorded.body.Bytes()
		sum := sha256.Sum256(body)
		// Weak, because the same tag is given to the compressed and the
		// uncompressed body, which are equivalent but not identical.
		etag := `W/"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`
		header.Set("ETag", etag)
		header.Set("Cache-Control", cacheControl)
		if matches(r.Header.Get("If-None-Match"), etag) {
			header.Del("Content-Type")
			header.Del("Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return
		}

		if len(body) >= minCompressed && acceptsGzip(r.Header.Values("Accept-Encoding")) {
			var compressed bytes.Buffer
			writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
			writer.Write(body)
			writer.Close()
			body = compressed.Bytes()
			header.Set("Content-Encoding", "gzip")
		}
		header.Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			w.Write(body)
		}
	})
}

// recorder holds a response until it is complete.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(status int) {
	if !r.wrote {
		r.status = status
		r.wrote = true
	}
}

func (r *recorder) Write(data []byte) (int, error) {
	r.wrote = true
	return r.body.Write(data)
}

// matches says whether an If-None-Match header names etag, with the weak
// comparison that RFC 9110 prescribes for it.
func matches(ifNoneMatch, etag string) bool {
	for candidate := range strings.SplitSeq(ifNoneMatch, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
			return true
		}
	}
	return false
}

// acceptsGzip says whether Accept-Encoding names gzip with a quality above
// zero. A wildcard is not taken as a request for gzip: every client that
// wants it names it.
func acceptsGzip(acceptEncoding []string) bool {
	for _, field := range acceptEncoding {
		for part := range strings.SplitSeq(field, ",") {
			coding, params, _ := strings.Cut(part, ";")
			if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
				continue
			}
			for param := range strings.SplitSeq(params, ";") {
				name, value, _ := strings.Cut(param, "=")
				if strings.EqualFold(strings.TrimSpace(name), "q") {
					quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
					return err == nil && quality > 0
				}
			}
			return true
		}
	}
	return false
}

// describeCaching writes into the contract what withCaching does, since the
// operations do not set those headers themselves.
func describeCaching(openAPI *huma.OpenAPI) {
	stringSchema := &huma.Schema{Type: huma.TypeString}
	for _, path := range openAPI.Paths {
		operation := path.Get
		if operation == nil {
			continue
		}
		if ok := operation.Responses["200"]; ok != nil {
			if ok.Headers == nil {
				ok.Headers = map[string]*huma.Param{}
			}
			ok.Headers["ETag"] = &huma.Param{
				Description: "Changes exactly when the response does. Send it back in If-None-Match to get 304 when nothing changed.",
				Schema:      stringSchema,
			}
			ok.Headers["Cache-Control"] = &huma.Param{
				Description: "The response may be reused for five minutes: the data changes once a day.",
				Schema:      stringSchema,
			}
		}
		operation.Responses["304"] = &huma.Response{
			Description: "Not modified: the response named by If-None-Match is still the current one.",
		}
		hasIfNoneMatch := false
		for _, param := range operation.Parameters {
			if param.In == "header" && strings.EqualFold(param.Name, "If-None-Match") {
				hasIfNoneMatch = true
			}
		}
		if !hasIfNoneMatch {
			operation.Parameters = append(operation.Parameters, &huma.Param{
				Name:        "If-None-Match",
				In:          "header",
				Description: "The ETag of a response already held.",
				Schema:      stringSchema,
			})
		}
	}
}
