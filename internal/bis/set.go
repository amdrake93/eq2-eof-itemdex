// Package bis builds the Assassin's best-in-slot gear set by iterating to
// convergence and renders the explainable report.
package bis

import (
	"sort"

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
	Arts     []spell.CombatArt
	AutoMult float64 // class-intrinsic auto-attack multiplier (classes/<class>.toml)
	FightLen float64 // target fight length in seconds (smoothed CADPS window center)
	Equipped map[string][]store.ScorableItem
}

// NewSet returns an empty set seeded with the profile baseline and loadout.
func NewSet(profile model.StatBlock, lo store.Loadout, autoMult, fightLen float64) *Set {
	return &Set{Profile: profile, Arts: lo.Arts, AutoMult: autoMult, FightLen: fightLen, Equipped: map[string][]store.ScorableItem{}}
}

// restBase is the set's StatBlock with one slot's items excluded (exclude=""
// includes everything). Slots are summed in sorted order so the float
// accumulation is deterministic (float addition isn't associative) — without
// this, map iteration order makes Set.DPS() wobble in its low bits, flipping
// pickBest near-ties and producing non-reproducible reports.
func (s *Set) restBase(exclude string) model.StatBlock {
	slots := make([]string, 0, len(s.Equipped))
	for slot := range s.Equipped {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	b := s.Profile
	for _, slot := range slots {
		if slot == exclude {
			continue
		}
		for _, it := range s.Equipped[slot] {
			b = b.Add(it.Stats)
		}
	}
	return b
}

// weaponFrom returns the first equippable weapon in items (zero Weapon if none).
func weaponFrom(items []store.ScorableItem) model.Weapon {
	for _, it := range items {
		if it.IsWeapon() {
			return model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
		}
	}
	return model.Weapon{}
}

// mainWeapon is the equipped main-hand weapon (zero Weapon if none), derived from
// Equipped["Primary"] — the mirror of offWeapon()/Equipped["Secondary"].
func (s *Set) mainWeapon() model.Weapon { return weaponFrom(s.Equipped[mainHandSlot]) }

// offWeapon is the equipped off-hand weapon (zero Weapon if none).
func (s *Set) offWeapon() model.Weapon { return weaponFrom(s.Equipped[offHandSlot]) }

// restMain is the main-hand weapon excluding a slot (zero if the slot IS the main-hand).
func (s *Set) restMain(exclude string) model.Weapon {
	if exclude == mainHandSlot {
		return model.Weapon{}
	}
	return s.mainWeapon()
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
	return model.TotalDPSDual(s.restBase(""), s.mainWeapon(), s.offWeapon(), s.Arts, s.AutoMult, s.FightLen)
}

// slotDPS computes full-set DPS with `slot`'s equipped items REPLACED by `items`,
// every other slot held fixed. For the off-hand slot the off-hand weapon is
// re-derived from `items`; otherwise the current off-hand is kept.
func (s *Set) slotDPS(slot string, items []store.ScorableItem) float64 {
	rb := s.restBase(slot)
	for _, it := range items {
		rb = rb.Add(it.Stats)
	}
	main := s.mainWeapon()
	if slot == mainHandSlot {
		main = weaponFrom(items)
	}
	off := s.offWeapon()
	if slot == offHandSlot {
		off = weaponFrom(items)
	}
	return model.TotalDPSDual(rb, main, off, s.Arts, s.AutoMult, s.FightLen)
}

// ReplaceInstanceDelta is the ΔDPS of swapping the worn item at index `idx` in
// `slot` for candidate `c`, holding every other equipped item — including the
// slot's other instance(s) — fixed. idx == -1 fills an empty position (appends c,
// replacing nothing).
func (s *Set) ReplaceInstanceDelta(slot string, idx int, c store.ScorableItem) float64 {
	cur := s.Equipped[slot]
	swapped := make([]store.ScorableItem, 0, len(cur)+1)
	for i, it := range cur {
		if i == idx {
			continue
		}
		swapped = append(swapped, it)
	}
	swapped = append(swapped, c)
	return s.slotDPS(slot, swapped) - s.DPS()
}

// EquippedInstanceValue is the marginal slot-DPS contribution of the worn item at
// index `idx` in `slot`: current DPS minus DPS with that item removed (others fixed).
func (s *Set) EquippedInstanceValue(slot string, idx int) float64 {
	cur := s.Equipped[slot]
	without := make([]store.ScorableItem, 0, len(cur))
	for i, it := range cur {
		if i == idx {
			continue
		}
		without = append(without, it)
	}
	return s.DPS() - s.slotDPS(slot, without)
}

// CandidateDelta is the in-context ΔDPS of putting a candidate in a slot, given
// the rest of the (otherwise-fixed) set with that slot emptied.
func (s *Set) CandidateDelta(slot string, c store.ScorableItem) float64 {
	rb := s.restBase(slot)
	var newMain, newOff *model.Weapon
	if c.IsWeapon() {
		w := model.Weapon{AvgDamage: c.WeaponAvg, MinDamage: c.WeaponMin, MaxDamage: c.WeaponMax, DelaySecs: c.WeaponDelay}
		switch slot {
		case mainHandSlot:
			newMain = &w
		case offHandSlot:
			newOff = &w
		}
	}
	return model.ItemDelta(rb, s.restMain(slot), s.restOff(slot), s.Arts, c.Stats, newMain, newOff, s.AutoMult, s.FightLen)
}
