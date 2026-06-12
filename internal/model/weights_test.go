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
	weights := DeriveWeights(StatBlock{DPSMod: 300}, dps)
	require.InDelta(t, 0.0, weights["dpsmod"], 1e-6)
}

func TestCurveStatMarginalMultiAttack(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// bracket (30,40): (40*1.43 - 40*1.33)/10 = (57.2-53.2)/10 = 0.4
	require.InDelta(t, 0.4, curveStatMarginal(StatBlock{MultiAttack: 34.2}, "multiattack", dps), 1e-9)
}

func TestCurveStatMarginalHaste(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// At haste 0 the bracket is the curve's (0,1) effect crossings: (0, 1.2520).
	// dps: 40·1.00 → 40·1.01; marginal = 0.4 / 1.2520 = 0.3195
	require.InDelta(t, 0.3195, curveStatMarginal(StatBlock{}, "haste", dps), 1e-3)
}

func TestCurveStatMarginalHasteAtCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{Haste: 300}, "haste", dps), 1e-9)
}

func TestCurveStatMarginalDPSMod(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.3195, curveStatMarginal(StatBlock{}, "dpsmod", dps), 1e-3)
}

func TestCurveStatMarginalDPSModAtCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{DPSMod: 300}, "dpsmod", dps), 1e-9)
}

func TestCurveStatMarginalDPSModRaidBaseline(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// At dps-mod 114.2 (raid baseline): f=74.80 → bracket = the (74,75) effect
	// crossings (112.63, 114.59); dps: 40·1.74 → 40·1.75; 0.4/1.9561 = 0.2045.
	// Nonzero — under the old 200-cap model the raid baseline read 0 here.
	require.InDelta(t, 0.2045, curveStatMarginal(StatBlock{DPSMod: 114.2}, "dpsmod", dps), 1e-3)
}

func TestCurveStatMarginalJustBelowCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// 285 → f=124.74: comfortably below the last integer crossing (f=125 at
	// ≈289), so a small future re-fit won't flip this into the dead zone.
	m := curveStatMarginal(StatBlock{Haste: 285}, "haste", dps)
	require.Greater(t, m, 0.0) // 285 was "capped → 0" under the old 200-cap model
	require.Less(t, m, 0.1)    // but the curve is nearly flat near its peak
}

func TestCurveStatMarginalDeadZoneBeforeCap(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	// Past the curve's last integer crossing (f=125 at ≈289) the floored effect
	// can never reach 126, so the marginal is legitimately zero before the cap.
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{Haste: 295}, "haste", dps), 1e-9)
}

func TestDeriveWeightsDPSModIntegration(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.3195, DeriveWeights(StatBlock{}, dps)["dpsmod"], 1e-3)
	require.InDelta(t, 0.3195, DeriveWeights(StatBlock{}, dps)["haste"], 1e-3)
	require.InDelta(t, 0.0, DeriveWeights(StatBlock{DPSMod: 300}, dps)["dpsmod"], 1e-6)
}

func TestDeriveWeightsMultiAttackIntegration(t *testing.T) {
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, Weapon{AvgDamage: 160, DelaySecs: 4}) }
	require.InDelta(t, 0.4, DeriveWeights(StatBlock{MultiAttack: 34.2}, dps)["multiattack"], 1e-9)
}
