package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCapacityOf(t *testing.T) {
	require.Equal(t, 2, capacityOf("Ear"))
	require.Equal(t, 2, capacityOf("Finger"))
	require.Equal(t, 1, capacityOf("Chest"))
}

func TestPickBestRespectsCapacityAndContext(t *testing.T) {
	set := NewSet(model.StatBlock{}, testLoadout(), 1.0, 600)
	cands := []store.ScorableItem{
		{ID: 1, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Flurry: 20}},
		{ID: 2, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Haste: 100}},
		{ID: 3, Slot: "Ear", Tier: "FABLED", Stats: model.StatBlock{Flurry: 20}},
	}
	got := pickBest(set, "Ear", cands)
	require.Len(t, got, 2)
	ids := map[int]bool{got[0].ID: true, got[1].ID: true}
	require.True(t, ids[2], "should include the haste item (a fresh multiplier)")
}
