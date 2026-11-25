// Package api is the public HTTP API: read-only, and the only way to the data
// for the website, the MCP server and third parties. docs/architecture.md
// describes it.
//
// The routes and the types they return are the contract. Huma generates the
// OpenAPI document from them, which `swisscoverage openapi` prints and which
// is committed as openapi.json: a change here shows in that file's diff.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
)

// Attribution is the credit that OFCOM's terms of use require. Every response
// that carries data carries it too.
const Attribution = "Source: Federal Office of Communications OFCOM"

// Version is the version of the contract. It changes when the contract does,
// which may only add to it: what exists keeps its meaning.
const Version = "1.0.0"

func init() {
	// Every list the API returns is a list, empty rather than null. Huma
	// reads this setting when it describes a type.
	huma.DefaultArrayNullable = false
}

// Data is what the API reads. *store.Store is the implementation.
type Data interface {
	Published(ctx context.Context) (store.Published, error)
	Search(ctx context.Context, search store.Search) (store.Found, error)
	History(ctx context.Context, site store.Site, page store.Page) (store.History, bool, error)
	Counts(ctx context.Context) (store.Counts, error)
	SnapshotPage(ctx context.Context, page store.Page) (store.SnapshotPage, error)
}

// API is the HTTP handler of the public API.
type API struct {
	huma    huma.API
	handler http.Handler
}

// New returns the API, reading from data. Errors the client cannot act on
// are logged to logger and answered without their details.
func New(data Data, logger *slog.Logger) *API {
	config := huma.DefaultConfig("swisscoverage", Version)
	// No $schema member in the responses, nor documentation pages that load
	// scripts from elsewhere: the API serves data and its contract only.
	config.CreateHooks = nil
	config.DocsPath = ""
	config.SchemasPath = ""
	config.OpenAPIPath = "/v1/openapi"
	config.RejectUnknownQueryParameters = true
	config.Info.Description = "The mobile network sites of Switzerland that OFCOM publishes, " +
		"as they are today and as they were since the archive began. Read-only, and free to use. " +
		Attribution + ", whose terms ask that the source be given: " +
		"https://opendata.swiss/en/terms-of-use/#terms_by. " +
		"Every response may be cached: the data changes once a day."
	config.Info.License = &huma.License{
		Name:       "AGPL-3.0-only, for the code; the data keeps the terms of its source",
		Identifier: "AGPL-3.0-only",
	}

	mux := http.NewServeMux()
	humaAPI := humago.New(mux, config)
	h := handlers{data: data, logger: logger}

	huma.Register(humaAPI, huma.Operation{
		OperationID: "list-stations",
		Method:      http.MethodGet,
		Path:        "/v1/stations",
		Summary:     "Every station published today",
		Description: "Every site of the latest snapshot, as one GeoJSON FeatureCollection, " +
			"which is what the map loads. It is not paginated: the dataset bounds it, at about " +
			"23,000 sites. For a station's details, read it by its id.",
		Tags:   []string{"Stations"},
		Errors: []int{http.StatusServiceUnavailable},
	}, h.listStations)

	huma.Register(humaAPI, huma.Operation{
		OperationID: "search-stations",
		Method:      http.MethodGet,
		Path:        "/v1/stations/search",
		Summary:     "Stations in a rectangle or around a point",
		Description: "The sites of the latest snapshot inside a rectangle, given by `bbox`, or " +
			"within `radius` metres of a point, given by `lat` and `lon`. Exactly one of the two " +
			"is required. Each filter accepts several values, separated by commas, and keeps the " +
			"sites that have one of them; filters combine with each other. Results are ordered by " +
			"label and position, or nearest first around a point.",
		Tags:   []string{"Stations"},
		Errors: []int{http.StatusUnprocessableEntity, http.StatusServiceUnavailable},
	}, h.searchStations)

	huma.Register(humaAPI, huma.Operation{
		OperationID: "count-stations",
		Method:      http.MethodGet,
		Path:        "/v1/stations/counts",
		Summary:     "Counts of stations",
		Description: "How many sites the latest snapshot publishes, in all and for each operator, " +
			"kind, power class and technology. Every value is listed, with zero when no site has " +
			"it. A site counts once for each of its technologies.",
		Tags:   []string{"Stations"},
		Errors: []int{http.StatusServiceUnavailable},
	}, h.countStations)

	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-station",
		Method:      http.MethodGet,
		Path:        "/v1/stations/{id}",
		Summary:     "One station and its history",
		Description: "A site, published today or in the past, with what each snapshot published " +
			"about it: one period for each stretch of time during which nothing changed, the latest " +
			"first.",
		Tags:   []string{"Stations"},
		Errors: []int{http.StatusNotFound, http.StatusUnprocessableEntity},
	}, h.getStation)

	huma.Register(humaAPI, huma.Operation{
		OperationID: "list-snapshots",
		Method:      http.MethodGet,
		Path:        "/v1/snapshots",
		Summary:     "The snapshots",
		Description: "The files published by OFCOM that the data comes from, one per day since the " +
			"archive began, the newest first.",
		Tags:   []string{"Snapshots"},
		Errors: []int{http.StatusUnprocessableEntity},
	}, h.listSnapshots)

	describeCaching(humaAPI.OpenAPI())
	return &API{huma: humaAPI, handler: withCaching(mux)}
}

