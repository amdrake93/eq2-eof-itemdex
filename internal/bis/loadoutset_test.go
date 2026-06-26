package bis

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/amdrake93/eq2-eof-itemdex/internal/loadout"
	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
)

func TestOptimizableSlot(t *testing.T) {
	for _, s := range []string{"Primary", "Secondary", "Head", "Chest", "Finger", "Ear", "Wrist", "Cloak", "Waist"} {
		require.True(t, OptimizableSlot(s), s)
	}
	for _, s := range []string{"Charm", "Ranged", "Ammo", "Food"} {
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
	set, optimizable := SetFromLoadout(f, model.StatBlock{}, store.Loadout{}, 1.0, 600)
	seedMain(set)

	require.Contains(t, set.Equipped, "Cloak")
	require.Contains(t, set.Equipped, "Charm")
	require.Equal(t, map[string]bool{"Cloak": true}, optimizable)
	require.Greater(t, set.DPS(), 0.0)
}

func TestSetFromLoadoutSecondaryWeaponAffectsDPS(t *testing.T) {
	// A Secondary weapon ScorableItem with WeaponDelay>0 must be picked up by
	// offWeapon() and increase DPS relative to no off-hand.
	// Without off-hand
	fNoOH := loadout.File{}
	setNoOH, _ := SetFromLoadout(fNoOH, model.StatBlock{}, store.Loadout{}, 1.0, 600)
	seedMain(setNoOH)
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
	setWithOH, _ := SetFromLoadout(fWithOH, model.StatBlock{}, store.Loadout{}, 1.0, 600)
	seedMain(setWithOH)
	dpsWithOH := setWithOH.DPS()

	require.Greater(t, dpsWithOH, dpsNoOH, "Secondary weapon should increase DPS over no off-hand")
}

func TestTwoEffectHasteItemsDoNotStack(t *testing.T) {
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
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
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
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
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
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
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
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

func TestRankSlotUpgradesMultiCapAlwaysShowsBothInstances(t *testing.T) {
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
	// A very strong ring and a weak ring; the only candidate beats ONLY the weak one.
	set.Equipped["Finger"] = []store.ScorableItem{
		{ID: 10, Name: "StrongRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 100}},
		{ID: 11, Name: "WeakRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 5}},
	}
	optimizable := map[string]bool{"Finger": true}
	bySlot := map[string][]store.ScorableItem{
		"Finger": {{ID: 20, Name: "MidRing", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 50}}},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)

	// Both worn instances appear, even though StrongRing has no positive upgrade.
	require.Len(t, got, 2)

	byEquipped := map[string]SlotUpgrade{}
	for _, su := range got {
		byEquipped[su.EquippedName] = su
	}
	require.Contains(t, byEquipped, "WeakRing")
	require.Contains(t, byEquipped, "StrongRing")

	// WeakRing has a real upgrade; StrongRing has the zero-value "no upgrade" sentinel.
	require.Equal(t, "MidRing", byEquipped["WeakRing"].Best.Name)
	require.Equal(t, "", byEquipped["StrongRing"].Best.Name)
	require.Nil(t, byEquipped["StrongRing"].Alt)

	// No-upgrade rows sort last (Best.Delta 0).
	require.Equal(t, "StrongRing", got[1].EquippedName)
}

func TestRankSlotUpgradesSingleCapStillOmitsNoUpgrade(t *testing.T) {
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
	// Single-cap Head with a strong worn item the candidate cannot beat.
	set.Equipped["Head"] = []store.ScorableItem{
		{ID: 10, Name: "StrongHead", Slot: "Head", Stats: model.StatBlock{MultiAttack: 100}},
	}
	optimizable := map[string]bool{"Head": true}
	bySlot := map[string][]store.ScorableItem{
		"Head": {{ID: 20, Name: "WeakHead", Slot: "Head", Stats: model.StatBlock{MultiAttack: 5}}},
	}

	// No positive upgrade for a single-cap slot -> the slot is omitted entirely.
	require.Empty(t, RankSlotUpgrades(set, bySlot, optimizable, 0))
}

func TestRankSlotUpgradesDeterministicTieBreak(t *testing.T) {
	lo := store.Loadout{}
	set := NewSet(model.StatBlock{}, lo, 1.0, 600)
	seedMain(set)
	set.Equipped["Finger"] = []store.ScorableItem{{ID: 1, Name: "Worn", Slot: "Finger"}}
	optimizable := map[string]bool{"Finger": true}
	// Two candidates with identical stats => identical ΔDPS (a tie).
	bySlot := map[string][]store.ScorableItem{
		"Finger": {
			{ID: 77, Name: "RingHi", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 20}},
			{ID: 33, Name: "RingLo", Slot: "Finger", Stats: model.StatBlock{MultiAttack: 20}},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	// Tie broken by candidate id: lower id is Best.
	require.Equal(t, 33, got[0].Best.ID)
	require.Equal(t, 77, got[0].Alt.ID)

	// Whole result is stable across repeated runs.
	for i := 0; i < 5; i++ {
		require.Equal(t, got, RankSlotUpgrades(set, bySlot, optimizable, 0))
	}
}

func TestRankSlotUpgradesWeaponSlotsStayDistinct(t *testing.T) {
	set := NewSet(model.StatBlock{}, store.Loadout{}, 1.0, 600)
	seedMain(set) // worn main-hand has ID 1000
	set.Equipped["Secondary"] = []store.ScorableItem{{ID: 2, Slot: "Secondary", WeaponAvg: 100, WeaponDelay: 4}}
	optimizable := map[string]bool{"Secondary": true}
	// The pool offers the WORN main-hand (id 1000) as an off-hand option; it must be
	// excluded (one physical weapon), even though its raw delta would look positive.
	bySlot := map[string][]store.ScorableItem{
		"Secondary": {
			{ID: 1000, Slot: "Secondary", WeaponAvg: 160, WeaponDelay: 4},
			{ID: 50, Slot: "Secondary", WeaponAvg: 150, WeaponDelay: 4},
		},
	}

	got := RankSlotUpgrades(set, bySlot, optimizable, 0)
	require.NotEmpty(t, got)
	for _, su := range got {
		require.NotEqual(t, 1000, su.Best.ID, "must not suggest the worn main-hand as an off-hand upgrade")
		if su.Alt != nil {
			require.NotEqual(t, 1000, su.Alt.ID)
		}
	}
}
