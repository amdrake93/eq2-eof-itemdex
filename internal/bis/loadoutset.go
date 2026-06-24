package bis

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// UpgradeDelta is the DPS gain from swapping candidate c into slot, REPLACING the
// weakest item currently equipped there. It differs from CandidateDelta, which
// measures a candidate against an EMPTY slot (its standalone contribution): for the
// "what to upgrade next" report on an already-equipped loadout, the meaningful
// number is the gain over what's worn, so we subtract the contribution of the
// equipped item the candidate would displace. An empty slot falls back to the full
// standalone contribution.
func (s *Set) UpgradeDelta(slot string, c store.ScorableItem) float64 {
	equipped := s.Equipped[slot]
	if len(equipped) == 0 {
		return s.CandidateDelta(slot, c)
	}
	weakest := math.Inf(1)
	for _, it := range equipped {
		if d := s.CandidateDelta(slot, it); d < weakest {
			weakest = d
		}
	}
	return s.CandidateDelta(slot, c) - weakest
}

// optimizableCatalogSlots are catalog (census item slot_list) slot names that have
// a candidate pool — the slots bis can suggest upgrades for. Ranged/Ammo/Charm/
// event carry stats on import but are never swap candidates (SPEC §7, §16).
// Slot names are the census ITEM slot_list vocabulary, verified against the live
// catalog (bis.db items.slot): Cloak (not "Back"), plural Shoulders/Hands/Legs/Feet.
// Charm, Ranged, Ammo and Primary (the fixed main-hand) are intentionally excluded.
var optimizableCatalogSlots = map[string]bool{
	"Secondary": true, "Head": true, "Chest": true, "Shoulders": true,
	"Forearms": true, "Hands": true, "Legs": true, "Feet": true,
	"Finger": true, "Ear": true, "Wrist": true, "Neck": true,
	"Cloak": true, "Waist": true,
}

// OptimizableSlot reports whether a catalog slot is a bis swap candidate.
func OptimizableSlot(catalogSlot string) bool { return optimizableCatalogSlots[catalogSlot] }

// SetFromLoadout builds a Set with every kept slot's stats equipped (fixed +
// optimizable alike) and returns the set of catalog slots eligible for
// re-optimization. The Primary weapon entry overrides lo.Main; the Secondary
// weapon entry lands in Equipped["Secondary"] so Set.DPS()'s off-hand picks it up.
func SetFromLoadout(f loadout.File, profile model.StatBlock, lo store.Loadout, autoMult, fightLen float64) (*Set, map[string]bool) {
	set := NewSet(profile, lo, autoMult, fightLen)
	optimizable := map[string]bool{}
	for _, e := range f.Slots {
		it := store.ScorableItem{
			ID:    int(e.ItemID),
			Name:  e.Name,
			Slot:  e.CatalogSlot,
			Stats: e.Stats,
		}
		if e.WeaponDelay > 0 {
			it.WeaponMin = e.WeaponMin
			it.WeaponMax = e.WeaponMax
			it.WeaponAvg = (e.WeaponMin + e.WeaponMax) / 2
			it.WeaponDelay = e.WeaponDelay
		}
		set.Equipped[e.CatalogSlot] = append(set.Equipped[e.CatalogSlot], it)
		if e.Optimizable {
			optimizable[e.CatalogSlot] = true
		}
		if e.CatalogSlot == "Primary" && it.IsWeapon() {
			set.Main = model.Weapon{AvgDamage: it.WeaponAvg, MinDamage: it.WeaponMin, MaxDamage: it.WeaponMax, DelaySecs: it.WeaponDelay}
		}
	}
	return set, optimizable
}
