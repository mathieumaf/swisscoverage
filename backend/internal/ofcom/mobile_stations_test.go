package ofcom

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files from what the parser returns")

const (
	samplePath = "testdata/mobile-stations.sample.json"
	goldenPath = "testdata/mobile-stations.golden.json"
)

// The fixture holds nine real stations taken from the snapshot of 2026-11-23,
// one per shape the file is known to contain.
func readSample(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseSample(t *testing.T) MobileStations {
	t.Helper()
	parsed, err := ParseMobileStations(readSample(t))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func byName(t *testing.T, stations []MobileStation, name string) MobileStation {
	t.Helper()
	for _, station := range stations {
		if station.Name == name {
			return station
		}
	}
	t.Fatalf("no station named %s", name)
	return MobileStation{}
}

// sampleDocument returns the fixture as a document a test can change.
func sampleDocument(t *testing.T) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(readSample(t), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func encode(t *testing.T, document any) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// firstFeatureWith returns the fixture reduced to its first feature, after
// change has edited that feature.
func firstFeatureWith(t *testing.T, change func(feature map[string]any)) []byte {
	t.Helper()
	document := sampleDocument(t)
	feature := document["features"].([]any)[0].(map[string]any)
	change(feature)
	document["features"] = []any{feature}
	return encode(t, document)
}

// documentWith returns the fixture's first feature with some properties
// replaced.
func documentWith(t *testing.T, replaced map[string]any) []byte {
	t.Helper()
	return firstFeatureWith(t, func(feature map[string]any) {
		properties := feature["properties"].(map[string]any)
		for key, value := range replaced {
			properties[key] = value
		}
	})
}

func parse(t *testing.T, data []byte) MobileStations {
	t.Helper()
	parsed, err := ParseMobileStations(data)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// wantFormatError checks that data is refused with a FormatError whose
// message contains want.
func wantFormatError(t *testing.T, data []byte, want string) {
	t.Helper()
	parsed, err := ParseMobileStations(data)
	var formatError *FormatError
	if !errors.As(err, &formatError) {
		t.Fatalf("got error %v, want a FormatError", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("got error %q, want it to contain %q", err, want)
	}
	if parsed.Stations != nil || parsed.Warnings != nil {
		t.Errorf("got %+v beside the error, want nothing", parsed)
	}
}

func wantEqual[T any](t *testing.T, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func pointer[T any](value T) *T { return &value }

// These tests are the ones of the TypeScript parser this one replaces, ported
// one for one and under the same names.
func TestParseMobileStations(t *testing.T) {
	sample := parseSample(t)
	stations := sample.Stations

	t.Run("reads every station of the sample without warnings", func(t *testing.T) {
		features := sampleDocument(t)["features"].([]any)
		wantEqual(t, len(stations), len(features))
		wantEqual(t, sample.Warnings, []string{})
	})

	t.Run("reads a typical operator site", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom BIFO"), MobileStation{
			Name:                   "Swisscom BIFO",
			Operator:               OperatorSwisscom,
			Code:                   "BIFO",
			Kind:                   KindOutdoor,
			PowerClass:             PowerMedium,
			Technologies:           []Technology{Technology4G, Technology5G},
			Adaptive:               pointer(true),
			Permit:                 &Permit{Type: PermitSiteDataSheet, Date: "2021-06-03"},
			InstallationLimitValue: pointer(5.0),
			LV95:                   LV95{East: 2_676_878, North: 1_246_069},
		})
	})

	t.Run("separates the operator from the station code", func(t *testing.T) {
		salt := byName(t, stations, "Salt TI_3787A")
		wantEqual(t, salt.Operator, OperatorSalt)
		wantEqual(t, salt.Code, "TI_3787A")
		sunrise := byName(t, stations, "Sunrise BN375-1")
		wantEqual(t, sunrise.Operator, OperatorSunrise)
		wantEqual(t, sunrise.Code, "BN375-1")
		sbb := byName(t, stations, "SBB AU-WAE-02202")
		wantEqual(t, sbb.Operator, OperatorSBB)
		wantEqual(t, sbb.Code, "AU-WAE-02202")
	})

	t.Run("groups foreign border stations under other", func(t *testing.T) {
		foreign := byName(t, stations, "DB-BSBB")
		wantEqual(t, foreign.Operator, OperatorOther)
		wantEqual(t, foreign.Code, "DB-BSBB")
	})

	t.Run("reads each kind of site", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom BIFO").Kind, KindOutdoor)
		wantEqual(t, byName(t, stations, "SBB AU-WAE-02202").Kind, KindOutdoorLowPower)
		wantEqual(t, byName(t, stations, "Swisscom BEAL").Kind, KindIndoor)
		wantEqual(t, byName(t, stations, "Salt FR_7401A").Kind, KindTunnel)
		wantEqual(t, byName(t, stations, "Swisscom XTHR").Kind, KindTemporary)
	})

	t.Run("accepts a site published without a type", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Sunrise BN375-1").Kind, KindUnknown)
	})

	t.Run("reads each power class", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom BEAL").PowerClass, PowerVeryLow)
		wantEqual(t, byName(t, stations, "Swisscom DIBH").PowerClass, PowerLow)
		wantEqual(t, byName(t, stations, "Swisscom BIFO").PowerClass, PowerMedium)
		wantEqual(t, byName(t, stations, "Salt TI_3787A").PowerClass, PowerHigh)
	})

	t.Run("reads the technologies in generation order", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "SBB AU-WAE-02202").Technologies,
			[]Technology{Technology2G})
		wantEqual(t, byName(t, stations, "Salt TI_3787A").Technologies,
			[]Technology{Technology3G, Technology4G, Technology5G})
	})

	t.Run("reads the adaptive flag", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom BEAL").Adaptive, pointer(true))
		wantEqual(t, byName(t, stations, "Swisscom DIBH").Adaptive, pointer(false))
	})

	t.Run("distinguishes a data sheet, an exemption and no permit", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom DIBH").Permit,
			&Permit{Type: PermitSiteDataSheet, Date: "2018-09-18"})
		wantEqual(t, byName(t, stations, "Salt FR_7401A").Permit, &Permit{Type: PermitExempt})
		wantEqual(t, byName(t, stations, "Swisscom BEAL").Permit, nil)
	})

	t.Run("reads the installation limit value, or nil when left blank", func(t *testing.T) {
		wantEqual(t, byName(t, stations, "Swisscom DIBH").InstallationLimitValue, pointer(6.0))
		// Published as "Installation limit value - V/m".
		wantEqual(t, byName(t, stations, "SBB AU-WAE-02202").InstallationLimitValue, nil)
		// Published as an empty string.
		wantEqual(t, byName(t, stations, "Swisscom BEAL").InstallationLimitValue, nil)
	})

	t.Run("keeps a station with unrecognised labels and reports them", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{
			"typ_en":         "Balloon",
			"power_en":       "Power class : enormous",
			"techno_en":      "Technology 6G",
			"adaptiv_en":     "Fully adaptive operation",
			"bewilligung_en": "Pending",
			"agw_en":         "Installation limit value high",
		}))

		wantEqual(t, result.Stations, []MobileStation{{
			Name:                   "Swisscom BIFO",
			Operator:               OperatorSwisscom,
			Code:                   "BIFO",
			Kind:                   KindUnknown,
			PowerClass:             PowerUnknown,
			Technologies:           []Technology{},
			Adaptive:               nil,
			Permit:                 nil,
			InstallationLimitValue: nil,
			LV95:                   LV95{East: 2_676_878, North: 1_246_069},
		}})
		wantEqual(t, result.Warnings, []string{
			`Swisscom BIFO: unrecognised type "Balloon"`,
			`Swisscom BIFO: unrecognised power class "Power class : enormous"`,
			`Swisscom BIFO: unrecognised technology "Technology 6G"`,
			`Swisscom BIFO: unrecognised adaptive operation "Fully adaptive operation"`,
			`Swisscom BIFO: unrecognised permit "Pending"`,
			`Swisscom BIFO: unrecognised installation limit value "Installation limit value high"`,
		})
	})

	t.Run("reports a technology it does not know beside ones it does", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{"techno_en": "Technology 4G,6G"}))

		wantEqual(t, result.Stations[0].Technologies, []Technology{Technology4G})
		wantEqual(t, result.Warnings, []string{
			`Swisscom BIFO: unrecognised technology "Technology 4G,6G"`,
		})
	})

	t.Run("reports a technology list that is not in the published shape", func(t *testing.T) {
		for _, label := range []string{"4G,5G", "Technology4G", "Technology 4G/5G", ""} {
			result := parse(t, documentWith(t, map[string]any{"techno_en": label}))

			wantEqual(t, result.Warnings, []string{
				`Swisscom BIFO: unrecognised technology "` + label + `"`,
			})
		}
	})

	// These labels are names every JavaScript object inherits. The TypeScript
	// parser once read them as known values; the test stays as a guard.
	t.Run("does not mistake a label for a built-in property name", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{
			"station":    "constructor X",
			"typ_en":     "toString",
			"adaptiv_en": "constructor",
		}))

		station := result.Stations[0]
		wantEqual(t, station.Operator, OperatorOther)
		wantEqual(t, station.Code, "constructor X")
		wantEqual(t, station.Kind, KindUnknown)
		wantEqual(t, station.Adaptive, nil)
		wantEqual(t, result.Warnings, []string{
			`constructor X: unrecognised type "toString"`,
			`constructor X: unrecognised adaptive operation "constructor"`,
		})
	})

	t.Run("rejects a document that is not a FeatureCollection", func(t *testing.T) {
		wantFormatError(t, []byte(`{"type": "Feature"}`), "expected a GeoJSON FeatureCollection")
		wantFormatError(t, []byte(`null`), "expected a GeoJSON FeatureCollection")
	})

	t.Run("rejects coordinates in another reference system", func(t *testing.T) {
		wgs84 := sampleDocument(t)
		wgs84["crs"] = map[string]any{
			"type":       "name",
			"properties": map[string]any{"name": "EPSG:4326"},
		}
		wantFormatError(t, encode(t, wgs84), "expected coordinates in EPSG:2056, found EPSG:4326")

		withoutCRS := sampleDocument(t)
		delete(withoutCRS, "crs")
		wantFormatError(t, encode(t, withoutCRS), "found no CRS")
	})

	t.Run("rejects a feature without a station label or a position", func(t *testing.T) {
		wantFormatError(t, documentWith(t, map[string]any{"station": 42}),
			`feature 0 has no "station" property`)
		wantFormatError(t, documentWith(t, map[string]any{"station": ""}),
			"feature 0 has an empty station label")
		withoutGeometry := firstFeatureWith(t, func(feature map[string]any) {
			feature["geometry"] = nil
		})
		wantFormatError(t, withoutGeometry, "feature 0 (Swisscom BIFO) has no point geometry")
	})
}

