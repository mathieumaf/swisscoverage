// Package ofcom reads the files that the Federal Office of Communications
// (OFCOM) publishes. The files, their vocabulary and their terms of use are
// described in docs/data-sources.md.
//
// The package is pure: it takes bytes and returns values. It never reads the
// network, the file system, the environment, a database or the clock, so that
// its tests are deterministic.
package ofcom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// This file parses OFCOM's public dataset of mobile phone base stations
// (ch.bakom.standorte-mobilfunkanlagen). The file is a GeoJSON
// FeatureCollection in LV95 whose properties are display strings repeated in
// four languages. The parser reads the English ones and turns them into a
// typed, language-neutral model.

// LV95 is a position in the Swiss LV95 grid (EPSG:2056), in metres.
type LV95 struct {
	East  float64 `json:"east"`
	North float64 `json:"north"`
}

// Operator is who runs a station.
type Operator string

const (
	OperatorSwisscom Operator = "swisscom"
	OperatorSunrise  Operator = "sunrise"
	OperatorSalt     Operator = "salt"
	OperatorSBB      Operator = "sbb"
	// OperatorOther groups the stations published without a known operator,
	// such as foreign stations near the border.
	OperatorOther Operator = "other"
)

// Technology is a mobile network generation.
type Technology string

const (
	Technology2G Technology = "2G"
	Technology3G Technology = "3G"
	Technology4G Technology = "4G"
	Technology5G Technology = "5G"
)

// StationKind is the type of site OFCOM publishes.
type StationKind string

const (
	KindOutdoor         StationKind = "outdoor"
	KindOutdoorLowPower StationKind = "outdoor_low_power"
	KindIndoor          StationKind = "indoor"
	KindTunnel          StationKind = "tunnel"
	KindTemporary       StationKind = "temporary"
	KindUnknown         StationKind = "unknown"
)

// PowerClass is the class of ERP90, the highest cumulated power in a 90°
// sector.
type PowerClass string

const (
	PowerVeryLow PowerClass = "very_low"
	PowerLow     PowerClass = "low"
	PowerMedium  PowerClass = "medium"
	PowerHigh    PowerClass = "high"
	PowerUnknown PowerClass = "unknown"
)

// PermitType says what OFCOM publishes about a site's permit.
type PermitType string

const (
	// PermitSiteDataSheet means a site data sheet was filed.
	PermitSiteDataSheet PermitType = "site_data_sheet"
	// PermitExempt means the site is exempt from the precautionary emission
	// limit (NISV Annex 1 No. 64).
	PermitExempt PermitType = "exempt"
)

// Every value of each vocabulary, in the order they are reported. Technologies
// are in generation order, which is also the order of a station's list.
var (
	allOperators = [...]Operator{
		OperatorSwisscom, OperatorSunrise, OperatorSalt, OperatorSBB, OperatorOther,
	}
	allTechnologies = [...]Technology{
		Technology2G, Technology3G, Technology4G, Technology5G,
	}
	allStationKinds = [...]StationKind{
		KindOutdoor, KindOutdoorLowPower, KindIndoor, KindTunnel, KindTemporary, KindUnknown,
	}
	allPowerClasses = [...]PowerClass{
		PowerVeryLow, PowerLow, PowerMedium, PowerHigh, PowerUnknown,
	}
)

// Operators returns every operator, in the order they are reported.
func Operators() []Operator { return slices.Clone(allOperators[:]) }

// Technologies returns every technology, in generation order.
func Technologies() []Technology { return slices.Clone(allTechnologies[:]) }

// StationKinds returns every kind of site, in the order they are reported.
func StationKinds() []StationKind { return slices.Clone(allStationKinds[:]) }

// PowerClasses returns every power class, from the lowest to unknown.
func PowerClasses() []PowerClass { return slices.Clone(allPowerClasses[:]) }

// Permit is the permit information of a site.
type Permit struct {
	Type PermitType `json:"type"`
	// Date is the date OFCOM publishes for a site data sheet, as YYYY-MM-DD.
	// It is empty for an exemption, and for a data sheet whose published
	// date is not a day that exists.
	Date string `json:"date,omitempty"`
}

