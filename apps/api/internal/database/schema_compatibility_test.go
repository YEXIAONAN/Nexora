package database

import "testing"

func TestCompatibleSchemaVersion(t *testing.T) {
	for _, tc := range []struct {
		version int64
		want    bool
	}{
		{0, false},
		{6, false},
		{7, false},
		{8, true},
		{9, true},
		{10, false},
		{100, false},
	} {
		if got := CompatibleSchemaVersion(tc.version); got != tc.want {
			t.Errorf("schema v%d compatibility = %t, want %t", tc.version, got, tc.want)
		}
	}
}
