package api_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/api"
	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
	"github.com/mathieumaf/swisscoverage/backend/internal/store/storetest"
)

// Sites in Bern, about 40 metres apart, one in Zurich and one in Lugano.
var (
	bernA = storetest.Station("Swisscom BMAO", 2600640, 1199640)
	bernB = storetest.With(storetest.Station("Salt BE_0001A", 2600680, 1199620), func(s *ofcom.MobileStation) {
		s.Technologies = []ofcom.Technology{ofcom.Technology2G}
		s.Kind = ofcom.KindIndoor
		s.Adaptive = nil
		s.Permit = &ofcom.Permit{Type: ofcom.PermitExempt}
		s.InstallationLimitValue = nil
	})
	zurich = storetest.Station("Sunrise ZH601-1", 2683328, 1247916)
	lugano = storetest.With(storetest.Station("Salt TI_3787A", 2717696, 1102456), func(s *ofcom.MobileStation) {
		s.PowerClass = ofcom.PowerHigh
		s.Permit = nil
	})
	// A label with what an address must escape.
	odd = storetest.Station("Other A/B %1?#", 2600000, 1200000)
)

type fixture struct {
	t        *testing.T
	database *store.Store
	server   *httptest.Server
}

// newAPI serves the API on an empty database with the schema applied.
func newAPI(t *testing.T) *fixture {
	t.Helper()
	writer, address := storetest.Open(t)
	reader, err := store.OpenReadOnly(context.Background(), address, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.Close)
	server := httptest.NewServer(api.New(reader, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Minute))
	t.Cleanup(server.Close)
	return &fixture{t: t, database: writer, server: server}
}

func (f *fixture) load(publishedAt time.Time, stations ...ofcom.MobileStation) {
	f.t.Helper()
	_, err := f.database.LoadSnapshot(context.Background(),
		storetest.Snapshot(publishedAt, len(stations)), stations, false)
	if err != nil {
		f.t.Fatal(err)
	}
}

// get requests path and returns the response, its body read.
func (f *fixture) get(path string, headers ...string) (*http.Response, []byte) {
	f.t.Helper()
	request, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	for index := 0; index+1 < len(headers); index += 2 {
		request.Header.Set(headers[index], headers[index+1])
	}
	// Without a transport that decompresses, to see what is sent.
	transport := &http.Transport{DisableCompression: true}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	return response, body
}

// getJSON requests path, checks the status and decodes the body.
func (f *fixture) getJSON(path string, status int) map[string]any {
	f.t.Helper()
	response, body := f.get(path)
	if response.StatusCode != status {
		f.t.Fatalf("GET %s: got status %d, want %d: %s", path, response.StatusCode, status, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		f.t.Fatalf("GET %s: %v: %s", path, err, body)
	}
	return decoded
}

func wantEqual[T any](t *testing.T, what string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", what, got, want)
	}
}

// ids returns the id of each item of a list of stations or features.
func ids(list any) []string {
	result := []string{}
	for _, item := range list.([]any) {
		result = append(result, item.(map[string]any)["id"].(string))
	}
	return result
}

func TestListStations(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA, zurich, lugano)
	f.load(storetest.Day2, bernA, bernB, zurich)

	response, body := f.get("/v1/stations")
	wantEqual(t, "status", response.StatusCode, http.StatusOK)
	wantEqual(t, "content type", response.Header.Get("Content-Type"), "application/geo+json")
	var collection map[string]any
	if err := json.Unmarshal(body, &collection); err != nil {
		t.Fatal(err)
	}
	wantEqual(t, "type", collection["type"], any("FeatureCollection"))
	wantEqual(t, "attribution", collection["attribution"], any(api.Attribution))
	wantEqual(t, "snapshot", collection["snapshot"], any(map[string]any{
		"publishedAt": "2026-11-24T22:01:55.936174Z",
		"sha256":      strings.Repeat("0", 60) + "1124",
		"sites":       float64(3),
	}))
	// Ordered by label: the site that left the file is not there.
	wantEqual(t, "ids", ids(collection["features"]), []string{
		"2600680_1199620_Salt BE_0001A", "2683328_1247916_Sunrise ZH601-1", "2600640_1199640_Swisscom BMAO",
	})
	wantEqual(t, "a feature", collection["features"].([]any)[2], any(map[string]any{
		"type": "Feature",
		"id":   "2600640_1199640_Swisscom BMAO",
		"geometry": map[string]any{
			"type": "Point", "coordinates": []any{7.4470391, 46.9478443},
		},
		"properties": map[string]any{
			"name": "Swisscom BMAO", "operator": "swisscom", "kind": "outdoor",
			"powerClass": "medium", "technologies": []any{"4G", "5G"},
		},
	}))
}