// MobileStation is one site of the dataset.
type MobileStation struct {
	// Name is the station label exactly as published, e.g. "Sunrise ZH601-1".
	// It is not unique: one label appears at several positions. The pair of
	// label and position is the smallest safe identity.
	Name     string   `json:"name"`
	Operator Operator `json:"operator"`
	// Code is the label without the operator prefix, e.g. "ZH601-1".
	Code       string      `json:"code"`
	Kind       StationKind `json:"kind"`
	PowerClass PowerClass  `json:"powerClass"`
	// Technologies is never nil. It is empty when the file lists none that
	// the parser recognises.
	Technologies []Technology `json:"technologies"`
	// Adaptive is true when at least part of the site runs adaptive antennas.
	// It is nil when the file says something about it that the parser does
	// not recognise.
	Adaptive *bool `json:"adaptive"`
	// Permit is nil when OFCOM publishes no permit information for the site.
	Permit *Permit `json:"permit"`
	// InstallationLimitValue is in V/m. It is nil when absent or left blank.
	InstallationLimitValue *float64 `json:"installationLimitValue"`
	// LV95 is the position of the site's first mast, as published. The
	// database converts it to WGS84: PostGIS applies the rigorous
	// transformation, which the usual approximate formulas miss by up to
	// three metres near Geneva.
	LV95 LV95 `json:"lv95"`
}

// MobileStations is what one snapshot of the dataset contains.
type MobileStations struct {
	// Stations holds every feature of the file, in the file's order.
	Stations []MobileStation `json:"stations"`
	// Warnings lists the values the parser did not recognise. The station is
	// still returned, with the field set to unknown, nil or an empty list,
	// so one new upstream label never blocks an import. Callers decide
	// whether warnings should fail their run.
	Warnings []string `json:"warnings"`
}

// FormatError reports that the input is not the dataset this parser
// understands. No station is returned with it.
type FormatError struct {
	message string
}

func (e *FormatError) Error() string { return e.message }

func formatErrorf(format string, args ...any) error {
	return &FormatError{message: fmt.Sprintf(format, args...)}
}

const expectedCRS = "EPSG:2056"

var operators = map[string]Operator{
	"Swisscom": OperatorSwisscom,
	"Sunrise":  OperatorSunrise,
	"Salt":     OperatorSalt,
	"SBB":      OperatorSBB,
}

var kinds = map[string]StationKind{
	"Outdoor > 6 Werp":  KindOutdoor,
	"Outdoor ≤ 6 Werp":  KindOutdoorLowPower,
	"Indoor ≤ 6 Werp":   KindIndoor,
	"Tunnel":            KindTunnel,
	"Temporary station": KindTemporary,
	// About a hundred sites are published without a type.
	"": KindUnknown,
}

var powerClasses = map[string]PowerClass{
	"very low": PowerVeryLow,
	"low":      PowerLow,
	"medium":   PowerMedium,
	"high":     PowerHigh,
}

var adaptiveLabels = map[string]bool{
	"":                             false,
	"Partially adaptive operation": true,
}

const (
	technologyPrefix = "Technology "
	exemptMarker     = "NISV Annex 1 No. 64"
	// Published when OFCOM lists the field without a value.
	blankLimitValue = "Installation limit value - V/m"
)

var (
	powerPattern      = regexp.MustCompile(`^Power class : (very low|low|medium|high) \(`)
	dataSheetPattern  = regexp.MustCompile(`^Site data sheet (\d{4}-\d{2}-\d{2})$`)
	limitValuePattern = regexp.MustCompile(`^Installation limit value (\d+(?:\.\d+)?) V/m$`)
)

