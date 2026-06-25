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

func TestResolveItemStats(t *testing.T) {
	ch := census.Character{
		DisplayName: "Biffels (Wuoshi)",
		LastUpdate:  123,
		EquipmentSlots: []census.EquipmentSlot{
			{Name: "cloak", Item: census.EquippedItem{ID: 264598753}},
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
	optimizable := func(catalogSlot string) bool { return catalogSlot == "Back" }

	f, missItems := Resolve(ch, catalog, func(int64) map[string]float64 { return nil }, optimizable)

	require.Empty(t, missItems)
	require.Equal(t, "Biffels (Wuoshi)", f.CharacterName)
	require.Len(t, f.Slots, 1, "food + mount_armor skipped")
	cloak := f.Slots[0]
	require.Equal(t, "Back", cloak.CatalogSlot)
	require.Equal(t, "cloak", cloak.CharSlot)
	require.True(t, cloak.Optimizable)
	require.InDelta(t, 25, cloak.Stats.Haste, 1e-9)
}

func TestResolveOffHandWeaponGetsSecondarySlot(t *testing.T) {
	weaponSlots := []census.Slot{{Name: "Primary"}, {Name: "Secondary"}}
	weaponType := census.TypeInfo{Delay: 3, MinBaseDamage: 50, MaxBaseDamage: 100}
	ch := census.Character{
		EquipmentSlots: []census.EquipmentSlot{
			{Name: "primary", Item: census.EquippedItem{ID: 6}},
			{Name: "secondary", Item: census.EquippedItem{ID: 7}},
		},
	}
	catalog := func(id int64) (census.Item, bool) {
		if id == 6 || id == 7 {
			return census.Item{ID: id, Slots: weaponSlots, TypeInfo: weaponType}, true
		}
		return census.Item{}, false
	}
	f, missItems := Resolve(ch, catalog, func(int64) map[string]float64 { return nil }, func(string) bool { return true })

	require.Empty(t, missItems)
	require.Len(t, f.Slots, 2)

	bySlot := map[string]SlotEntry{}
	for _, e := range f.Slots {
		bySlot[e.CharSlot] = e
	}
	main := bySlot["primary"]
	require.Equal(t, "Primary", main.CatalogSlot)
	require.InDelta(t, 3, main.WeaponDelay, 1e-9)

	off := bySlot["secondary"]
	require.Equal(t, "Secondary", off.CatalogSlot)
	require.InDelta(t, 3, off.WeaponDelay, 1e-9)
}

func TestResolveItemEffectStatsCounted(t *testing.T) {
	ch := census.Character{
		EquipmentSlots: []census.EquipmentSlot{
			{Name: "head", Item: census.EquippedItem{ID: 42}},
		},
	}
	catalog := func(id int64) (census.Item, bool) {
		if id == 42 {
			return census.Item{
				ID:    id,
				Slots: []census.Slot{{Name: "Head"}},
				EffectList: []census.Effect{
					{Description: "When Equipped:", Indentation: 0},
					{Description: "Increases Haste of caster by 25.0.", Indentation: 1},
				},
			}, true
		}
		return census.Item{}, false
	}
	f, missItems := Resolve(ch, catalog, func(int64) map[string]float64 { return nil }, func(string) bool { return true })

	require.Empty(t, missItems)
	require.Len(t, f.Slots, 1)
	require.InDelta(t, 25, f.Slots[0].Stats.HasteEffect, 1e-9)
}

func TestResolveRoutesEffectHasteToHasteEffect(t *testing.T) {
	ch := census.Character{EquipmentSlots: []census.EquipmentSlot{
		{Name: "cloak", Item: census.EquippedItem{ID: 1}},
	}}
	catalog := func(id int64) (census.Item, bool) {
		it := census.Item{ID: 1, DisplayName: "Cloak of Flames", Slots: []census.Slot{{Name: "Cloak"}}}
		return it, true
	}
	effects := func(id int64) map[string]float64 { return map[string]float64{"attackspeed": 25} }
	optimizable := func(string) bool { return true }

	f, miss := Resolve(ch, catalog, effects, optimizable)
	require.Empty(t, miss)
	require.Len(t, f.Slots, 1)
	require.InDelta(t, 0, f.Slots[0].Stats.Haste, 1e-9)
	require.InDelta(t, 25, f.Slots[0].Stats.HasteEffect, 1e-9)
}

func TestResolveReportsMissing(t *testing.T) {
	ch := census.Character{EquipmentSlots: []census.EquipmentSlot{
		{Name: "head", Item: census.EquippedItem{ID: 999}},
	}}
	none := func(id int64) (census.Item, bool) { return census.Item{}, false }
	f, missItems := Resolve(ch, none, func(int64) map[string]float64 { return nil }, func(string) bool { return true })

	require.Equal(t, []int64{999}, missItems)
	require.Empty(t, f.Slots, "unresolved item produces no slot entry")
}
