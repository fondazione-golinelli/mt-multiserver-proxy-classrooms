package main

import (
	"database/sql"
	"testing"
)

func TestZoneAccessRoundTrip(t *testing.T) {
	groups := []classGroup{{ID: 7, Name: "Red"}, {ID: 9, Name: "Blue"}}
	options := zoneAccessOptions(groups)
	if len(options) != 4 || options[0] != "Teachers only" || options[1] != "Everyone" || options[3] != "Group: Blue" {
		t.Fatalf("unexpected options %v", options)
	}

	zones := []zoneData{
		{},
		{Open: true},
		{GroupID: sql.NullInt64{Int64: 7, Valid: true}},
		{GroupID: sql.NullInt64{Int64: 9, Valid: true}},
	}
	for want, z := range zones {
		idx := zoneAccessIndex(z, groups)
		if idx != want+1 {
			t.Fatalf("zone %d: index %d, want %d", want, idx, want+1)
		}
		access, _, ok := zoneAccessFromIndex(idx, groups)
		if !ok || access.Open != z.Open || (z.GroupID.Valid && int64(access.GroupID) != z.GroupID.Int64) {
			t.Fatalf("zone %d: round trip gave %+v", want, access)
		}
	}
	if _, _, ok := zoneAccessFromIndex(5, groups); ok {
		t.Fatal("index past the last group must be rejected")
	}
	// A zone whose group was deleted falls back to "Teachers only".
	if idx := zoneAccessIndex(zoneData{GroupID: sql.NullInt64{Int64: 99, Valid: true}}, groups); idx != 1 {
		t.Fatalf("missing group index %d", idx)
	}
}
