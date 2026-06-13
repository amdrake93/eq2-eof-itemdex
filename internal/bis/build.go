package bis

import (
	"math"
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
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

// mainHandSlot is the fixed main-hand slot (Soulfire); it is not optimized.
const mainHandSlot = "Primary"

func sameItems(a, b []store.ScorableItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// BuildSet runs coordinate ascent: each pass fills every optimizable slot with
// the DPS-maximizing pick at the current set state; passes repeat until no slot
// changes (converged) or maxPasses is hit. Locked slots are pre-filled and never
// re-optimized; the main-hand slot is fixed to the loadout and excluded.
func BuildSet(profile model.StatBlock, lo store.Loadout, bySlot, locked map[string][]store.ScorableItem, maxPasses int, autoMult float64) *Set {
	set := NewSet(profile, lo, autoMult)
	lockedSlot := map[string]bool{}
	for slot, items := range locked {
		set.Equipped[slot] = items
		lockedSlot[slot] = true
	}
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		if slot == mainHandSlot || lockedSlot[slot] {
			continue
		}
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for _, slot := range slots {
			best := pickBest(set, slot, bySlot[slot])
			if !sameItems(best, set.Equipped[slot]) {
				set.Equipped[slot] = best
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return set
}
