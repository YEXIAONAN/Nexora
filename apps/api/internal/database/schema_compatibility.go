package database

// MinimumSupportedSchemaVersion is the oldest migration level the API can
// safely operate against. Phase07 progress routes require task_progress_reports
// from v9; v7/v8 databases are only useful for historical migration tests.
const MinimumSupportedSchemaVersion int64 = 9

const MaximumSupportedSchemaVersion int64 = 11

// Bonus routes require the publication and payment records introduced in v11.
const BonusSchemaVersion int64 = 11

// CompatibleSchemaVersion is shared by startup and readiness so a database
// cannot pass /readyz while the running API would reject it on restart.
func CompatibleSchemaVersion(version int64) bool {
	return version >= MinimumSupportedSchemaVersion && version <= MaximumSupportedSchemaVersion
}

// Bonus reads and writes share the same publication-aware schema boundary.
func SupportsBonusSchema(version int64) bool {
	return CompatibleSchemaVersion(version) && version >= BonusSchemaVersion
}
