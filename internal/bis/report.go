package bis

import (
	"sort"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

// ScoredItem is one candidate with its in-context ΔDPS and explainable breakdown.
type ScoredItem struct {
	Item  store.ScorableItem
	Delta float64
	Terms []model.ScoreTerm
}

// SlotReport is one slot's converged pick plus ranked alternatives by tier.
type SlotReport struct {
	Slot      string
	Chosen    []store.ScorableItem
	Mythical  []ScoredItem
	Fabled    []ScoredItem
	Legendary []ScoredItem
}

// ConvergedWeights derives the stat weights at the converged set's full baseline,
// against the converged off-hand — the weights used for explainable breakdowns.
func ConvergedWeights(set *Set) map[string]float64 {
	base := set.restBase("")
	off := set.offWeapon()
	dps := func(sb model.StatBlock) float64 {
		return model.TotalDPSDual(sb, set.Main, off, set.Arts)
	}
	return model.DeriveWeights(base, dps)
}

// SlotCandidatesScored ranks a slot's candidates by in-context ΔDPS (against the
// converged set with that slot emptied), attaching the weight×stat breakdown.
func SlotCandidatesScored(set *Set, slot string, cands []store.ScorableItem, weights map[string]float64) []ScoredItem {
	out := make([]ScoredItem, 0, len(cands))
	for _, c := range cands {
		_, terms := model.ScoreItem(weights, c.Stats)
		out = append(out, ScoredItem{Item: c, Delta: set.CandidateDelta(slot, c), Terms: terms})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Delta > out[j].Delta })
	return out
}

func topByTier(scored []ScoredItem, tier string, n int) []ScoredItem {
	var f []ScoredItem
	for _, s := range scored {
		if s.Item.Tier == tier {
			f = append(f, s)
		}
	}
	if n >= 0 && len(f) > n {
		f = f[:n]
	}
	return f
}

// BuildSlotReports produces one SlotReport per slot (sorted), each with the
// converged pick and top-n Fabled/Legendary + all Mythical alternatives.
func BuildSlotReports(set *Set, bySlot map[string][]store.ScorableItem, weights map[string]float64, n int) []SlotReport {
	slots := make([]string, 0, len(bySlot))
	for slot := range bySlot {
		slots = append(slots, slot)
	}
	sort.Strings(slots)

	reports := make([]SlotReport, 0, len(slots))
	for _, slot := range slots {
		scored := SlotCandidatesScored(set, slot, bySlot[slot], weights)
		reports = append(reports, SlotReport{
			Slot:      slot,
			Chosen:    set.Equipped[slot],
			Mythical:  topByTier(scored, "MYTHICAL", -1),
			Fabled:    topByTier(scored, "FABLED", n),
			Legendary: topByTier(scored, "LEGENDARY", n),
		})
	}
	return reports
}
