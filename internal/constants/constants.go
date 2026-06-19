// Package constants holds the server-wide locked combat constants shared across
// the model and report packages. Per-character values live in the TOML config
// (internal/charconfig), not here.
package constants

// Locked combat constants — the source of truth for these values (docs/SPEC.md §15 mirrors them).
// Values tagged "confirm vs guild leader / Varsoon parse" where uncertain.
const (
	CritMultiplier   = 1.50  // base crit factor: a crit is max(rangeMax+1, 1.50·roll) (measured 2026-06-19, spec §11)
	FlurryMultiplier = 4.0   // a flurry proc does +100%–500% more (2×–6×), averaging 4× — use the mean for expected DPS
	HasteStatCap     = 300.0 // haste stat hard cap; fitted curve gives f(300) ≈ 125.56 → shows 125%; overcap wasted (no flurry)
	DPSModCap        = 300.0 // dps-mod stat hard cap — shares the haste curve and cap

	RecastReductionCeiling = 0.50 // recast floor = 50% of base; reuse is a DIVISOR base/(1+reuse/100) that reaches it at 100% reuse (recalibrated 2026-06-18, six Eviscerate reads). AA art mods (Assassinate/Mortal Blade) sit on this floor.
	CARecoveryBaseSecs     = 0.5  // server base post-cast recovery, reduced subtractively by the character's recovery-speed stat (100 → "Recovery: Instant")

	DualWieldDelayPenalty = 1.33 // equipping an off-hand multiplies each weapon's auto-attack delay ×1.33 (measured 1.32–1.34 across two haste levels; documented +33%)

	// Rotation-sim parameters. Each cast occupies effCast + effRecovery (both reduced by the character's timing stats).
	FightDurationSecs = 600.0 // 10-minute fight (long-fight-aware; short enough that one extra big nuke matters)
	CACastTimeSecs    = 0.5   // combat arts share ~0.5s cast time
)
