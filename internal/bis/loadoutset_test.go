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

func TestRankSlotUpgrades(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// Finger is a two-capacity slot: one strong ring, one weak ring.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "StrongRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}},
		{ID: 11, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	// One candidate strong enough to beat BOTH worn rings, so both instances yield
	// a positive upgrade row.
	bySlot := map[string][]store.ScorableItem{
		"Finger": {
			{ID: 20, Name: "BigRing", Tier: "FABLED", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)

	// Two-capacity slot -> two instance rows, both labelled "Finger".
	require.Len(t, got, 2)
	require.Equal(t, "Finger", got[0].Slot)
	require.Equal(t, "Finger", got[1].Slot)

	// Rows are ranked by best delta: replacing the WEAK ring ranks first.
	require.Equal(t, "WeakRing", got[0].EquippedName)
	require.Equal(t, 11, got[0].EquippedID)
	require.Greater(t, got[0].EquippedValue, 0.0)
	require.Equal(t, "BigRing", got[0].Best.Name)
	require.Equal(t, 20, got[0].Best.ID)
	require.Greater(t, got[0].Best.Delta, got[1].Best.Delta)

	require.Equal(t, "StrongRing", got[1].EquippedName)
}

func TestRankSlotUpgradesEmptyPositionRow(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	// Only ONE ring worn in a two-capacity slot: the second position is empty.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "OnlyRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}},
	}
	optimizable := map[string]bool{"Finger": true}
	// NewRing beats OnlyRing, so the worn-item row is also a positive upgrade —
	// giving two rows total: the OnlyRing row and the Empty row.
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "NewRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}}},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	require.Len(t, got, 2)

	var foundEmpty bool
	for _, su := range got {
		if su.EquippedName == "Empty" {
			foundEmpty = true
			require.Equal(t, 0, su.EquippedID)
			require.Equal(t, 0.0, su.EquippedValue)
		}
	}
	require.True(t, foundEmpty, "an unfilled position should produce an Empty row")
}

func TestRankSlotUpgradesTopNCapsRows(t *testing.T) {
	lo := store.Loadout{Main: model.Weapon{AvgDamage: 160, MinDamage: 100, MaxDamage: 220, DelaySecs: 4}}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
		{ID: 11, Name: "WeakRing2", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "BigRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 40}}},
	}

	require.Len(t, RankSlotUpgrades(set, bySlot, optimizable, 1), 1)
}