func TestCaching(t *testing.T) {
	f := newAPI(t)
	// Enough sites for the response to be worth compressing.
	many := []ofcom.MobileStation{}
	for number := range 20 {
		many = append(many, storetest.Station("Swisscom B"+strconv.Itoa(number), 2600000+float64(number), 1200000))
	}
	f.load(storetest.Day1, many...)

	response, body := f.get("/v1/stations")
	etag := response.Header.Get("ETag")
	if !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("got ETag %q, want a weak one", etag)
	}
	wantEqual(t, "Cache-Control", response.Header.Get("Cache-Control"), "public, max-age=300")
	wantEqual(t, "CORS", response.Header.Get("Access-Control-Allow-Origin"), "*")

	// A cache that holds the response revalidates it for nothing.
	again, empty := f.get("/v1/stations", "If-None-Match", `"other", `+etag)
	wantEqual(t, "status", again.StatusCode, http.StatusNotModified)
	wantEqual(t, "body", len(empty), 0)
	wantEqual(t, "ETag", again.Header.Get("ETag"), etag)
	// The strong form of the same tag matches too.
	strong, _ := f.get("/v1/stations", "If-None-Match", strings.TrimPrefix(etag, "W/"))
	wantEqual(t, "status with the strong form", strong.StatusCode, http.StatusNotModified)

	// Compressed for the clients that accept it, with the same tag.
	compressed, gzipped := f.get("/v1/stations", "Accept-Encoding", "br;q=1, gzip;q=0.5")
	wantEqual(t, "Content-Encoding", compressed.Header.Get("Content-Encoding"), "gzip")
	wantEqual(t, "ETag of the compressed body", compressed.Header.Get("ETag"), etag)
	reader, err := gzip.NewReader(bytes.NewReader(gzipped))
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decompressed, body) {
		t.Error("the compressed body is not the response")
	}
	refused, _ := f.get("/v1/stations", "Accept-Encoding", "gzip;q=0")
	wantEqual(t, "Content-Encoding when refused", refused.Header.Get("Content-Encoding"), "")

	// The preflight of a browser that sends If-None-Match itself.
	preflight, err := http.NewRequest(http.MethodOptions, f.server.URL+"/v1/stations", nil)
	if err != nil {
		t.Fatal(err)
	}
	preflight.Header.Set("Origin", "https://example.org")
	preflight.Header.Set("Access-Control-Request-Method", "GET")
	preflight.Header.Set("Access-Control-Request-Headers", "if-none-match")
	allowed, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	allowed.Body.Close()
	wantEqual(t, "preflight", []string{allowed.Status, allowed.Header.Get("Access-Control-Allow-Origin"),
		allowed.Header.Get("Access-Control-Allow-Headers")},
		[]string{"204 No Content", "*", "If-None-Match"})

	// gzip only when it is named, with a quality above zero.
	for header, want := range map[string]string{
		"gzip":              "gzip",
		"GZIP; q=0.1":       "gzip",
		"*":                 "",
		"*, gzip;q=0":       "",
		"*;q=0, gzip;q=1":   "gzip",
		"gzip;\tq=0":        "",
		"identity, deflate": "",
	} {
		response, _ := f.get("/v1/stations", "Accept-Encoding", header)
		wantEqual(t, "Accept-Encoding "+header, response.Header.Get("Content-Encoding"), want)
	}

	// A new snapshot is a new response.
	f.load(storetest.Day2, append(many, zurich)...)
	changed, _ := f.get("/v1/stations", "If-None-Match", etag)
	wantEqual(t, "status after an import", changed.StatusCode, http.StatusOK)
	if changed.Header.Get("ETag") == etag {
		t.Error("the ETag did not change with the data")
	}

	// Errors are not kept: what is missing may come with the next import.
	missing, _ := f.get("/v1/stations/1_2_Nobody")
	wantEqual(t, "status of a missing station", missing.StatusCode, http.StatusNotFound)
	wantEqual(t, "Cache-Control of an error", missing.Header.Get("Cache-Control"), "no-store")
	wantEqual(t, "ETag of an error", missing.Header.Get("ETag"), "")
}