// ServeHTTP answers a request.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.handler.ServeHTTP(w, r)
}

// OpenAPI returns the contract, as the JSON that openapi.json holds.
func (a *API) OpenAPI() ([]byte, error) {
	contract, err := json.MarshalIndent(a.huma.OpenAPI(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(contract, '\n'), nil
}

type handlers struct {
	data   Data
	logger *slog.Logger
}

// internal logs an error the client cannot act on and answers without it.
func (h handlers) internal(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		h.logger.Error("answering a request", "error", err)
	}
	return huma.Error500InternalServerError("the request could not be answered")
}

// noSnapshot is the answer before the first import.
func noSnapshot() error {
	return huma.Error503ServiceUnavailable("no snapshot is loaded yet")
}

// The vocabulary of the data. Each type lists its values in the contract,
// from the parser's own list.

// Operator is who runs a station.
type Operator ofcom.Operator

// Kind is the type of site.
type Kind ofcom.StationKind

// PowerClass is the class of the highest power radiated in a 90° sector.
type PowerClass ofcom.PowerClass

// Technology is a mobile network generation.
type Technology ofcom.Technology

// PermitType says what OFCOM publishes about a site's permit.
type PermitType ofcom.PermitType

func enumSchema[T ~string](description string, values []T) *huma.Schema {
	schema := &huma.Schema{Type: huma.TypeString, Description: description}
	for _, value := range values {
		schema.Enum = append(schema.Enum, string(value))
	}
	return schema
}

func (Operator) Schema(huma.Registry) *huma.Schema {
	return enumSchema("Who runs the station. `other` groups the stations published without a "+
		"known operator, such as foreign stations near the border.", ofcom.Operators())
}

func (Kind) Schema(huma.Registry) *huma.Schema {
	return enumSchema("The type of site: outdoor above 6 W ERP, outdoor up to 6 W ERP, indoor up "+
		"to 6 W ERP, in a tunnel, temporary, or not published.", ofcom.StationKinds())
}

func (PowerClass) Schema(huma.Registry) *huma.Schema {
	return enumSchema("The class of ERP90, the highest cumulated power radiated in a 90° sector, "+
		"as OFCOM publishes it.", ofcom.PowerClasses())
}

func (Technology) Schema(huma.Registry) *huma.Schema {
	return enumSchema("A mobile network generation.", ofcom.Technologies())
}

func (PermitType) Schema(huma.Registry) *huma.Schema {
	return enumSchema("`site_data_sheet`: a site data sheet was filed. `exempt`: the site is "+
		"exempt from the precautionary emission limit (NISV Annex 1 No. 64).",
		[]ofcom.PermitType{ofcom.PermitSiteDataSheet, ofcom.PermitExempt})
}

// The types of the responses.

// Snapshot is one file published by OFCOM.
type Snapshot struct {
	PublishedAt time.Time `json:"publishedAt" doc:"When OFCOM published the file. It identifies the snapshot."`
	SHA256      string    `json:"sha256" doc:"The SHA-256 of the file as published, in hexadecimal." pattern:"^[0-9a-f]{64}$"`
	Sites       int       `json:"sites" doc:"The number of sites in the file." minimum:"0"`
}

func snapshotOf(snapshot store.Snapshot) Snapshot {
	return Snapshot{PublishedAt: snapshot.PublishedAt, SHA256: snapshot.SHA256, Sites: snapshot.Sites}
}

// Position is a point in WGS84.
type Position struct {
	Latitude  float64 `json:"lat" doc:"Latitude in WGS84, in degrees." minimum:"-90" maximum:"90"`
	Longitude float64 `json:"lon" doc:"Longitude in WGS84, in degrees." minimum:"-180" maximum:"180"`
}

// LV95 is a point in the Swiss grid, as OFCOM publishes it.
type LV95 struct {
	East  float64 `json:"east" doc:"Easting in LV95 (EPSG:2056), in metres."`
	North float64 `json:"north" doc:"Northing in LV95 (EPSG:2056), in metres."`
}

// Permit is the permit information of a site. It is null where OFCOM
// publishes none.
type Permit struct {
	_    struct{}   `nullable:"true"`
	Type PermitType `json:"type"`
	Date string     `json:"date,omitempty" format:"date" doc:"The date of the site data sheet. Absent for an exemption, and when the published date is not a day that exists."`
}

// Station is what never changes about a site.
type Station struct {
	ID        string    `json:"id" doc:"The site's identifier: its LV95 position and its label, as east_north_label. OFCOM publishes no identifier, and a label alone is not unique."`
	Name      string    `json:"name" doc:"The station label exactly as published, such as \"Sunrise ZH601-1\"."`
	Operator  Operator  `json:"operator"`
	Code      string    `json:"code" doc:"The label without the operator's name, such as \"ZH601-1\"."`
	LV95      LV95      `json:"lv95" doc:"The position as published."`
	Position  Position  `json:"position" doc:"The same position in WGS84, computed with the rigorous transformation, to 7 decimals."`
	FirstSeen time.Time `json:"firstSeen" doc:"The publication time of the first snapshot that showed the site."`
}

// State is what a snapshot publishes about a site, apart from its identity.
type State struct {
	Kind                   Kind         `json:"kind"`
	PowerClass             PowerClass   `json:"powerClass"`
	Technologies           []Technology `json:"technologies" doc:"In generation order. Empty when the file lists none that is recognised."`
	Adaptive               *bool        `json:"adaptive" nullable:"true" doc:"Whether at least part of the site runs adaptive antennas. Null when the file says something that is not recognised."`
	Permit                 *Permit      `json:"permit" doc:"Null when OFCOM publishes no permit information."`
	InstallationLimitValue *float64     `json:"installationLimitValue" nullable:"true" doc:"The installation limit value, in V/m. Null when absent or left blank."`
}

// PublishedStation is a site as the latest snapshot shows it.
type PublishedStation struct {
	Station
	State
	Distance *float64 `json:"distance,omitempty" doc:"Around a point only: the distance from it, in metres, on the ellipsoid."`
}

// Period is what was published of a site from one snapshot to another.
type Period struct {
	From time.Time  `json:"from" doc:"The publication time of the snapshot that first showed these values."`
	To   *time.Time `json:"to" nullable:"true" doc:"The publication time of the first snapshot that no longer did. Null while they are still published."`
	State
}

func roundDegrees(value float64) float64 {
	return math.Round(value*1e7) / 1e7
}

// ID returns the identifier of a site: its position and its label. Both
// numbers are written in full, without exponent, so the identifier reads
// back into the same site.
func ID(site store.Site) string {
	return strconv.FormatFloat(site.LV95.East, 'f', -1, 64) + "_" +
		strconv.FormatFloat(site.LV95.North, 'f', -1, 64) + "_" + site.Name
}

var idPattern = regexp.MustCompile(`^(-?[0-9]+(?:\.[0-9]+)?)_(-?[0-9]+(?:\.[0-9]+)?)_(.+)$`)

// parseID reads an identifier written by ID.
func parseID(id string) (store.Site, bool) {
	match := idPattern.FindStringSubmatch(id)
	if match == nil {
		return store.Site{}, false
	}
	east, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return store.Site{}, false
	}
	north, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return store.Site{}, false
	}
	return store.Site{Name: match[3], LV95: ofcom.LV95{East: east, North: north}}, true
}

