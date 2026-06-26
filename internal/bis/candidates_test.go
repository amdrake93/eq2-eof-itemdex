package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSlotCandidatesWeaponPoolsAndFilter(t *testing.T) {
	items := []store.ScorableItem{
		{ID: 1, Name: "Fabled Rapier", Slot: "Primary", Tier: "FABLED", WieldStyle: "One-Handed", WeaponAvg: 144, WeaponDelay: 4},
		{ID: 2, Name: "Sky Hunter's Stiletto", Slot: "Secondary", Tier: "LEGENDARY", WieldStyle: "One-Handed", WeaponAvg: 242, WeaponDelay: 6},
		{ID: 3, Name: "Fabled Chest", Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Potency: 30}},
		{ID: 4, Name: "Great Axe", Slot: "Primary", Tier: "FABLED", WieldStyle: "Two-Handed", WeaponAvg: 300, WeaponDelay: 6},
		{ID: 5, Name: "Soulfire Gladius", Slot: "Primary", Tier: "MYTHICAL", WieldStyle: "One-Handed", WeaponAvg: 160, WeaponDelay: 4},
	}
	keep := func(it store.ScorableItem) bool { return !IsHunters(it) }

	bySlot := SlotCandidates(items, keep, []string{"One-Handed"}, true)

	ids := func(slot string) map[int]bool {
		m := map[int]bool{}
		for _, c := range bySlot[slot] {
			m[c.ID] = true
		}
		return m
	}
	main := ids("Primary")
	off := ids("Secondary")

	// One-handed weapons — including the single Soulfire — are candidates for BOTH
	// hands; the no-duplicate optimizer rule keeps one physical weapon out of both.
	require.True(t, main[1])
	require.True(t, off[1])
	require.True(t, main[5], "Soulfire is an ordinary weapon candidate")
	require.True(t, off[5])

	// Hunter's filtered by keep; two-handed excluded for a one-handed class.
	require.False(t, main[2])
	require.False(t, off[2])
	require.False(t, main[4])
	require.False(t, off[4])

	require.Len(t, bySlot["Chest"], 1)
	require.Equal(t, 3, bySlot["Chest"][0].ID)
}

func TestSlotCandidatesNoDualWieldHasNoOffHand(t *testing.T) {
	items := []store.ScorableItem{
		{ID: 1, Name: "Rapier", Slot: "Primary", WieldStyle: "One-Handed", WeaponAvg: 144, WeaponDelay: 4},
	}
	keep := func(store.ScorableItem) bool { return true }

	bySlot := SlotCandidates(items, keep, []string{"One-Handed"}, false)
	require.NotEmpty(t, bySlot["Primary"])
	_, hasOff := bySlot["Secondary"]
	require.False(t, hasOff, "a non-dual-wield class has no off-hand pool")
}
