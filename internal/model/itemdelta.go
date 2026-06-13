package model

import "github.com/amdrake93/eq2-eof-itemdex/internal/spell"

// ItemDelta is the ΔDPS of equipping an item into an otherwise-fixed set.
// restBase is the set's StatBlock with the target slot empty; restOff is the
// off-hand weapon when that slot is empty (zero Weapon if the target IS the
// off-hand slot, or no off-hand is equipped). itemStats is the candidate's
// stats; for an off-hand weapon candidate pass its weapon as newOff (else nil).
//
// It diffs full TotalDPSDual evaluations at the set's live stat totals, so a
// stat already at its cap in restBase contributes ~0 and the multiplicative
// auto cluster (haste·MA·crit·flurry·dps-mod) makes a stat worth more as the
// set accumulates its partners.
func ItemDelta(restBase StatBlock, main, restOff Weapon, arts []spell.CombatArt, itemStats StatBlock, newOff *Weapon, classAutoMult, fightLen float64) float64 {
	before := TotalDPSDual(restBase, main, restOff, arts, classAutoMult, fightLen)
	off := restOff
	if newOff != nil {
		off = *newOff
	}
	after := TotalDPSDual(restBase.Add(itemStats), main, off, arts, classAutoMult, fightLen)
	return after - before
}