func stationOf(station store.Station) Station {
	return Station{
		ID:       ID(station.Site),
		Name:     station.Name,
		Operator: Operator(station.Operator),
		Code:     station.Code,
		LV95:     LV95{East: station.LV95.East, North: station.LV95.North},
		Position: Position{
			Latitude:  roundDegrees(station.Position.Latitude),
			Longitude: roundDegrees(station.Position.Longitude),
		},
		FirstSeen: station.FirstSeen,
	}
}

func stateOf(state store.State) State {
	result := State{
		Kind:                   Kind(state.Kind),
		PowerClass:             PowerClass(state.PowerClass),
		Technologies:           make([]Technology, 0, len(state.Technologies)),
		Adaptive:               state.Adaptive,
		InstallationLimitValue: state.InstallationLimitValue,
	}
	for _, technology := range state.Technologies {
		result.Technologies = append(result.Technologies, Technology(technology))
	}
	if state.Permit != nil {
		result.Permit = &Permit{Type: PermitType(state.Permit.Type), Date: state.Permit.Date}
	}
	return result
}

// 1. Every station, as GeoJSON.

// StationCollection is every station of the latest snapshot, as a GeoJSON
// FeatureCollection (RFC 7946) with two foreign members.
type StationCollection struct {
	Type        string           `json:"type" enum:"FeatureCollection"`
	Attribution string           `json:"attribution" doc:"The credit OFCOM's terms require."`
	Snapshot    Snapshot         `json:"snapshot" doc:"The snapshot these stations come from."`
	Features    []StationFeature `json:"features"`
}

