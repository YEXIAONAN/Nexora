package database

// MinimumSupportedSchemaVersion is the oldest migration level the API can
// safely operate against. Phase05 contribution workflows require v7.
const MinimumSupportedSchemaVersion int64 = 7

// MaximumSupportedSchemaVersion includes the additive Phase06 M2 foundation.
// A future migration must be reviewed before increasing this value.
const MaximumSupportedSchemaVersion int64 = 8

// BonusSchemaVersion is the first schema containing the Phase06 bonus
// relations. Bonus routes and schema.d.ts models are only available from this
// version onwards; older compatible databases keep the Phase05 feature set.
const BonusSchemaVersion int64 = 8

// CompatibleSchemaVersion is shared by startup and readiness so a database
// cannot pass /readyz while the running API would reject it on restart.
func CompatibleSchemaVersion(version int64) bool {
	return version >= MinimumSupportedSchemaVersion && version <= MaximumSupportedSchemaVersion
}

// SupportsBonusSchema reports whether the Phase06 bonus read models exist.
// It never widens the startup compatibility window: a version outside the
// supported range is rejected before routes are registered.
func SupportsBonusSchema(version int64) bool {
	return CompatibleSchemaVersion(version) && version >= BonusSchemaVersion
}
