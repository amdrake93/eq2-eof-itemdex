// Package bis builds the Assassin's best-in-slot gear set by iterating to
// convergence and renders the explainable report.
package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// offHandSlot is the census slot name for the off-hand weapon.
const offHandSlot = "Secondary"

// Set is a candidate gear set: a profile baseline + fixed main-hand/arts + the
// items chosen per slot. DPS is always recomputed from the full set so caps and
// interactions are exact.
type Set struct {
	Profile  model.StatBlock
	Main     model.Weapon
	Arts     []spell.CombatArt
	AutoMult float64 // class-intrinsic auto-attack multiplier (classes/<class>.toml)
	Equipped map[string][]store.ScorableItem
}

// NewSet returns an empty set seeded with the profile baseline and loadout.
func NewSet(profile model.StatBlock, lo store.Loadout, autoMult float64) *Set {
	return &Set{Profile: profile, Main: lo.Main, Arts: lo.Arts, AutoMult: autoMult, Equipped: map[string][]store.ScorableItem{}}
}

// restBase is the set's StatBlock with one slot's items excluded (exclude=""
// includes everything).
func (s *Set) restBase(exclude string) model.StatBlock {
	b := s.Profile
	for slot, items := range s.Equipped {
		if slot == exclude {
			continue
		}
		for _, it := range items {
			b = b.Add(it.Stats)
		}
	}
	return b
}

// offWeapon is the equipped off-hand weapon (zero Weapon if none).
func (s *Set) offWeapon() model.Weapon {
	for _, it := range s.Equipped[offHandSlot] {
		if it.IsWeapon() {
			return model.Weapon{AvgDamage: it.WeaponAvg, DelaySecs: it.WeaponDelay}
		}
	}
	return model.Weapon{}
}

// restOff is the off-hand weapon excluding a slot (zero if the slot IS the off-hand).
func (s *Set) restOff(exclude string) model.Weapon {
	if exclude == offHandSlot {
		return model.Weapon{}
	}
	return s.offWeapon()
}

// DPS is the full set's modeled TotalDPS.
func (s *Set) DPS() float64 {
	return model.TotalDPSDual(s.restBase(""), s.Main, s.offWeapon(), s.Arts, s.AutoMult)
}

// CandidateDelta is the in-context ΔDPS of putting a candidate in a slot, given
// the rest of the (otherwise-fixed) set with that slot emptied.
func (s *Set) CandidateDelta(slot string, c store.ScorableItem) float64 {
	rb := s.restBase(slot)
	ro := s.restOff(slot)
	if slot == offHandSlot && c.IsWeapon() {
		w := model.Weapon{AvgDamage: c.WeaponAvg, DelaySecs: c.WeaponDelay}
		return model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, &w, s.AutoMult)
	}
	return model.ItemDelta(rb, s.Main, ro, s.Arts, c.Stats, nil, s.AutoMult)
}
