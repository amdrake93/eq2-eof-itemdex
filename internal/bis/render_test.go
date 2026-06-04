package bis

import (
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

	out := Render(reports)

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
