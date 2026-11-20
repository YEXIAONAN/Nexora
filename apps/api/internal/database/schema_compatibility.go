package database

// MinimumSupportedSchemaVersion is the oldest migration level the API can
// safely operate against. Alpha P0 relies on users.email and users.is_active,
// so v7 databases are only useful for historical migration tests.
const MinimumSupportedSchemaVersion int64 = 8

// MaximumSupportedSchemaVersion includes the additive task progress report
// migration. A future migration must be reviewed before increasing this value.
const MaximumSupportedSchemaVersion int64 = 9

// CompatibleSchemaVersion is shared by startup and readiness so a database
// cannot pass /readyz while the running API would reject it on restart.
func CompatibleSchemaVersion(version int64) bool {
	return version >= MinimumSupportedSchemaVersion && version <= MaximumSupportedSchemaVersion
}