func TestSearchAroundAPoint(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA, bernB, zurich, lugano)

	// Bern's Zytglogge, a few dozen metres from both Bern sites.
	found := f.getJSON("/v1/stations/search?lat=46.948&lon=7.4474&radius=300", http.StatusOK)
	wantEqual(t, "ids", ids(found["stations"]), []string{
		"2600640_1199640_Swisscom BMAO", "2600680_1199620_Salt BE_0001A",
	})
	wantEqual(t, "total", found["total"], any(float64(2)))
	first := found["stations"].([]any)[0].(map[string]any)
	wantEqual(t, "distance", first["distance"], any(32.5))
	wantEqual(t, "a station", first, map[string]any{
		"id": "2600640_1199640_Swisscom BMAO", "name": "Swisscom BMAO", "operator": "swisscom",
		"code": "BMAO", "lv95": map[string]any{"east": 2600640.0, "north": 1199640.0},
		"position":  map[string]any{"lat": 46.9478443, "lon": 7.4470391},
		"firstSeen": "2026-11-23T22:01:35.611529Z",
		"kind":      "outdoor", "powerClass": "medium", "technologies": []any{"4G", "5G"},
		"adaptive": true, "permit": map[string]any{"type": "site_data_sheet", "date": "2021-06-03"},
		"installationLimitValue": 5.0, "distance": 32.5,
	})
	second := found["stations"].([]any)[1].(map[string]any)
	wantEqual(t, "what the file left blank", []any{second["adaptive"], second["permit"],
		second["installationLimitValue"]}, []any{nil, map[string]any{"type": "exempt"}, nil})

	// The default radius, a kilometre, reaches neither Zurich nor Lugano.
	found = f.getJSON("/v1/stations/search?lat=46.948&lon=7.4474", http.StatusOK)
	wantEqual(t, "with the default radius", found["total"], any(float64(2)))

	found = f.getJSON("/v1/stations/search?lat=46.948&lon=7.4474&technology=2G,3G", http.StatusOK)
	wantEqual(t, "2G or 3G", ids(found["stations"]), []string{"2600680_1199620_Salt BE_0001A"})
}

func TestSearchInABox(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA, bernB, zurich, lugano)
	// All of Switzerland.
	const country = "/v1/stations/search?bbox=5.9,45.8,10.5,47.9"

	found := f.getJSON(country, http.StatusOK)
	wantEqual(t, "everything", ids(found["stations"]), []string{
		"2600680_1199620_Salt BE_0001A", "2717696_1102456_Salt TI_3787A",
		"2683328_1247916_Sunrise ZH601-1", "2600640_1199640_Swisscom BMAO",
	})
	if _, ok := found["stations"].([]any)[0].(map[string]any)["distance"]; ok {
		t.Error("a station found in a box has a distance")
	}

	for query, want := range map[string][]string{
		"&operator=salt":                 {"2600680_1199620_Salt BE_0001A", "2717696_1102456_Salt TI_3787A"},
		"&operator=salt&powerClass=high": {"2717696_1102456_Salt TI_3787A"},
		"&kind=indoor,tunnel":            {"2600680_1199620_Salt BE_0001A"},
		"&operator=sbb":                  {},
	} {
		found := f.getJSON(country+query, http.StatusOK)
		wantEqual(t, query, ids(found["stations"]), want)
	}

	// Bern only.
	found = f.getJSON("/v1/stations/search?bbox=7.4,46.9,7.5,47", http.StatusOK)
	wantEqual(t, "Bern", ids(found["stations"]), []string{
		"2600680_1199620_Salt BE_0001A", "2600640_1199640_Swisscom BMAO",
	})

	// Edges are included, exactly: a rectangle reduced to the point of a
	// site finds it, and one that stops just short of it does not.
	published, err := f.database.Published(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exact := published.Stations[0]
	lon, lat := exact.Position.Longitude, exact.Position.Latitude
	format := func(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }
	point := fmt.Sprintf("/v1/stations/search?bbox=%s,%s,%s,%s", format(lon), format(lat), format(lon), format(lat))
	wantEqual(t, "a point", ids(f.getJSON(point, http.StatusOK)["stations"]), []string{api.ID(exact.Site)})
	short := fmt.Sprintf("/v1/stations/search?bbox=%s,%s,%s,%s",
		format(lon-1), format(lat), format(math.Nextafter(lon, 0)), format(lat))
	wantEqual(t, "just short", ids(f.getJSON(short, http.StatusOK)["stations"]), []string{})

	// Pages, and a page past the end that still says how many there are.
	page := f.getJSON(country+"&limit=1&offset=2", http.StatusOK)
	wantEqual(t, "page", ids(page["stations"]), []string{"2683328_1247916_Sunrise ZH601-1"})
	wantEqual(t, "page counts", []any{page["total"], page["limit"], page["offset"]},
		[]any{4.0, 1.0, 2.0})
	past := f.getJSON(country+"&offset=10", http.StatusOK)
	wantEqual(t, "past the end", []any{ids(past["stations"]), past["total"]}, []any{[]string{}, 4.0})
}

