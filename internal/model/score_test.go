package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScoreItem(t *testing.T) {
	weights := map[string]float64{
		"reuse": 16.0, "potency": 7.0, "critchance": 2.0,
	}
	item := StatBlock{Potency: 35, CritChance: 22, Reuse: 4}

	total, terms := ScoreItem(weights, item)

	require.InDelta(t, 353.0, total, 1e-9) // 35*7 + 22*2 + 4*16
	require.Len(t, terms, 3)
	require.Equal(t, "potency", terms[0].Stat)
	require.InDelta(t, 245.0, terms[0].Contribution, 1e-9)
	require.InDelta(t, 35.0, terms[0].ItemValue, 1e-9)
	require.InDelta(t, 7.0, terms[0].Weight, 1e-9)
	require.Equal(t, "reuse", terms[1].Stat)
	require.Equal(t, "critchance", terms[2].Stat)
}

func TestScoreItemEmpty(t *testing.T) {
	total, terms := ScoreItem(map[string]float64{"potency": 7}, StatBlock{})
	require.InDelta(t, 0.0, total, 1e-9)
	require.Empty(t, terms)
}
