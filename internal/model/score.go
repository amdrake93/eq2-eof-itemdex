package model

import "sort"

// ScoreTerm is one stat's linear contribution: itemValue × weight.
type ScoreTerm struct {
	Stat         string
	ItemValue    float64
	Weight       float64
	Contribution float64
}

// ScoreItem returns Σ(weight × itemStat) over WeightStats and the per-stat
// breakdown (nonzero stats only) sorted by contribution descending. Used for
// the report's explainable breakdown, not for set selection (that uses ItemDelta).
func ScoreItem(weights map[string]float64, item StatBlock) (total float64, terms []ScoreTerm) {
	for _, s := range WeightStats {
		v := getStat(item, s)
		if v == 0 {
			continue
		}
		w := weights[s]
		c := w * v
		terms = append(terms, ScoreTerm{Stat: s, ItemValue: v, Weight: w, Contribution: c})
		total += c
	}
	sort.Slice(terms, func(i, j int) bool { return terms[i].Contribution > terms[j].Contribution })
	return total, terms
}
