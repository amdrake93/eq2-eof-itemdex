package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBuildSetStopsStackingPastCap(t *testing.T) {
	lo := testLoadout()
	haste := func(id int, slot string) store.ScorableItem {
		return store.ScorableItem{ID: id, Slot: slot, Tier: "FABLED", Stats: model.StatBlock{Haste: 150}}
	}
	flurry := func(id int, slot string) store.ScorableItem {
		return store.ScorableItem{ID: id, Slot: slot, Tier: "FABLED", Stats: model.StatBlock{Flurry: 30}}
	}
	bySlot := map[string][]store.ScorableItem{
		"Head":  {haste(1, "Head"), flurry(2, "Head")},
		"Chest": {haste(3, "Chest"), flurry(4, "Chest")},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12, 1.0, 600)

	picks := []int{set.Equipped["Head"][0].ID, set.Equipped["Chest"][0].ID}
	hasteCount := 0
	for _, id := range picks {
		if id == 1 || id == 3 {
			hasteCount++
		}
	}
	require.Equal(t, 1, hasteCount, "exactly one haste item; the cap stops the second")
}

func TestBuildSetRespectsLocked(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Head": {{ID: 1, Slot: "Head", Tier: "FABLED", Stats: model.StatBlock{Potency: 5}}},
	}
	locked := map[string][]store.ScorableItem{
		"Chest": {{ID: 99, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Flurry: 50}}},
	}
	set := BuildSet(model.StatBlock{}, lo, bySlot, locked, 12, 1.0, 600)
	require.Equal(t, 99, set.Equipped["Chest"][0].ID)
	require.Equal(t, 1, set.Equipped["Head"][0].ID)
}

func TestBuildSetConverges(t *testing.T) {
	lo := testLoadout()
	bySlot := map[string][]store.ScorableItem{
		"Head":  {{ID: 1, Slot: "Head", Tier: "FABLED", Stats: model.StatBlock{Flurry: 10}}},
		"Chest": {{ID: 2, Slot: "Chest", Tier: "FABLED", Stats: model.StatBlock{Haste: 50}}},
	}
	a := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12, 1.0, 600)
	b := BuildSet(model.StatBlock{}, lo, bySlot, nil, 12, 1.0, 600)
	require.Equal(t, a.Equipped["Head"][0].ID, b.Equipped["Head"][0].ID)
	require.Equal(t, 1, a.Equipped["Head"][0].ID)
	require.Equal(t, 2, a.Equipped["Chest"][0].ID)
}
