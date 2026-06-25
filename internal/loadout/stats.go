package loadout

import (
	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
)

// ItemStatBlock builds an item's StatBlock from its modifiers block plus its effect
// grants, routing the non-stacking "Haste" item effect (effect-source attackspeed)
// into HasteEffect rather than the additive Haste field (spec §11). Effect grants
// come from the item's effect_list (freshly-fetched items) and/or extraEffectStats
// (cataloged items, whose grants are persisted in item-effects.csv). All other
// effect stats fold normally. census attackspeed key == haste.
func ItemStatBlock(it census.Item, extraEffectStats map[string]float64) model.StatBlock {
	var sb model.StatBlock

	mods := map[string]float64{}
	for k, m := range it.Modifiers {
		mods[k] += m.Value
	}
	sb.AddModifiers(mods)

	effects := map[string]float64{}
	parsed, _, _ := catalog.ParseEffects(it.EffectList)
	for k, v := range parsed {
		effects[k] += v
	}
	for k, v := range extraEffectStats {
		effects[k] += v
	}
	for k, v := range effects {
		if k == "attackspeed" {
			if v > sb.HasteEffect {
				sb.HasteEffect = v
			}
			continue
		}
		sb.AddModifiers(map[string]float64{k: v})
	}
	return sb
}
