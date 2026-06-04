package bis

import (
	"strings"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// SlotCandidates groups items by their census slot, dropping any that fail keep,
// then overrides the off-hand slot with every one-handed weapon that passes keep
// — except Soulfire weapons, since the single Soulfire is the fixed main-hand and
// the player never owns a second one. The main-hand (Primary) bucket is left as-is;
// BuildSet ignores it (the main is fixed).
func SlotCandidates(items []store.ScorableItem, keep func(store.ScorableItem) bool) map[string][]store.ScorableItem {
	bySlot := map[string][]store.ScorableItem{}
	var oneHanders []store.ScorableItem
	for _, it := range items {
		if !keep(it) {
			continue
		}
		bySlot[it.Slot] = append(bySlot[it.Slot], it)
		if it.WieldStyle == "One-Handed" && !strings.HasPrefix(it.Name, "Soulfire") {
			oneHanders = append(oneHanders, it)
		}
	}
	bySlot[offHandSlot] = oneHanders
	return bySlot
}
