package bis

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// SlotCandidates groups items by census slot (dropping any that fail keep), then
// overrides the weapon slots from the class weapon config: the main-hand (Primary)
// pool is every weapon whose WieldStyle is allowed, and — when dualWield — the
// off-hand (Secondary) pool is the same one-handed set. The single-physical-weapon
// reality is enforced by the optimizer's no-duplicate rule, not by pool exclusion.
func SlotCandidates(items []store.ScorableItem, keep func(store.ScorableItem) bool, wieldStyles []string, dualWield bool) map[string][]store.ScorableItem {
	allowed := map[string]bool{}
	for _, ws := range wieldStyles {
		allowed[ws] = true
	}

	bySlot := map[string][]store.ScorableItem{}
	var weapons []store.ScorableItem
	for _, it := range items {
		if !keep(it) {
			continue
		}
		if it.Slot != mainHandSlot {
			bySlot[it.Slot] = append(bySlot[it.Slot], it)
		}
		if allowed[it.WieldStyle] {
			weapons = append(weapons, it)
		}
	}

	bySlot[mainHandSlot] = weapons
	if dualWield {
		bySlot[offHandSlot] = weapons
	} else {
		delete(bySlot, offHandSlot)
	}
	return bySlot
}
