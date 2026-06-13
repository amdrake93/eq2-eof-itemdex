package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBuildSlotReports(t *testing.T) {
	lo := testLoadout()
	mk := func(id int, tier string, flurry float64) store.ScorableItem {
		return store.ScorableItem{ID: id, Name: "i", Slot: "Ear", Tier: tier, Stats: model.StatBlock{Flurry: flurry}}
	}
	bySlot := map[string][]store.ScorableItem{
		"Ear": {mk(1, "FABLED", 5), mk(2, "FABLED", 20), mk(3, "FABLED", 12), mk(4, "FABLED", 1),
			mk(5, "LEGENDARY", 8), mk(6, "MYTHICAL", 50)},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12, 1.0)
	weights := ConvergedWeights(set)
	reports := BuildSlotReports(set, bySlot, weights, 3)

	require.Len(t, reports, 1)
	r := reports[0]
	require.Equal(t, "Ear", r.Slot)
	require.Len(t, r.Chosen, 2) // Ear capacity 2

	// merged priority list: top-3 across ALL candidates by Delta (regardless of
	// rarity tier). The mythical flurry-50 item has the highest delta.
	require.Len(t, r.Ranked, 3)
	require.Equal(t, 6, r.Ranked[0].Item.ID)
	require.NotEmpty(t, r.Ranked[0].Terms)
	require.GreaterOrEqual(t, r.Ranked[0].Delta, r.Ranked[1].Delta)
	require.GreaterOrEqual(t, r.Ranked[1].Delta, r.Ranked[2].Delta)
}

func TestBuildSlotReportsSkipsPrimary(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Primary": {{ID: 1, Slot: "Primary", Tier: "FABLED", WieldStyle: "One-Handed", WeaponAvg: 160, WeaponDelay: 4}},
		"Chest":   {{ID: 2, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Potency: 30}}},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12, 1.0)
	reports := BuildSlotReports(set, bySlot, ConvergedWeights(set), 3)

	for _, r := range reports {
		require.NotEqual(t, "Primary", r.Slot, "main-hand slot is fixed; it should not be ranked")
	}
}
