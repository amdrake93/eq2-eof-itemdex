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