func TestSearchRefusesWhatItCannotAnswer(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA)

	for _, query := range []string{
		"",
		"bbox=7,46,8",
		"bbox=7,46,8,47,9",
		"bbox=8,46,7,47",
		"bbox=7,47,8,46",
		"bbox=7,95,8,96",
		"bbox=NaN,46,8,47",
		"bbox=7,46,8,47&lat=46.9&lon=7.4",
		"lat=46.9",
		"lon=7.4",
		"lat=91&lon=7.4",
		"lat=NaN&lon=7.4",
		"lat=46.9&lon=7.4&radius=0",
		"lat=46.9&lon=7.4&radius=50001",
		"lat=46.9&lon=7.4&limit=0",
		"lat=46.9&lon=7.4&limit=1001",
		"lat=46.9&lon=7.4&offset=-1",
		"lat=46.9&lon=7.4&operator=orange",
		"lat=46.9&lon=7.4&technology=6G",
		"lat=46.9&lon=7.4&kind=outdoor,attic",
		"lat=46.9&lon=7.4&powerClass=huge",
		"lat=46.9&lon=7.4&operators=salt",
		"lat=north&lon=7.4",
		// Empty, which would count as absent: here, as zero.
		"lat=&lon=",
		"lat=46.9&lon=",
		"bbox=&lat=46.9&lon=7.4",
		"lat=46.9&lon=7.4&operator=",
		// Twice, of which only the first would count.
		"lat=46.9&lon=7.4&operator=salt&operator=sunrise",
		// Not a number JSON can carry back in the error.
		"lat=Inf&lon=7.4",
		"lat=46.9&lon=-Inf",
		"lat=46.9&lon=7.4&radius=Inf",
		"bbox=NaN,46,8",
		"bbox=7,46,8,Inf",
		// A query string that does not decode.
		"lat=46.9&lon=7.4&%zz=1",
	} {
		response, body := f.get("/v1/stations/search?" + query)
		var problem map[string]any
		if response.StatusCode != http.StatusUnprocessableEntity ||
			response.Header.Get("Content-Type") != "application/problem+json" ||
			json.Unmarshal(body, &problem) != nil || problem["status"] != 422.0 {
			t.Errorf("%q: got status %d (%s): %s", query, response.StatusCode,
				response.Header.Get("Content-Type"), body)
		}
	}
	// The other routes check their parameters too.
	for _, path := range []string{
		"/v1/stations?bbox=1,2,3,4", "/v1/stations/counts?x=1", "/v1/snapshots?limit=",
	} {
		response, body := f.get(path)
		if response.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: got status %d: %s", path, response.StatusCode, body)
		}
	}
}

// slowData answers nothing until the request gives up.
type slowData struct{ api.Data }

func (slowData) Counts(ctx context.Context) (store.Counts, error) {
	<-ctx.Done()
	return store.Counts{}, ctx.Err()
}

