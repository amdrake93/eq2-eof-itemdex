package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// optimizableCatalogSlots are catalog (census item slot_list) slot names that have
// a candidate pool — the slots bis can suggest upgrades for. Ranged/Ammo/Charm/
// event carry stats on import but are never swap candidates (SPEC §7, §16).
var optimizableCatalogSlots = map[string]bool{
	"Secondary": true, "Head": true, "Chest": true,
	"Shoulder": true, "Shoulders": true, "Forearms": true, "Hand": true, "Hands": true,
	"Leg": true, "Legs": true, "Foot": true, "Feet": true, "Finger": true,
	"Ear": true, "Wrist": true, "Neck": true, "Back": true, "Waist": true,
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