// ContentType makes the response a GeoJSON document.
func (StationCollection) ContentType(string) string { return "application/geo+json" }

// StationFeature is one station, with what the map filters on.
type StationFeature struct {
	Type       string            `json:"type" enum:"Feature"`
	ID         string            `json:"id" doc:"The station's identifier, as in the station's own response."`
	Geometry   PointGeometry     `json:"geometry"`
	Properties StationProperties `json:"properties"`
}

// PointGeometry is a GeoJSON point: longitude, then latitude.
type PointGeometry struct {
	Type        string    `json:"type" enum:"Point"`
	Coordinates []float64 `json:"coordinates" minItems:"2" maxItems:"2" doc:"Longitude and latitude in WGS84, in degrees, to 7 decimals."`
}

// StationProperties is what the map shows and filters on. The rest is in
// the station's own response.
type StationProperties struct {
	Name         string       `json:"name" doc:"The station label exactly as published."`
	Operator     Operator     `json:"operator"`
	Kind         Kind         `json:"kind"`
	PowerClass   PowerClass   `json:"powerClass"`
	Technologies []Technology `json:"technologies" doc:"In generation order. Empty when the file lists none that is recognised."`
}

type listStationsOutput struct {
	Body StationCollection
}

func (h handlers) listStations(ctx context.Context, _ *struct{}) (*listStationsOutput, error) {
	published, err := h.data.Published(ctx)
	if err != nil {
		return nil, h.internal(ctx, err)
	}
	if published.Snapshot == nil {
		return nil, noSnapshot()
	}
	collection := StationCollection{
		Type:        "FeatureCollection",
		Attribution: Attribution,
		Snapshot:    snapshotOf(*published.Snapshot),
		Features:    make([]StationFeature, 0, len(published.Stations)),
	}
	for _, station := range published.Stations {
		state := stateOf(station.State)
		collection.Features = append(collection.Features, StationFeature{
			Type: "Feature",
			ID:   ID(station.Site),
			Geometry: PointGeometry{Type: "Point", Coordinates: []float64{
				roundDegrees(station.Position.Longitude), roundDegrees(station.Position.Latitude),
			}},
			Properties: StationProperties{
				Name:         station.Name,
				Operator:     Operator(station.Operator),
				Kind:         state.Kind,
				PowerClass:   state.PowerClass,
				Technologies: state.Technologies,
			},
		})
	}
	return &listStationsOutput{Body: collection}, nil
}