// A request that takes too long is stopped, and answered like any error.
func TestTimeout(t *testing.T) {
	server := httptest.NewServer(api.New(slowData{}, slog.New(slog.DiscardHandler), 50*time.Millisecond))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/stations/counts")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	wantEqual(t, "status", response.StatusCode, http.StatusServiceUnavailable)
	wantEqual(t, "Content-Type", response.Header.Get("Content-Type"), "application/problem+json")
	wantEqual(t, "Cache-Control", response.Header.Get("Cache-Control"), "no-store")
	wantEqual(t, "CORS", response.Header.Get("Access-Control-Allow-Origin"), "*")
	if !strings.Contains(string(body), "the request took too long") {
		t.Errorf("got %s", body)
	}
}

func TestStationHistory(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA, zurich, odd)
	f.load(storetest.Day2, storetest.With(bernA, func(s *ofcom.MobileStation) {
		s.Technologies = []ofcom.Technology{ofcom.Technology5G}
	}), odd)
	f.load(storetest.Day3, storetest.With(bernA, func(s *ofcom.MobileStation) {
		s.Technologies = []ofcom.Technology{ofcom.Technology5G}
	}), zurich)

	history := f.getJSON("/v1/stations/"+url.PathEscape("2600640_1199640_Swisscom BMAO"), http.StatusOK)
	wantEqual(t, "station", []any{history["id"], history["name"], history["published"], history["total"]},
		[]any{"2600640_1199640_Swisscom BMAO", "Swisscom BMAO", true, 2.0})
	wantEqual(t, "attribution", history["attribution"], any(api.Attribution))
	periods := history["periods"].([]any)
	wantEqual(t, "latest period", periods[0].(map[string]any)["from"], any("2026-11-24T22:01:55.936174Z"))
	wantEqual(t, "latest period's end", periods[0].(map[string]any)["to"], nil)
	wantEqual(t, "latest technologies", periods[0].(map[string]any)["technologies"], any([]any{"5G"}))
	wantEqual(t, "first period", []any{periods[1].(map[string]any)["from"], periods[1].(map[string]any)["to"]},
		[]any{"2026-11-23T22:01:35.611529Z", "2026-11-24T22:01:55.936174Z"})

	// Zurich left the file for a day and came back.
	zurichHistory := f.getJSON("/v1/stations/2683328_1247916_Sunrise%20ZH601-1?limit=1", http.StatusOK)
	wantEqual(t, "Zurich", []any{zurichHistory["total"], len(zurichHistory["periods"].([]any)),
		zurichHistory["periods"].([]any)[0].(map[string]any)["from"]},
		[]any{2.0, 1, "2026-11-25T22:02:07Z"})
	past := f.getJSON("/v1/stations/2683328_1247916_Sunrise%20ZH601-1?offset=5", http.StatusOK)
	wantEqual(t, "past the end", []any{past["total"], len(past["periods"].([]any))}, []any{2.0, 0})

	// A site that left the file is still known, and its id reads back
	// whatever its label holds.
	id := ids(f.getJSON("/v1/stations/search?bbox=5.9,45.8,10.5,47.9&operator=other", http.StatusOK)["stations"])
	wantEqual(t, "the odd site is gone", id, []string{})
	gone := f.getJSON("/v1/stations/"+url.PathEscape("2600000_1200000_Other A/B %1?#"), http.StatusOK)
	wantEqual(t, "gone", []any{gone["id"], gone["name"], gone["published"]},
		[]any{"2600000_1200000_Other A/B %1?#", "Other A/B %1?#", false})

	for path, status := range map[string]int{
		"/v1/stations/2600640_1199641_Swisscom%20BMAO":             http.StatusNotFound,
		"/v1/stations/2600640_1199640_Swisscom%20BMA":              http.StatusNotFound,
		"/v1/stations/Swisscom%20BMAO":                             http.StatusUnprocessableEntity,
		"/v1/stations/2600640_Swisscom%20BMAO":                     http.StatusUnprocessableEntity,
		"/v1/stations/2600640_1199640_":                            http.StatusUnprocessableEntity,
		"/v1/stations/2600640_1199640_" + strings.Repeat("x", 200): http.StatusUnprocessableEntity,
		"/v1/stations/2600640_1199640_Swisscom%20BMAO?limit=0":     http.StatusUnprocessableEntity,
	} {
		response, body := f.get(path)
		if response.StatusCode != status {
			t.Errorf("%s: got status %d, want %d: %s", path, response.StatusCode, status, body)
		}
	}
}

