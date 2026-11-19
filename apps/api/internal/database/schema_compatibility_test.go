package database

import "testing"

func TestSupportsBonusSchema(t *testing.T) {
	for _, tc := range []struct {
		version int64
		want    bool
	}{
		{0, false},
		{6, false},
		{7, false},
		{8, true},
		{9, false},
	} {
		if got := SupportsBonusSchema(tc.version); got != tc.want {
			t.Errorf("schema v%d bonus support = %t, want %t", tc.version, got, tc.want)
		}
	}
}

func TestCompatibleSchemaVersion(t *testing.T) {
	for _, tc := range []struct {
		version int64
		want    bool
	}{
		{0, false},
		{6, false},
		{7, true},
		{8, true},
		{9, false},
		{100, false},
	} {
		if got := CompatibleSchemaVersion(tc.version); got != tc.want {
			t.Errorf("schema v%d compatibility = %t, want %t", tc.version, got, tc.want)
		}
	}
}
