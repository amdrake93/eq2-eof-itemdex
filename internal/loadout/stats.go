package loadout

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

// ItemStatGrants returns an item's census-stat-key -> value grants, summing its
// modifiers block with the static stat grants parsed from its effect_list (the
// "When Equipped: Increases <stat> of caster by N" lines). Triggered procs and
// percent-unit effects are not stat grants and are excluded.
func ItemStatGrants(it census.Item) map[string]float64 {
	grants := map[string]float64{}
	for k, m := range it.Modifiers {
		grants[k] += m.Value
	}
	effectStats, _, _ := catalog.ParseEffects(it.EffectList)
	for k, v := range effectStats {
		grants[k] += v
	}
	return grants
}

// MergeEffectStats returns copies of items with each matching effect stat folded
// into the item's Modifiers (by census stat key). Cached items load with an empty
// EffectList, so their "When Equipped" grants — persisted separately in
// item-effects.csv — must be merged back in here. Effects whose ItemID matches no
// item are ignored.
func MergeEffectStats(items []census.Item, effects []catalog.EffectStat) []census.Item {
	byID := make(map[int64]int, len(items))
	out := make([]census.Item, len(items))
	for i, it := range items {
		mods := make(map[string]census.Modifier, len(it.Modifiers))
		for k, m := range it.Modifiers {
			mods[k] = m
		}
		it.Modifiers = mods
		out[i] = it
		byID[it.ID] = i
	}

	for _, es := range effects {
		i, ok := byID[int64(es.ItemID)]
		if !ok {
			continue
		}
		mods := out[i].Modifiers
		existing := mods[es.Stat]
		existing.Value += es.Value
		mods[es.Stat] = existing
	}

	return out
}
