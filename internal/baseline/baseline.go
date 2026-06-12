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
	HasteStatCap      = constants.HasteStatCap
	DPSModCap         = constants.DPSModCap
	AbilityModCapFrac = constants.AbilityModCapFrac
	ReuseHalvesAt     = constants.ReuseHalvesAt
	ReuseHalveCoeff   = constants.ReuseHalveCoeff
)

// Solo: the Assassin's self-buffs only (no group buffs). The self-haste buff is
// temporary, so it's excluded from this sustained baseline. Best-guess; confirm.
var Solo = model.StatBlock{
	MultiAttack: 34.2, // Villainy IV
}

// Raid: self + group package. Group DPS-mod measured live (2026-06): Coercer 74
// + Inquisitor 30.2 + Dirge 10 = 114.2 — mid-curve, well below the 300 cap (the
// old "buffs reach the cap → 200" assumption died with the 200 cap and the comp
// losing its Berserker). Crit elevated by AAs/buffs. Haste still low (no
// maintained group haste buff). Refine per component as readings firm up.
var Raid = model.StatBlock{
	MultiAttack: 34.2,  // Villainy IV
	DPSMod:      114.2, // coercer 74 + inquis 30.2 + dirge 10 (live estimate)
	CritChance:  31.0,  // ~31% buffed in an MT group (research; confirm)
}
