// Package storetest holds what the tests of the store and of the API share: a
// store on a database of its own with the schema applied, sites with ordinary
// values, and the snapshots that load them. Only tests import it.
package storetest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mathieumaf/swisscoverage/backend/internal/ofcom"
	"github.com/mathieumaf/swisscoverage/backend/internal/store"
	"github.com/mathieumaf/swisscoverage/backend/internal/testdb"
)

// Three publication times, a day apart, as OFCOM's files have them.
var (
	Day1 = time.Date(2026, 11, 23, 22, 1, 35, 611529000, time.UTC)
	Day2 = time.Date(2026, 11, 24, 22, 1, 55, 936174000, time.UTC)
	Day3 = time.Date(2026, 11, 25, 22, 2, 7, 0, time.UTC)
)

// Open returns a store on an empty database with the schema applied, and the
// address of that database, for assertions and for other connections.
func Open(t *testing.T) (*store.Store, string) {
	t.Helper()
	address := testdb.New(t)
	database, err := store.Open(context.Background(), address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return database, address
}

// Pointer returns the address of a copy of value.
func Pointer[T any](value T) *T { return &value }

// Station returns a site with ordinary values, which a test then changes. The
// operator is the first word of the label.
func Station(name string, east, north float64) ofcom.MobileStation {
	operator, code, _ := strings.Cut(name, " ")
	return ofcom.MobileStation{
		Name:                   name,
		Operator:               ofcom.Operator(strings.ToLower(operator)),
		Code:                   code,
		Kind:                   ofcom.KindOutdoor,
		PowerClass:             ofcom.PowerMedium,
		Technologies:           []ofcom.Technology{ofcom.Technology4G, ofcom.Technology5G},
		Adaptive:               Pointer(true),
		Permit:                 &ofcom.Permit{Type: ofcom.PermitSiteDataSheet, Date: "2021-06-03"},
		InstallationLimitValue: Pointer(5.0),
		LV95:                   ofcom.LV95{East: east, North: north},
	}
}

// With returns a copy of a station after change has edited it.
func With(original ofcom.MobileStation, change func(*ofcom.MobileStation)) ofcom.MobileStation {
	change(&original)
	return original
}

// Snapshot returns a snapshot whose checksum depends on its time only.
func Snapshot(publishedAt time.Time, sites int) store.Snapshot {
	checksum := strings.Repeat("0", 60) + publishedAt.Format("0102")
	return store.Snapshot{PublishedAt: publishedAt, SHA256: checksum, Sites: sites}
}
