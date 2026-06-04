package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSlotCandidatesOffHandAndFilter(t *testing.T) {
	items := []store.ScorableItem{
		{ID: 1, Name: "Fabled Rapier", Slot: "Primary", Tier: "FABLED", WieldStyle: "One-Handed", WeaponAvg: 144, WeaponDelay: 4},
		{ID: 2, Name: "Sky Hunter's Stiletto", Slot: "Secondary", Tier: "LEGENDARY", WieldStyle: "One-Handed", WeaponAvg: 242, WeaponDelay: 6},
		{ID: 3, Name: "Fabled Chest", Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Potency: 30}},
		{ID: 4, Name: "Great Axe", Slot: "Primary", Tier: "FABLED", WieldStyle: "Two-Handed", WeaponAvg: 300, WeaponDelay: 6},
		{ID: 5, Name: "Soulfire Gladius", Slot: "Primary", Tier: "MYTHICAL", WieldStyle: "One-Handed", WeaponAvg: 160, WeaponDelay: 4},
	}
	keep := func(it store.ScorableItem) bool { return !IsHunters(it) }

	bySlot := SlotCandidates(items, keep)

	secIDs := map[int]bool{}
	for _, c := range bySlot["Secondary"] {
		secIDs[c.ID] = true
	}
	require.True(t, secIDs[1], "fabled 1H weapon must be an off-hand candidate")
	require.False(t, secIDs[2], "Hunter's stiletto filtered by keep")
	require.False(t, secIDs[4], "two-handed weapon is not an off-hand candidate")
	require.False(t, secIDs[5], "the single Soulfire is the fixed main-hand, not an off-hand option")

	require.Len(t, bySlot["Chest"], 1)
	require.Equal(t, 3, bySlot["Chest"][0].ID)
}