// The golden file is what the TypeScript parser returned for the fixture
// before it was removed. From then on it records what this parser returns:
// a change to it shows up in review as a change to the file.
func TestParseMobileStationsGolden(t *testing.T) {
	var got bytes.Buffer
	encoder := json.NewEncoder(&got)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(parseSample(t)); err != nil {
		t.Fatal(err)
	}

	if *update {
		if err := os.WriteFile(goldenPath, got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("the parser's output differs from %s.\ngot:\n%s", goldenPath, got.Bytes())
	}
}

// The cases below have no counterpart in the TypeScript tests: they cover
// what only exists now that the parser takes the bytes of the file.
func TestParseMobileStationsInput(t *testing.T) {
	t.Run("rejects bytes that are not JSON", func(t *testing.T) {
		wantFormatError(t, []byte(`{"type": "FeatureCollection"`), "the file is not valid JSON")
		wantFormatError(t, nil, "the file is not valid JSON")
	})

	t.Run("rejects JSON that is not an object", func(t *testing.T) {
		wantFormatError(t, []byte(`[]`), "expected a GeoJSON FeatureCollection")
		wantFormatError(t, []byte(`"FeatureCollection"`), "expected a GeoJSON FeatureCollection")
	})

	t.Run("rejects a collection without a features array", func(t *testing.T) {
		for _, features := range []any{nil, map[string]any{}, "none"} {
			document := sampleDocument(t)
			document["features"] = features
			wantFormatError(t, encode(t, document), "the FeatureCollection has no features array")
		}
		missing := sampleDocument(t)
		delete(missing, "features")
		wantFormatError(t, encode(t, missing), "the FeatureCollection has no features array")
	})

	t.Run("returns empty lists for a collection without features", func(t *testing.T) {
		document := sampleDocument(t)
		document["features"] = []any{}
		wantEqual(t, parse(t, encode(t, document)), MobileStations{
			Stations: []MobileStation{},
			Warnings: []string{},
		})
	})

	t.Run("rejects a feature that is not an object with properties", func(t *testing.T) {
		for _, feature := range []any{nil, 42, []any{}, map[string]any{"properties": "none"}} {
			document := sampleDocument(t)
			document["features"] = []any{feature}
			wantFormatError(t, encode(t, document), "feature 0 has no properties")
		}
	})

	t.Run("rejects a feature without one of the labels every station has", func(t *testing.T) {
		for _, key := range []string{
			"typ_en", "power_en", "techno_en", "adaptiv_en", "bewilligung_en", "agw_en",
		} {
			data := firstFeatureWith(t, func(feature map[string]any) {
				delete(feature["properties"].(map[string]any), key)
			})
			wantFormatError(t, data, `feature 0 has no "`+key+`" property`)
		}
	})

	t.Run("matches keys exactly", func(t *testing.T) {
		document := sampleDocument(t)
		document["Features"] = document["features"]
		delete(document, "features")
		wantFormatError(t, encode(t, document), "the FeatureCollection has no features array")
	})

	t.Run("rejects a position that is not two finite numbers", func(t *testing.T) {
		for _, coordinates := range []string{
			`[2676878]`, `["2676878", "1246069"]`, `[2676878, null]`, `[1e999, 1246069]`, `{}`,
		} {
			data := firstFeatureWith(t, func(feature map[string]any) {
				feature["geometry"] = map[string]any{
					"type":        "Point",
					"coordinates": json.RawMessage(coordinates),
				}
			})
			wantFormatError(t, data, "feature 0 (Swisscom BIFO) has no point geometry")
		}
		polygon := firstFeatureWith(t, func(feature map[string]any) {
			feature["geometry"].(map[string]any)["type"] = "Polygon"
		})
		wantFormatError(t, polygon, "feature 0 (Swisscom BIFO) has no point geometry")
	})

	t.Run("keeps a position with decimals and extra dimensions", func(t *testing.T) {
		data := firstFeatureWith(t, func(feature map[string]any) {
			feature["geometry"].(map[string]any)["coordinates"] = []any{2676878.25, 1246069.5, 431}
		})
		wantEqual(t, parse(t, data).Stations[0].LV95, LV95{East: 2676878.25, North: 1246069.5})
	})

	t.Run("reports a technology list that runs over several lines", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{"techno_en": "Technology 4G\n5G"}))

		wantEqual(t, result.Stations[0].Technologies, []Technology{})
		wantEqual(t, result.Warnings, []string{
			"Swisscom BIFO: unrecognised technology \"Technology 4G\n5G\"",
		})
	})

	t.Run("reports a limit value too large to hold", func(t *testing.T) {
		label := "Installation limit value " + strings.Repeat("9", 400) + " V/m"
		result := parse(t, documentWith(t, map[string]any{"agw_en": label}))

		wantEqual(t, result.Stations[0].InstallationLimitValue, nil)
		wantEqual(t, result.Warnings, []string{
			`Swisscom BIFO: unrecognised installation limit value "` + label + `"`,
		})
	})

	t.Run("keeps a data sheet whose date does not exist, without the date", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{
			"bewilligung_en": "Site data sheet 2021-02-30",
		}))

		wantEqual(t, result.Stations[0].Permit, &Permit{Type: PermitSiteDataSheet})
		wantEqual(t, result.Warnings, []string{
			`Swisscom BIFO: unrecognised permit "Site data sheet 2021-02-30"`,
		})
	})

	t.Run("reads a limit value with decimals", func(t *testing.T) {
		result := parse(t, documentWith(t, map[string]any{
			"agw_en": "Installation limit value 4.5 V/m",
		}))

		wantEqual(t, result.Stations[0].InstallationLimitValue, pointer(4.5))
		wantEqual(t, result.Warnings, []string{})
	})
}
