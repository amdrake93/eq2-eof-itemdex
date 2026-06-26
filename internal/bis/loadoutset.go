package bis

import (
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

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

// UpgradeOption is one candidate upgrade for a slot instance: its catalog id (for
// the EQ2U link), name, and in-context ΔDPS over the worn item it would replace.
type UpgradeOption struct {
	ID    int
	Name  string
	Delta float64
}

// SlotUpgrade is one physical slot instance: the worn item (name/id/slot-DPS
// value, or "Empty" with id 0 and value 0), its best upgrade, and an optional
// second-best alternative.
type SlotUpgrade struct {
	Slot          string
	EquippedName  string
	EquippedID    int
	EquippedValue float64
	Best          UpgradeOption
	Alt           *UpgradeOption // nil when the instance has only one positive upgrade
}

// RankSlotUpgrades returns one row per physical slot instance across the
// optimizable slots. Two-capacity slots (capacityOf) emit a row per worn item;
// any unfilled position becomes a synthetic "Empty" row (value 0). Each row's
// candidates are ranked by ReplaceInstanceDelta — the gain of swapping that one
// worn item (or filling the Empty) while the slot's other instance is held fixed
// — keeping the best plus an optional second-best alternative. Rows are ranked
// across all instances by the best candidate's ΔDPS and limited to topN rows
// (topN <= 0 = no limit). Instances with no positive upgrade are omitted.
func RankSlotUpgrades(set *Set, bySlot map[string][]store.ScorableItem, optimizable map[string]bool, topN int) []SlotUpgrade {
	var out []SlotUpgrade
	for slot := range optimizable {
		worn := set.Equipped[slot]
		for idx := 0; idx < capacityOf(slot); idx++ {
			su := SlotUpgrade{Slot: slot}
			replaceIdx := idx
			if idx < len(worn) {
				su.EquippedName = worn[idx].Name
				su.EquippedID = worn[idx].ID
				su.EquippedValue = set.EquippedInstanceValue(slot, idx)
			} else {
				su.EquippedName = "Empty"
				replaceIdx = -1
			}

			var cands []UpgradeOption
			for _, c := range bySlot[slot] {
				if d := set.ReplaceInstanceDelta(slot, replaceIdx, c); d > 0 {
					cands = append(cands, UpgradeOption{ID: c.ID, Name: c.Name, Delta: d})
				}
			}
			if len(cands) == 0 {
				// Single-capacity slots stay actionable: omit when there's no upgrade.
				// Multi-capacity slots always show every worn instance (both rings/
				// ears/wrists), even with no upgrade — Best stays the zero-value
				// "no upgrade" sentinel and sorts last.
				if capacityOf(slot) <= 1 || idx >= len(worn) {
					continue
				}
				out = append(out, su)
				continue
			}
			sort.Slice(cands, func(i, j int) bool { return cands[i].Delta > cands[j].Delta })
			su.Best = cands[0]
			if len(cands) > 1 {
				alt := cands[1]
				su.Alt = &alt
			}
			out = append(out, su)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Best.Delta > out[j].Best.Delta })
	if topN > 0 && len(out) > topN {
		out = out[:topN]
	}
	return out
}

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
