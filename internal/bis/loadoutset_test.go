package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func TestUpgradeDeltaIsSwapGainNotStandalone(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	equipped := store.ScorableItem{ID: 1, Name: "Worn", Slot: "Head", Stats: model.StatBlock{MultiAttack: 10}}
	set.Equipped["Head"] = []store.ScorableItem{equipped}

	same := store.ScorableItem{ID: 2, Name: "Same", Slot: "Head", Stats: model.StatBlock{MultiAttack: 10}}
	better := store.ScorableItem{ID: 3, Name: "Better", Slot: "Head", Stats: model.StatBlock{MultiAttack: 20}}

	// CandidateDelta is standalone-vs-empty: positive even for an equal item.
	require.Greater(t, set.CandidateDelta("Head", same), 0.0)

	// UpgradeDelta is the swap gain: ~0 for an equal item, and strictly less than the
	// equal item's inflated standalone CandidateDelta.
	require.InDelta(t, 0.0, set.UpgradeDelta("Head", same), 1e-9)
	require.Less(t, set.UpgradeDelta("Head", same), set.CandidateDelta("Head", same))

	// A genuinely better item shows a positive — but still modest — swap gain.
	up := set.UpgradeDelta("Head", better)
	require.Greater(t, up, 0.0)
	require.Less(t, up, set.CandidateDelta("Head", better))
}

func TestOptimizableSlot(t *testing.T) {
	for _, s := range []string{"Head", "Chest", "Finger", "Ear", "Wrist", "Cloak", "Waist", "Secondary"} {
		require.True(t, OptimizableSlot(s), s)
	}
	for _, s := range []string{"Charm", "Ranged", "Ammo", "Food", "Primary"} {
		require.False(t, OptimizableSlot(s), s)
	}
}

func TestSetFromLoadoutCountsFixedStats(t *testing.T) {
	f := loadout.File{
		Slots: []loadout.SlotEntry{
			{CatalogSlot: "Cloak", ItemID: 1, Name: "Cloak", Optimizable: true, Stats: model.StatBlock{Haste: 25}},
			{CatalogSlot: "Charm", ItemID: 2, Name: "Clicky", Optimizable: false, Stats: model.StatBlock{CritChance: 3}},
		},
	}
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, DelaySecs: 4}}
	set, optimizable := SetFromLoadout(f, model.StatBlock{}, lo, 1.0, 600)

	require.Contains(t, set.Equipped, "Cloak")
	require.Contains(t, set.Equipped, "Charm")
	require.Equal(t, map[string]bool{"Cloak": true}, optimizable)
	require.Greater(t, set.DPS(), 0.0)
}

func TestSetFromLoadoutSecondaryWeaponAffectsDPS(t *testing.T) {
	// A Secondary weapon ScorableItem with WeaponDelay>0 must be picked up by
	// offWeapon() and increase DPS relative to no off-hand.
	mainWeapon := model.Weapon{AvgDamage: 160, MinDamage: 80, MaxDamage: 240, DelaySecs: 4}
	lo := store.Loadout{Main: mainWeapon}

	// Without off-hand
	fNoOH := loadout.File{}
	setNoOH, _ := SetFromLoadout(fNoOH, model.StatBlock{}, lo, 1.0, 600)
	dpsNoOH := setNoOH.DPS()

	// With off-hand Secondary weapon
	fWithOH := loadout.File{
		Slots: []loadout.SlotEntry{
			{
				CatalogSlot: "Secondary",
				ItemID:      99,
				Name:        "Offhand Dagger",
				Optimizable: true,
				WeaponMin:   50,
				WeaponMax:   100,
				WeaponDelay: 3.0,
			},
		},
	}
	setWithOH, _ := SetFromLoadout(fWithOH, model.StatBlock{}, lo, 1.0, 600)
	dpsWithOH := setWithOH.DPS()

	require.Greater(t, dpsWithOH, dpsNoOH, "Secondary weapon should increase DPS over no off-hand")
}

func TestTwoEffectHasteItemsDoNotStack(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Cloak"] = []store.ScorableItem{
		{ID: 1, Name: "Cloak of Flames", Slot: "Cloak", Stats: model.StatBlock{HasteEffect: 25}},
	}

	// A second effect-haste item (21) is REDUNDANT — its haste shouldn't add (max-wins),
	// so its CandidateDelta for the Hands slot is ~0 (it has no other stats).
	redundant := store.ScorableItem{ID: 2, Name: "Lesser Haste Gloves", Slot: "Hands", Stats: model.StatBlock{HasteEffect: 21}}
	require.InDelta(t, 0, set.CandidateDelta("Hands", redundant), 1e-6)

	// A BIGGER effect-haste item (35) DOES help — it raises the max from 25 to 35.
	bigger := store.ScorableItem{ID: 3, Name: "Greater Haste Gloves", Slot: "Hands", Stats: model.StatBlock{HasteEffect: 35}}
	require.Greater(t, set.CandidateDelta("Hands", bigger), 0.0)
}
