package baseline

import "github.com/amdrake93/eq2-eof-itemdex/internal/model"

// Locked combat constants (docs/design-plan2.md §11). Single source of truth.
// Values tagged "confirm vs guild leader / Varsoon parse" where uncertain.
const (
	CritMultiplier    = 1.30  // a crit deals +30%
	FlurryMultiplier  = 5.0   // flurry applies a 5× burst to the auto-attack
	HasteCapPct       = 100.0 // haste effect cap; overcap converts to flurry
	HasteToFlurry     = 10.0  // 10 points overcap haste → 1% flurry
	DPSModCap         = 200.0 // dps-mod cap (≈ +125%); overcap wasted
	DPSModEffectAtCap = 1.25  // auto-attack multiplier added at the dps-mod cap
	AbilityModCapFrac = 0.50  // +CA-dmg cap = 50% of the potency-adjusted CA base
	ReuseHalvesAt     = 100.0 // 100% reuse → recast halved (cap)
	ReuseHalveCoeff   = 0.50  // recast reduction coefficient at full reuse
)

// Solo: the Assassin's self-buffs only (no group buffs). The self-haste buff is
// temporary, so it's excluded from this sustained baseline. Best-guess; confirm.
var Solo = model.StatBlock{
	MultiAttack: 34.2, // Villainy IV
}

// Raid: self + group package. DPS-mod buff-capped at 200; crit elevated by
// AAs/buffs. Haste still low (no maintained group haste buff). Confirm vs parse.
var Raid = model.StatBlock{
	MultiAttack: 34.2,  // Villainy IV
	DPSMod:      200.0, // buff-capped (guild-leader reported)
	CritChance:  31.0,  // ~31% buffed in an MT group (research; confirm)
}
