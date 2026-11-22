package database

import (
	"fmt"
	"strings"
	"testing"
)

func TestSchemaGuardMatchesMigrationChain(t *testing.T) {
	entries, err := migrationFS.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(entries)) != MaximumSupportedSchemaVersion {
		t.Fatalf("migration count = %d, supported current version = %d", len(entries), MaximumSupportedSchemaVersion)
	}
	for i, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), fmt.Sprintf("%05d_", i+1)) || !strings.HasSuffix(entry.Name(), ".sql") {
			t.Fatalf("migration chain is not unique and consecutive at %s", entry.Name())
		}
	}
	for version, name := range map[int64]string{
		MinimumSupportedSchemaVersion: "00009_task_progress_reports.sql",
		10:                            "00010_bonus_foundation.sql",
		BonusSchemaVersion:            "00011_bonus_workflow.sql",
	} {
		if entries[version-1].Name() != name {
			t.Errorf("schema gate v%d must correspond to %s", version, name)
		}
	}
}

func TestSupportsBonusSchema(t *testing.T) {
	for _, tc := range []struct {
		version int64
		want    bool
	}{
		{0, false},
		{6, false},
		{7, false},
		{8, false},
		{9, false},
		{10, false},
		{11, true},
		{12, false},
		{100, false},
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
		{7, false},
		{8, false},
		{9, true},
		{10, true},
		{11, true},
		{12, false},
		{100, false},
	} {
		if got := CompatibleSchemaVersion(tc.version); got != tc.want {
			t.Errorf("schema v%d compatibility = %t, want %t", tc.version, got, tc.want)
		}
	}
}
