package bis

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/model"
	"github.com/amdrake93/eq2-eof-itemdex/internal/store"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	weights := map[string]float64{"reuse": 16.67, "potency": 7.19}
	reports := []SlotReport{{
		Slot:   "Chest",
		Chosen: []store.ScorableItem{{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"}},
		Fabled: []ScoredItem{{
			Item:  store.ScorableItem{ID: 2, Name: "Fabled Chest", Tier: "FABLED", GameLink: "LINK2"},
			Delta: 41.2,
			Terms: []model.ScoreTerm{{Stat: "potency", ItemValue: 35, Weight: 7.19, Contribution: 251.65}},
		}},
	}}

	out := Render([]BaselineReport{{Name: "RAID", Weights: weights, Reports: reports}})

	require.Contains(t, out, "## RAID")
	require.Contains(t, out, "### Chest")
	require.Contains(t, out, "BiS: **Fabled Chest**")
	require.Contains(t, out, "+41.2 DPS")
	require.Contains(t, out, "LINK2")
	require.Contains(t, out, "potency 35 × 7.19")
	require.Contains(t, out, "reuse")
	require.Contains(t, out, "Assumptions")
	require.Contains(t, out, "in-context ΔDPS")
}
