// Package constants holds locked combat constants shared between the model and
// baseline packages. Keeping them here breaks the import cycle that would arise
// if model imported baseline (which itself imports model for StatBlock).
package constants

// Locked combat constants (docs/design-plan2.md §11). Single source of truth.
// Values tagged "confirm vs guild leader / Varsoon parse" where uncertain.
const (
	CritMultiplier    = 1.30  // a crit deals +30%
	FlurryMultiplier  = 4.0   // a flurry proc does +100%–500% more (2×–6×), averaging 4× — use the mean for expected DPS
	HasteStatCap      = 300.0 // haste stat hard cap; fitted curve gives f(300) ≈ 125.56 → shows 125%; overcap wasted (no flurry)
	DPSModCap         = 300.0 // dps-mod stat hard cap — shares the haste curve and cap
	AbilityModCapFrac = 0.50  // +CA-dmg cap = 50% of the potency-adjusted CA base
	ReuseHalvesAt     = 100.0 // 100% reuse → recast halved (cap)
	ReuseHalveCoeff   = 0.50  // recast reduction coefficient at full reuse

	// Rotation-sim parameters. Each cast occupies the timeline for cast +
	// recovery (CACastTimeSecs + CARecoverySecs = 0.75s).
	FightDurationSecs = 600.0 // 10-minute fight (long-fight-aware; short enough that one extra big nuke matters)
	CACastTimeSecs    = 0.5   // combat arts share ~0.5s cast time
	CARecoverySecs    = 0.25  // post-cast recovery; base 0.5s, halved by AA
)