// 2. Stations in a rectangle or around a point.

type searchStationsInput struct {
	BBox       []float64    `query:"bbox" minItems:"4" maxItems:"4" doc:"A rectangle in WGS84, as west,south,east,north in degrees: the longitude of its west edge, the latitude of its south edge, and so on. Edges are included."`
	Latitude   float64      `query:"lat" minimum:"-90" maximum:"90" doc:"The latitude of the point to search around, in WGS84 degrees. Requires lon."`
	Longitude  float64      `query:"lon" minimum:"-180" maximum:"180" doc:"The longitude of the point to search around, in WGS84 degrees. Requires lat."`
	Radius     float64      `query:"radius" minimum:"1" maximum:"50000" default:"1000" doc:"Around a point: the distance to search within, in metres."`
	Operator   []Operator   `query:"operator" maxItems:"5" doc:"Keeps the sites run by one of these operators."`
	Kind       []Kind       `query:"kind" maxItems:"6" doc:"Keeps the sites of one of these kinds."`
	PowerClass []PowerClass `query:"powerClass" maxItems:"5" doc:"Keeps the sites of one of these power classes."`
	Technology []Technology `query:"technology" maxItems:"4" doc:"Keeps the sites that have one of these technologies."`
	Paging

	// Whether lat and lon were given: zero is a value like another.
	hasLatitude, hasLongitude bool
}

// Paging says which page of a list to return. Huma reads the parameters of
// an embedded struct only when its type is exported.
type Paging struct {
	Limit  int `query:"limit" minimum:"1" maximum:"1000" default:"100" doc:"The most items to return."`
	Offset int `query:"offset" minimum:"0" maximum:"1000000" default:"0" doc:"How many items to skip, for the next pages."`
}

func (p Paging) page() store.Page {
	return store.Page{Limit: p.Limit, Offset: p.Offset}
}

// Resolve checks what the tags on the fields cannot say.
func (input *searchStationsInput) Resolve(ctx huma.Context) []error {
	var errs []error
	url := ctx.URL()
	query := url.Query()
	input.hasLatitude, input.hasLongitude = query.Has("lat"), query.Has("lon")
	center := input.hasLatitude || input.hasLongitude
	switch {
	case input.BBox == nil && !center:
		errs = append(errs, &huma.ErrorDetail{
			Message:  "give either bbox, or lat and lon",
			Location: "query.bbox",
		})
	case input.BBox != nil && center:
		errs = append(errs, &huma.ErrorDetail{
			Message:  "give either bbox, or lat and lon, not both",
			Location: "query.bbox",
		})
	case center && !input.hasLatitude:
		errs = append(errs, &huma.ErrorDetail{Message: "lon requires lat", Location: "query.lat"})
	case center && !input.hasLongitude:
		errs = append(errs, &huma.ErrorDetail{Message: "lat requires lon", Location: "query.lon"})
	}
	// NaN passes every minimum and maximum.
	for _, number := range []struct {
		name  string
		value float64
	}{{"lat", input.Latitude}, {"lon", input.Longitude}, {"radius", input.Radius}} {
		if math.IsNaN(number.value) {
			errs = append(errs, &huma.ErrorDetail{Message: "expected a number", Location: "query." + number.name})
		}
	}
	if len(input.BBox) == 4 {
		west, south, east, north := input.BBox[0], input.BBox[1], input.BBox[2], input.BBox[3]
		for index, value := range input.BBox {
			limit := 180.0
			if index%2 == 1 {
				limit = 90
			}
			if math.IsNaN(value) || value < -limit || value > limit {
				errs = append(errs, &huma.ErrorDetail{
					Message:  fmt.Sprintf("a longitude is within ±180 and a latitude within ±90, not %v", value),
					Location: fmt.Sprintf("query.bbox[%d]", index),
				})
			}
		}
		if west > east || south > north {
			errs = append(errs, &huma.ErrorDetail{
				Message:  "bbox is west,south,east,north: west may not exceed east, nor south north",
				Location: "query.bbox",
			})
		}
	}
	return errs
}

