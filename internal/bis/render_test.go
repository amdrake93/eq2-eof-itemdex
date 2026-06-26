package bis

import (
	"strings"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	weights := map[string]float64{"reuse": 16.67, "potency": 7.19}
	reports := []BaselineReport{{
		Name:    "RAID",
		Weights: weights,
		Reports: []SlotReport{{
			Slot:   "Chest",
			Chosen: []store.ScorableItem{{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"}},
			Ranked: []ScoredItem{
				{Item: store.ScorableItem{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"},
					Delta: 41.2, Terms: []model.ScoreTerm{{Stat: "potency", ItemValue: 35, Weight: 7.19, Contribution: 251.65}}},
				{Item: store.ScorableItem{ID: 9, Name: "Avatar Robe", Tier: "MYTHICAL"}, Delta: 60.0},
			},
		}},
	}}

	out := Render(reports, 600)

	require.Contains(t, out, "## RAID")
	require.Contains(t, out, "### Chest")
	require.Contains(t, out, "BiS: **Fabled Chest**")
	require.Contains(t, out, "Fabled Chest")
	require.Contains(t, out, "+41.2 DPS")
	require.Contains(t, out, "[FABLED]")
	require.Contains(t, out, "[MYTHICAL · avatar]")
	require.Contains(t, out, "potency 35 × 7.19")
	require.Contains(t, out, "reuse")
	require.Contains(t, out, "Assumptions")
}

func TestRenderFixedSlot(t *testing.T) {
	reports := []BaselineReport{{
		Name:    "RAID",
		Weights: map[string]float64{"potency": 7.0},
		Reports: []SlotReport{{
			Slot:   "Primary",
			Chosen: []store.ScorableItem{{Name: "Soulfire Gladius", Tier: "MYTHICAL"}},
			Ranked: nil,
		}},
	}}
	out := Render(reports, 600)
	require.Contains(t, out, "### Primary")
	require.Contains(t, out, "BiS: **Soulfire Gladius** _(fixed)_")
}

func TestRenderProgression(t *testing.T) {
	mk := func(name, tier string, d float64) SlotReport {
		return SlotReport{
			Slot:   "Chest",
			Ranked: []ScoredItem{{Item: store.ScorableItem{Name: name, Tier: tier}, Delta: d}},
		}
	}
	reports := []BaselineReport{
		{Name: "PRE-RAID", Reports: []SlotReport{mk("Dungeon Robe", "LEGENDARY", 30)}},
		{Name: "RAID", Reports: []SlotReport{mk("Fabled Chest", "FABLED", 50)}},
		{Name: "BEST-OF-BEST", Reports: []SlotReport{mk("Avatar Robe", "MYTHICAL", 70)}},
	}
	out := Render(reports, 600)

	require.Contains(t, out, "## Progression")
	chest := out[strings.Index(out, "## Progression"):]
	require.Contains(t, chest, "### Chest")
	pre := strings.Index(chest, "Dungeon Robe")
	raid := strings.Index(chest, "Fabled Chest")
	best := strings.Index(chest, "Avatar Robe")
	require.True(t, pre >= 0 && raid > pre && best > raid, "progression order pre-raid → raid → best")
}

func TestEQ2ULink(t *testing.T) {
	require.Equal(t,
		"[Cloak of Flames](https://u.eq2wire.com/item/264598753)",
		EQ2ULink("Cloak of Flames", 264598753))

	// No catalog id -> plain text, no link.
	require.Equal(t, "Empty", EQ2ULink("Empty", 0))
	require.Equal(t, "Mystery", EQ2ULink("Mystery", -1))
}
