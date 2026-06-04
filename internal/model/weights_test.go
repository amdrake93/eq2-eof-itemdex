package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestDeriveWeights(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) + CADPS(sb, cas) }
	weights := DeriveWeights(StatBlock{}, dps)
	for _, k := range WeightStats {
		_, ok := weights[k]
		require.True(t, ok, k)
	}
	require.Greater(t, weights["critchance"], 0.0)
	require.Greater(t, weights["dpsmod"], 0.0)
}

func TestDPSModWeightZeroAtCap(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) }
	weights := DeriveWeights(StatBlock{DPSMod: 200}, dps)
	require.InDelta(t, 0.0, weights["dpsmod"], 1e-6)
}