// SearchResult is a page of the stations a search matched.
type SearchResult struct {
	Attribution string             `json:"attribution" doc:"The credit OFCOM's terms require."`
	Snapshot    Snapshot           `json:"snapshot" doc:"The snapshot these stations come from."`
	Stations    []PublishedStation `json:"stations"`
	Total       int                `json:"total" doc:"How many stations the search matched, on every page."`
	Limit       int                `json:"limit"`
	Offset      int                `json:"offset"`
}

type searchStationsOutput struct {
	Body SearchResult
}

func (h handlers) searchStations(ctx context.Context, input *searchStationsInput) (*searchStationsOutput, error) {
	search := store.Search{Page: input.page()}
	if input.BBox != nil {
		search.Box = &store.Box{
			West: input.BBox[0], South: input.BBox[1], East: input.BBox[2], North: input.BBox[3],
		}
	} else {
		search.Around = &store.Around{
			Center: store.WGS84{Latitude: input.Latitude, Longitude: input.Longitude},
			Radius: input.Radius,
		}
	}
	for _, value := range input.Operator {
		search.Operators = append(search.Operators, ofcom.Operator(value))
	}
	for _, value := range input.Kind {
		search.Kinds = append(search.Kinds, ofcom.StationKind(value))
	}
	for _, value := range input.PowerClass {
		search.PowerClasses = append(search.PowerClasses, ofcom.PowerClass(value))
	}
	for _, value := range input.Technology {
		search.Technologies = append(search.Technologies, ofcom.Technology(value))
	}

	found, err := h.data.Search(ctx, search)
	if err != nil {
		return nil, h.internal(ctx, err)
	}
	if found.Snapshot == nil {
		return nil, noSnapshot()
	}
	result := SearchResult{
		Attribution: Attribution,
		Snapshot:    snapshotOf(*found.Snapshot),
		Stations:    make([]PublishedStation, 0, len(found.Stations)),
		Total:       found.Total,
		Limit:       input.Limit,
		Offset:      input.Offset,
	}
	for _, station := range found.Stations {
		item := PublishedStation{Station: stationOf(station.Station), State: stateOf(station.State)}
		if search.Around != nil {
			distance := math.Round(station.Distance*10) / 10
			item.Distance = &distance
		}
		result.Stations = append(result.Stations, item)
	}
	return &searchStationsOutput{Body: result}, nil
}

// 3. One station and its history.

type getStationInput struct {
	ID string `path:"id" maxLength:"200" pattern:"^-?[0-9]+(\\.[0-9]+)?_-?[0-9]+(\\.[0-9]+)?_.+$" doc:"The station's identifier, as east_north_label, such as 2683000_1247000_Sunrise ZH601-1, percent-encoded in the path."`
	Paging
}

// StationHistory is a site and a page of its periods.
type StationHistory struct {
	Attribution string `json:"attribution" doc:"The credit OFCOM's terms require."`
	Station
	Published bool     `json:"published" doc:"Whether the latest snapshot shows the site."`
	Periods   []Period `json:"periods" doc:"What was published of the site, period by period, the latest first."`
	Total     int      `json:"total" doc:"How many periods the site has, on every page."`
	Limit     int      `json:"limit"`
	Offset    int      `json:"offset"`
}

type getStationOutput struct {
	Body StationHistory
}

func (h handlers) getStation(ctx context.Context, input *getStationInput) (*getStationOutput, error) {
	site, ok := parseID(input.ID)
	if !ok {
		return nil, huma.Error404NotFound("no station has this identifier")
	}
	history, found, err := h.data.History(ctx, site, input.page())
	if err != nil {
		return nil, h.internal(ctx, err)
	}
	if !found {
		return nil, huma.Error404NotFound("no station has this identifier")
	}
	result := StationHistory{
		Attribution: Attribution,
		Station:     stationOf(history.Station),
		Published:   history.Published,
		Periods:     make([]Period, 0, len(history.Periods)),
		Total:       history.Total,
		Limit:       input.Limit,
		Offset:      input.Offset,
	}
	for _, period := range history.Periods {
		result.Periods = append(result.Periods, Period{
			From: period.From, To: period.To, State: stateOf(period.State),
		})
	}
	return &getStationOutput{Body: result}, nil
}