// ParseMobileStations parses a whole snapshot of the dataset.
//
// It returns a *FormatError when data is not a FeatureCollection in LV95 or
// when a feature lacks a field every station has. Coordinates read in the
// wrong reference system would be silently wrong, so that case stops the run
// instead of adding warnings.
func ParseMobileStations(data []byte) (MobileStations, error) {
	// Keys are matched exactly, as GeoJSON spells them: decoding into a map
	// does not fold case the way decoding into a struct would.
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		var wrongType *json.UnmarshalTypeError
		if errors.As(err, &wrongType) {
			return MobileStations{}, formatErrorf("expected a GeoJSON FeatureCollection")
		}
		return MobileStations{}, formatErrorf("the file is not valid JSON: %v", err)
	}
	if decode(document["type"]) != "FeatureCollection" {
		return MobileStations{}, formatErrorf("expected a GeoJSON FeatureCollection")
	}
	crs, ok := readCRS(decode(document["crs"]))
	if !ok {
		return MobileStations{}, formatErrorf("expected coordinates in %s, found no CRS", expectedCRS)
	}
	if crs != expectedCRS {
		return MobileStations{}, formatErrorf("expected coordinates in %s, found %s", expectedCRS, crs)
	}
	// A JSON null decodes to a nil slice and an empty array to an empty one.
	var features []json.RawMessage
	if err := json.Unmarshal(document["features"], &features); err != nil || features == nil {
		return MobileStations{}, formatErrorf("the FeatureCollection has no features array")
	}

	parsed := MobileStations{
		Stations: make([]MobileStation, 0, len(features)),
		Warnings: []string{},
	}
	for index, feature := range features {
		station, err := parseFeature(feature, index, &parsed.Warnings)
		if err != nil {
			return MobileStations{}, err
		}
		parsed.Stations = append(parsed.Stations, station)
	}
	return parsed, nil
}

