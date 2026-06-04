package bis

import (
	"math"

	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// slotCapacity is how many items a census slot equips; unlisted slots hold 1.
var slotCapacity = map[string]int{
	"Ear": 2, "Finger": 2, "Wrist": 2, "Charm": 2,
}

func capacityOf(slot string) int {
	if n, ok := slotCapacity[slot]; ok {
		return n
	}
	return 1
}

// pickBest greedily chooses up to capacityOf(slot) distinct candidates that
// maximize the full set DPS, each addition evaluated in the context of those
// already chosen (so within-slot caps/interactions are respected). It restores
// the slot's original contents before returning.
func pickBest(set *Set, slot string, cands []store.ScorableItem) []store.ScorableItem {
	orig := set.Equipped[slot]
	defer func() { set.Equipped[slot] = orig }()

	capN := capacityOf(slot)
	chosen := []store.ScorableItem{}
	used := map[int]bool{}
	for len(chosen) < capN {
		bestIdx, bestDPS := -1, math.Inf(-1)
		for i, c := range cands {
			if used[c.ID] {
				continue
			}
			set.Equipped[slot] = append(append([]store.ScorableItem{}, chosen...), c)
			if d := set.DPS(); d > bestDPS {
				bestDPS, bestIdx = d, i
			}
		}
		if bestIdx < 0 {
			break
		}
		chosen = append(chosen, cands[bestIdx])
		used[cands[bestIdx].ID] = true
	}
	return chosen
}
