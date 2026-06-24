package loadout

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/census"
)

func itemStats(mods map[string]float64) census.Item {
	m := map[string]census.Modifier{}
	for k, v := range mods {
		m[k] = census.Modifier{Value: v}
	}
	return census.Item{Modifiers: m}
}

func TestResolveSumsItemAndAdornments(t *testing.T) {
	ch := census.Character{
		DisplayName: "Biffels (Wuoshi)",
		LastUpdate:  123,
		EquipmentSlots: []census.EquipmentSlot{
			{Name: "cloak", Item: census.EquippedItem{ID: 264598753,
				Adornments: []census.Adornment{{Color: "white"}, {ID: 111}}}},
			{Name: "food", Item: census.EquippedItem{ID: 461060541}},
			{Name: "mount_armor", Item: census.EquippedItem{}},
		},
	}
	catalog := func(id int64) (census.Item, bool) {
		if id == 264598753 {
			it := itemStats(map[string]float64{"attackspeed": 25})
			it.ID = id
			it.DisplayName = "Cloak of Flames"
			it.Slots = []census.Slot{{Name: "Back"}}
			return it, true
		}
		return census.Item{}, false
	}
	adorn := func(id int64) (map[string]float64, bool) {
		if id == 111 {
			return map[string]float64{"critchance": 2}, true
		}
		return nil, false
	}
	optimizable := func(catalogSlot string) bool { return catalogSlot == "Back" }

	f, missItems, missAdorns := Resolve(ch, catalog, adorn, optimizable)

	require.Empty(t, missItems)
	require.Empty(t, missAdorns)
	require.Equal(t, "Biffels (Wuoshi)", f.CharacterName)
	require.Len(t, f.Slots, 1, "food + mount_armor skipped")
	cloak := f.Slots[0]
	require.Equal(t, "Back", cloak.CatalogSlot)
	require.Equal(t, "cloak", cloak.CharSlot)
	require.True(t, cloak.Optimizable)
	require.InDelta(t, 25, cloak.Stats.Haste, 1e-9)
	require.InDelta(t, 2, cloak.Stats.CritChance, 1e-9)
}

func TestResolveReportsMissing(t *testing.T) {
	ch := census.Character{EquipmentSlots: []census.EquipmentSlot{
		{Name: "head", Item: census.EquippedItem{ID: 999, Adornments: []census.Adornment{{ID: 888}}}},
	}}
	none := func(id int64) (census.Item, bool) { return census.Item{}, false }
	noneA := func(id int64) (map[string]float64, bool) { return nil, false }
	f, missItems, missAdorns := Resolve(ch, none, noneA, func(string) bool { return true })

	require.Equal(t, []int64{999}, missItems)
	require.Equal(t, []int64{888}, missAdorns)
	require.Empty(t, f.Slots, "unresolved item produces no slot entry")
}
