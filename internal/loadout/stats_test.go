package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func modItem(mods map[string]float64) census.Item {
	m := map[string]census.Modifier{}
	for k, v := range mods {
		m[k] = census.Modifier{Value: v}
	}
	return census.Item{Modifiers: m}
}

func TestItemStatBlockRoutesEffectHaste(t *testing.T) {
	it := modItem(map[string]float64{"attackspeed": 7})
	it.EffectList = []census.Effect{
		{Description: "When Equipped:", Indentation: 0},
		{Description: "Increases Haste of caster by 25.0.", Indentation: 1},
	}
	sb := ItemStatBlock(it, map[string]float64{"critchance": 2})
	require.InDelta(t, 7, sb.Haste, 1e-9)
	require.InDelta(t, 25, sb.HasteEffect, 1e-9)
	require.InDelta(t, 2, sb.CritChance, 1e-9)
}

func TestItemStatBlockCatalogedHaste(t *testing.T) {
	it := modItem(nil)
	sb := ItemStatBlock(it, map[string]float64{"attackspeed": 25})
	require.InDelta(t, 0, sb.Haste, 1e-9)
	require.InDelta(t, 25, sb.HasteEffect, 1e-9)
}
