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
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12)
	weights := ConvergedWeights(set)
	reports := BuildSlotReports(set, bySlot, weights, 3)

	require.Len(t, reports, 1)
	r := reports[0]
	require.Equal(t, "Ear", r.Slot)
	require.Len(t, r.Chosen, 2) // Ear capacity 2

	require.Len(t, r.Fabled, 3)
	require.Equal(t, 2, r.Fabled[0].Item.ID) // flurry 20 strongest
	require.Greater(t, r.Fabled[0].Delta, r.Fabled[1].Delta)
	require.NotEmpty(t, r.Fabled[0].Terms)
	require.Len(t, r.Legendary, 1)
	require.Len(t, r.Mythical, 1)

	// merged priority list: top-3 across ALL candidates by Delta (regardless of
	// rarity tier). The mythical flurry-50 item has the highest delta.
	require.Len(t, r.Ranked, 3)
	require.Equal(t, 6, r.Ranked[0].Item.ID)
	require.GreaterOrEqual(t, r.Ranked[0].Delta, r.Ranked[1].Delta)
	require.GreaterOrEqual(t, r.Ranked[1].Delta, r.Ranked[2].Delta)
}
