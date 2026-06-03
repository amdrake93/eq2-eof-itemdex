package baseline

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/constants"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// Re-export combat constants from internal/constants so callers that already
// import baseline can continue to reference e.g. baseline.CritMultiplier.
const (
	CritMultiplier    = constants.CritMultiplier
	FlurryMultiplier  = constants.FlurryMultiplier
	HasteCapPct       = constants.HasteCapPct
	HasteToFlurry     = constants.HasteToFlurry
	DPSModCap         = constants.DPSModCap
	DPSModEffectAtCap = constants.DPSModEffectAtCap
	AbilityModCapFrac = constants.AbilityModCapFrac
	ReuseHalvesAt     = constants.ReuseHalvesAt
	ReuseHalveCoeff   = constants.ReuseHalveCoeff
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
