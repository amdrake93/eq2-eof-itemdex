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

func TestCurveStatMarginalMultiAttack(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// bracket (30,40): (40*1.43 - 40*1.33)/10 = (57.2-53.2)/10 = 0.4
	require.InDelta(t, 0.4, curveStatMarginal(StatBlock{MultiAttack: 34.2}, "multiattack", dps), 1e-9)
}

func TestCurveStatMarginalHaste(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// bracket (0,10): @haste10 effect 12 → swings 160/(4/1.12)=44.8; @haste0=40; (44.8-40)/10=0.48
	require.InDelta(t, 0.48, curveStatMarginal(StatBlock{}, "haste", dps), 1e-6)
}

func TestDPSModWeightLinearViaStandardPath(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// +1 forward diff: 40*1.01 - 40 = 0.4
	require.InDelta(t, 0.4, DeriveWeights(StatBlock{}, dps)["dpsmod"], 1e-9)
	// at cap: +1 → min(201,200)=200 → no change → ~0
	require.InDelta(t, 0.0, DeriveWeights(StatBlock{DPSMod: 200}, dps)["dpsmod"], 1e-9)
}

func TestDeriveWeightsMultiAttackIntegration(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.4, DeriveWeights(StatBlock{MultiAttack: 34.2}, dps)["multiattack"], 1e-9)
}
