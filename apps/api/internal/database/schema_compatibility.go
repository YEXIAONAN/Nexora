package database

// MinimumSupportedSchemaVersion is the oldest migration level the API can
// safely operate against. v8 is the released Alpha P0 baseline: the auth and
// invitation flows rely on users.email / users.is_active, so v7 databases are
// only useful for historical migration tests and must not serve traffic.
const MinimumSupportedSchemaVersion int64 = 8

// MaximumSupportedSchemaVersion includes the additive Phase06 bonus
// foundation. A future migration must be reviewed before increasing this value.
const MaximumSupportedSchemaVersion int64 = 9

// BonusSchemaVersion is the first schema that carries the Phase06 bonus
// foundation. Bonus routes are mounted only when the database is at least this
// version, so a v8 (Alpha P0) deployment keeps serving P0 traffic without
// bonus endpoints while the same binary can serve v9 once migrated.
const BonusSchemaVersion int64 = 9

// CompatibleSchemaVersion is shared by startup and readiness so a database
// cannot pass /readyz while the running API would reject it on restart.
func CompatibleSchemaVersion(version int64) bool {
	return version >= MinimumSupportedSchemaVersion && version <= MaximumSupportedSchemaVersion
}

// SupportsBonus reports whether the given schema carries the Phase06 bonus
// foundation. Route mounting and readiness use the same predicate.
func SupportsBonus(version int64) bool {
	return version >= BonusSchemaVersion
}
