package database

// MinimumSupportedSchemaVersion is the oldest migration level the API can
// safely operate against. Phase05 contribution workflows require v7.
const MinimumSupportedSchemaVersion int64 = 7

// MaximumSupportedSchemaVersion includes the additive Phase06 M2 foundation.
// A future migration must be reviewed before increasing this value.
const MaximumSupportedSchemaVersion int64 = 8

// CompatibleSchemaVersion is shared by startup and readiness so a database
// cannot pass /readyz while the running API would reject it on restart.
func CompatibleSchemaVersion(version int64) bool {
	return version >= MinimumSupportedSchemaVersion && version <= MaximumSupportedSchemaVersion
}