func TestCounts(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA, bernB, zurich, lugano)

	counts := f.getJSON("/v1/stations/counts", http.StatusOK)
	wantEqual(t, "stations", counts["stations"], any(4.0))
	byValue := func(list any) map[string]float64 {
		result := map[string]float64{}
		for _, item := range list.([]any) {
			result[item.(map[string]any)["value"].(string)] = item.(map[string]any)["stations"].(float64)
		}
		return result
	}
	wantEqual(t, "operators", byValue(counts["operators"]), map[string]float64{
		"swisscom": 1, "sunrise": 1, "salt": 2, "sbb": 0, "other": 0,
	})
	wantEqual(t, "kinds", byValue(counts["kinds"]), map[string]float64{
		"outdoor": 3, "outdoor_low_power": 0, "indoor": 1, "tunnel": 0, "temporary": 0, "unknown": 0,
	})
	wantEqual(t, "power classes", byValue(counts["powerClasses"]), map[string]float64{
		"very_low": 0, "low": 0, "medium": 3, "high": 1, "unknown": 0,
	})
	// A site counts once for each of its technologies.
	wantEqual(t, "technologies", byValue(counts["technologies"]), map[string]float64{
		"2G": 1, "3G": 0, "4G": 3, "5G": 3,
	})
	// In the order of the vocabulary.
	wantEqual(t, "order", counts["operators"].([]any)[0].(map[string]any)["value"], any("swisscom"))
}

func TestSnapshots(t *testing.T) {
	f := newAPI(t)
	empty := f.getJSON("/v1/snapshots", http.StatusOK)
	wantEqual(t, "before the first import", []any{empty["snapshots"], empty["total"]}, []any{[]any{}, 0.0})

	f.load(storetest.Day1, bernA)
	f.load(storetest.Day2, bernA, zurich)
	list := f.getJSON("/v1/snapshots", http.StatusOK)
	wantEqual(t, "snapshots", list["snapshots"], any([]any{
		map[string]any{"publishedAt": "2026-11-24T22:01:55.936174Z", "sha256": strings.Repeat("0", 60) + "1124", "sites": 2.0},
		map[string]any{"publishedAt": "2026-11-23T22:01:35.611529Z", "sha256": strings.Repeat("0", 60) + "1123", "sites": 1.0},
	}))
	page := f.getJSON("/v1/snapshots?limit=1&offset=1", http.StatusOK)
	wantEqual(t, "second page", []any{len(page["snapshots"].([]any)), page["total"]}, []any{1, 2.0})
	past := f.getJSON("/v1/snapshots?offset=2", http.StatusOK)
	wantEqual(t, "past the end", []any{past["snapshots"], past["total"]}, []any{[]any{}, 2.0})
}

// Before the first import there is nothing to serve, and the API says so.
func TestBeforeTheFirstImport(t *testing.T) {
	f := newAPI(t)
	for _, path := range []string{
		"/v1/stations", "/v1/stations/counts", "/v1/stations/search?lat=46.9&lon=7.4",
	} {
		response, body := f.get(path)
		if response.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s: got status %d: %s", path, response.StatusCode, body)
		}
	}
	response, _ := f.get("/v1/stations/2600640_1199640_Swisscom%20BMAO")
	wantEqual(t, "a station", response.StatusCode, http.StatusNotFound)
}

// The API serves its own contract, and nothing it does not describe.
func TestContract(t *testing.T) {
	f := newAPI(t)
	f.load(storetest.Day1, bernA)
	response, body := f.get("/v1/openapi.json")
	wantEqual(t, "status", response.StatusCode, http.StatusOK)
	wantEqual(t, "Content-Type", response.Header.Get("Content-Type"), "application/openapi+json")
	contract, err := api.New(nil, slog.New(slog.DiscardHandler), 0).OpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, contract) {
		t.Error("the served contract is not the one `swisscoverage openapi` prints")
	}

	for _, path := range []string{
		"/", "/docs", "/schemas/Snapshot.json", "/v1/", "/v1/nothing",
		"/v1/openapi.yaml", "/v1/openapi-3.0.json",
	} {
		response, _ := f.get(path)
		wantEqual(t, path, response.StatusCode, http.StatusNotFound)
	}
	response, _ = f.get("/v1/stations/counts", "Accept", "application/cbor")
	wantEqual(t, "another format", response.Header.Get("Content-Type"), "application/json")
}
