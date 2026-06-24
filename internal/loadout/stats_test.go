package loadout

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/catalog"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
	"github.com/stretchr/testify/require"
)

func TestItemStatGrants_CombinesModifiersAndEffects(t *testing.T) {
	it := census.Item{
		Modifiers: map[string]census.Modifier{"attackspeed": {Value: 10}},
		EffectList: []census.Effect{
			{Description: "When Equipped:", Indentation: 0},
			{Description: "Increases Haste of caster by 25.0.", Indentation: 1},
		},
	}

	got := ItemStatGrants(it)

	require.Equal(t, 35.0, got["attackspeed"])
}

func TestItemStatGrants_SeparateKeys(t *testing.T) {
	it := census.Item{
		Modifiers: map[string]census.Modifier{"strength": {Value: 40}},
		EffectList: []census.Effect{
			{Description: "When Equipped:", Indentation: 0},
			{Description: "Increases Crit Chance of caster by 5.0.", Indentation: 1},
		},
	}

	got := ItemStatGrants(it)

	require.Equal(t, 40.0, got["strength"])
	require.Equal(t, 5.0, got["critchance"])
}

func TestItemStatGrants_Empty(t *testing.T) {
	require.Empty(t, ItemStatGrants(census.Item{}))
}

func TestMergeEffectStats(t *testing.T) {
	items := []census.Item{
		{ID: 101, Modifiers: map[string]census.Modifier{"attackspeed": {Value: 10}}},
	}
	effects := []catalog.EffectStat{
		{ItemID: 101, Stat: "attackspeed", Value: 25},
		{ItemID: 101, Stat: "critchance", Value: 2},
		{ItemID: 999, Stat: "flurry", Value: 5},
	}

	merged := MergeEffectStats(items, effects)

	require.Len(t, merged, 1)
	require.InDelta(t, 35, merged[0].Modifiers["attackspeed"].Value, 1e-9)
	require.InDelta(t, 2, merged[0].Modifiers["critchance"].Value, 1e-9)
	_, hasFlurry := merged[0].Modifiers["flurry"]
	require.False(t, hasFlurry, "id-999 effect must be ignored")
}
