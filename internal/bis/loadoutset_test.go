package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

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
