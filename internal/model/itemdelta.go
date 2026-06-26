package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/spell"

// ItemDelta is the ΔDPS of equipping an item into an otherwise-fixed set.
// restBase is the set's StatBlock with the target slot empty; restMain/restOff are
// the main-hand / off-hand weapons with the target slot empty (zero Weapon when the
// target IS that weapon slot, or none is equipped). itemStats is the candidate's
// stats; for a weapon candidate pass its weapon as newMain (main-hand slot) or
// newOff (off-hand slot), else nil.
//
// It diffs full TotalDPSDual evaluations at the set's live stat totals, so a
// stat already at its cap in restBase contributes ~0 and the multiplicative
// auto cluster (haste·MA·crit·flurry·dps-mod) makes a stat worth more as the
// set accumulates its partners.
func ItemDelta(restBase StatBlock, restMain, restOff Weapon, arts []spell.CombatArt, itemStats StatBlock, newMain, newOff *Weapon, classAutoMult, fightLen float64) float64 {
	before := TotalDPSDual(restBase, restMain, restOff, arts, classAutoMult, fightLen)
	main := restMain
	if newMain != nil {
		main = *newMain
	}
	off := restOff
	if newOff != nil {
		off = *newOff
	}
	after := TotalDPSDual(restBase.Add(itemStats), main, off, arts, classAutoMult, fightLen)
	return after - before
}