// 4. Counts.

// OperatorCount is how many stations one operator runs.
type OperatorCount struct {
	Value    Operator `json:"value"`
	Stations int      `json:"stations" minimum:"0"`
}

// KindCount is how many stations are of one kind.
type KindCount struct {
	Value    Kind `json:"value"`
	Stations int  `json:"stations" minimum:"0"`
}

// PowerClassCount is how many stations are in one power class.
type PowerClassCount struct {
	Value    PowerClass `json:"value"`
	Stations int        `json:"stations" minimum:"0"`
}

// TechnologyCount is how many stations have one technology.
type TechnologyCount struct {
	Value    Technology `json:"value"`
	Stations int        `json:"stations" minimum:"0"`
}

// StationCounts is how many stations the latest snapshot publishes.
type StationCounts struct {
	Attribution  string            `json:"attribution" doc:"The credit OFCOM's terms require."`
	Snapshot     Snapshot          `json:"snapshot" doc:"The snapshot these counts come from."`
	Stations     int               `json:"stations" minimum:"0" doc:"How many sites the snapshot publishes."`
	Operators    []OperatorCount   `json:"operators"`
	Kinds        []KindCount       `json:"kinds"`
	PowerClasses []PowerClassCount `json:"powerClasses"`
	Technologies []TechnologyCount `json:"technologies"`
}

type countStationsOutput struct {
	Body StationCounts
}

func (h handlers) countStations(ctx context.Context, _ *struct{}) (*countStationsOutput, error) {
	counts, err := h.data.Counts(ctx)
	if err != nil {
		return nil, h.internal(ctx, err)
	}
	if counts.Snapshot == nil {
		return nil, noSnapshot()
	}
	result := StationCounts{
		Attribution: Attribution,
		Snapshot:    snapshotOf(*counts.Snapshot),
		Stations:    counts.Stations,
	}
	for _, value := range ofcom.Operators() {
		result.Operators = append(result.Operators,
			OperatorCount{Value: Operator(value), Stations: counts.Operators[value]})
	}
	for _, value := range ofcom.StationKinds() {
		result.Kinds = append(result.Kinds, KindCount{Value: Kind(value), Stations: counts.Kinds[value]})
	}
	for _, value := range ofcom.PowerClasses() {
		result.PowerClasses = append(result.PowerClasses,
			PowerClassCount{Value: PowerClass(value), Stations: counts.PowerClasses[value]})
	}
	for _, value := range ofcom.Technologies() {
		result.Technologies = append(result.Technologies,
			TechnologyCount{Value: Technology(value), Stations: counts.Technologies[value]})
	}
	return &countStationsOutput{Body: result}, nil
}

// 5. The snapshots.

// SnapshotList is a page of the snapshots.
type SnapshotList struct {
	Attribution string     `json:"attribution" doc:"The credit OFCOM's terms require."`
	Snapshots   []Snapshot `json:"snapshots" doc:"The newest first."`
	Total       int        `json:"total" doc:"How many snapshots there are, on every page."`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
}

type listSnapshotsInput struct {
	Paging
}

type listSnapshotsOutput struct {
	Body SnapshotList
}

func (h handlers) listSnapshots(ctx context.Context, input *listSnapshotsInput) (*listSnapshotsOutput, error) {
	page, err := h.data.SnapshotPage(ctx, input.page())
	if err != nil {
		return nil, h.internal(ctx, err)
	}
	result := SnapshotList{
		Attribution: Attribution,
		Snapshots:   make([]Snapshot, 0, len(page.Snapshots)),
		Total:       page.Total,
		Limit:       input.Limit,
		Offset:      input.Offset,
	}
	for _, snapshot := range page.Snapshots {
		result.Snapshots = append(result.Snapshots, snapshotOf(snapshot))
	}
	return &listSnapshotsOutput{Body: result}, nil
}