func parseFeature(raw json.RawMessage, index int, warnings *[]string) (MobileStation, error) {
	where := fmt.Sprintf("feature %d", index)
	feature, _ := decode(raw).(map[string]any)
	properties, ok := feature["properties"].(map[string]any)
	if !ok {
		return MobileStation{}, formatErrorf("%s has no properties", where)
	}
	name, err := readString(properties, "station", where)
	if err != nil {
		return MobileStation{}, err
	}
	if name == "" {
		return MobileStation{}, formatErrorf("%s has an empty station label", where)
	}
	position, err := readPosition(feature["geometry"], where+" ("+name+")")
	if err != nil {
		return MobileStation{}, err
	}
	warn := func(field, value string) {
		*warnings = append(*warnings, fmt.Sprintf(`%s: unrecognised %s "%s"`, name, field, value))
	}

	kindLabel, err := readString(properties, "typ_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	kind, ok := kinds[kindLabel]
	if !ok {
		kind = KindUnknown
		warn("type", kindLabel)
	}

	powerLabel, err := readString(properties, "power_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	powerClass := PowerUnknown
	if match := powerPattern.FindStringSubmatch(powerLabel); match != nil {
		powerClass = powerClasses[match[1]]
	} else {
		warn("power class", powerLabel)
	}

	technologyLabel, err := readString(properties, "techno_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	// Published as "Technology 3G,4G,5G". Only that exact shape is read: the
	// prefix, then known items separated by commas. Anything else, such as an
	// unknown item beside known ones or another separator, adds a warning.
	listed := listedTechnologies(technologyLabel)
	technologies := []Technology{}
	for _, technology := range allTechnologies {
		if slices.Contains(listed, string(technology)) {
			technologies = append(technologies, technology)
		}
	}
	if len(listed) == 0 || len(technologies) != len(listed) {
		warn("technology", technologyLabel)
	}

	adaptiveLabel, err := readString(properties, "adaptiv_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	var adaptive *bool
	if value, ok := adaptiveLabels[adaptiveLabel]; ok {
		adaptive = &value
	} else {
		warn("adaptive operation", adaptiveLabel)
	}

	permitLabel, err := readString(properties, "bewilligung_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	permit, ok := parsePermit(permitLabel)
	if !ok {
		warn("permit", permitLabel)
	}

	limitLabel, err := readString(properties, "agw_en", where)
	if err != nil {
		return MobileStation{}, err
	}
	limitValue, ok := parseLimitValue(limitLabel)
	if !ok {
		warn("installation limit value", limitLabel)
	}

	operator, code := splitName(name)
	return MobileStation{
		Name:                   name,
		Operator:               operator,
		Code:                   code,
		Kind:                   kind,
		PowerClass:             powerClass,
		Technologies:           technologies,
		Adaptive:               adaptive,
		Permit:                 permit,
		InstallationLimitValue: limitValue,
		LV95:                   position,
	}, nil
}

// splitName separates the operator from the station code. The three operators
// and SBB are published as "Operator CODE". Foreign stations near the border
// come as a bare code such as "DTM-SCHL" and are grouped under "other" with
// the whole label as their code.
func splitName(name string) (Operator, string) {
	prefix, code, found := strings.Cut(name, " ")
	if !found {
		return OperatorOther, name
	}
	operator, ok := operators[prefix]
	if !ok {
		return OperatorOther, name
	}
	return operator, code
}

// listedTechnologies returns the items of a "Technology a,b,c" label, or nil
// when the label does not have that shape.
func listedTechnologies(label string) []string {
	list, found := strings.CutPrefix(label, technologyPrefix)
	// A list is one line of text.
	if !found || list == "" || strings.ContainsAny(list, "\n\r\u2028\u2029") {
		return nil
	}
	items := strings.Split(list, ",")
	for index, item := range items {
		items[index] = strings.TrimSpace(item)
	}
	return items
}

// parsePermit returns false when the label says something the parser does not
// recognise, with what it could still read. A blank label is recognised: it
// means no permit information.
func parsePermit(label string) (*Permit, bool) {
	if label == "" {
		return nil, true
	}
	if match := dataSheetPattern.FindStringSubmatch(label); match != nil {
		// A date such as 2021-02-30 is kept out: what stores the result
		// expects a real day. The data sheet itself is still recorded.
		if _, err := time.Parse(time.DateOnly, match[1]); err != nil {
			return &Permit{Type: PermitSiteDataSheet}, false
		}
		return &Permit{Type: PermitSiteDataSheet, Date: match[1]}, true
	}
	if strings.Contains(label, exemptMarker) {
		return &Permit{Type: PermitExempt}, true
	}
	return nil, false
}

// parseLimitValue returns false when the label says something the parser does
// not recognise. A blank label is recognised: it means no value.
func parseLimitValue(label string) (*float64, bool) {
	if label == "" || label == blankLimitValue {
		return nil, true
	}
	match := limitValuePattern.FindStringSubmatch(label)
	if match == nil {
		return nil, false
	}
	// Fails only for a number too large to hold, which is not a field
	// strength.
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return nil, false
	}
	return &value, true
}

func readPosition(value any, where string) (LV95, error) {
	geometry, _ := value.(map[string]any)
	coordinates, _ := geometry["coordinates"].([]any)
	if geometry["type"] == "Point" && len(coordinates) >= 2 {
		east, eastOK := finiteNumber(coordinates[0])
		north, northOK := finiteNumber(coordinates[1])
		if eastOK && northOK {
			return LV95{East: east, North: north}, nil
		}
	}
	return LV95{}, formatErrorf("%s has no point geometry", where)
}

func readCRS(value any) (string, bool) {
	crs, _ := value.(map[string]any)
	properties, _ := crs["properties"].(map[string]any)
	name, ok := properties["name"].(string)
	return name, ok
}

func readString(properties map[string]any, key, where string) (string, error) {
	value, ok := properties[key].(string)
	if !ok {
		return "", formatErrorf(`%s has no "%s" property`, where, key)
	}
	return value, nil
}

// decode reads one JSON value: an object as map[string]any, an array as
// []any, a string as string and a number as json.Number. It returns nil for
// null, for nothing, and for a value that cannot be read.
func decode(raw json.RawMessage) any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// Numbers are kept as written, so that one too large to hold is seen as
	// such instead of failing the decoding of everything around it.
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	return value
}

func finiteNumber(value any) (float64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	// Fails only for a number too large to hold.
	parsed, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
