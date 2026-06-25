package model

import (
	"math"
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestDeriveWeights(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 10}}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) + CADPS(sb, cas, 600) }
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

func TestCastSpeedWeightUsesPlainDiff(t *testing.T) {
	// castspeed now uses the standard +1 forward diff; fight-length smoothing
	// removes the cast-boundary quantization that required the old wide span.
	dps := func(sb StatBlock) float64 { return 100 + sb.CastSpeed } // 1/pt trend
	w := DeriveWeights(StatBlock{}, dps)
	require.InDelta(t, 1.0, w["castspeed"], 1e-9) // (101−100)/1
}

func TestWeightStatsIncludeMainStat(t *testing.T) {
	require.Contains(t, WeightStats, "mainstat")
	require.NotContains(t, WeightStats, "potencybonus") // calibrated config value, not a gear stat
}

func TestCurveStatMarginalMainStat(t *testing.T) {
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return CADPS(sb, cas, 600) }
	// At mainstat 700 the bracket is the (695, 738) samples; positive marginal
	// on a CA-only dps closure (mainstat multiplies CA damage).
	// Derivation: slot = 0.5s (recovery-bound, RecoverySpeed 100 → instant cast);
	// CADPS@695 = 1000×1.5522/0.5 = 3104.4; @738 = 1000×1.5710/0.5 = 3142.0;
	// marginal = (3142.0−3104.4)/(738−695) = 37.6/43 = 0.87442.
	m := curveStatMarginal(StatBlock{MainStat: 700, RecoverySpeed: 100}, "mainstat", dps)
	require.InDelta(t, 0.8744, m, 1e-3)
}

func TestCurveStatMarginalMainStatAtCap(t *testing.T) {
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return CADPS(sb, cas, 600) }
	require.InDelta(t, 0.0, curveStatMarginal(StatBlock{MainStat: 1100, RecoverySpeed: 100}, "mainstat", dps), 1e-9)
}

func TestReuseWeightUsesPlainDiff(t *testing.T) {
	// reuse now uses the standard +1 forward diff; fight-length smoothing removes
	// the cast-boundary quantization that required the old centered ±4 span.
	dps := func(sb StatBlock) float64 { return 1000 + 2*sb.Reuse } // 2/pt trend
	w := DeriveWeights(StatBlock{Reuse: 40}, dps)
	require.InDelta(t, 2.0, w["reuse"], 1e-9) // (1082−1080)/1
}

func TestWeightStatsIncludeCastSpeedNotRecovery(t *testing.T) {
	require.Contains(t, WeightStats, "castspeed")
	require.NotContains(t, WeightStats, "recoveryspeed") // not a gear stat in the EoF pool

	w := Weapon{AvgDamage: 100, DelaySecs: 2.0}
	cas := []spell.CombatArt{{Name: "X", MinDamage: 800, MaxDamage: 1200, RecastSecs: 0}}
	dps := func(sb StatBlock) float64 { return AutoDPS(sb, w) + CADPS(sb, cas, 600) }
	weights := DeriveWeights(StatBlock{}, dps)
	_, ok := weights["castspeed"]
	require.True(t, ok)
	// A zero-recast (spammable) art makes the CA timeline cast-bound, so faster
	// casts must add DPS.
	require.Greater(t, weights["castspeed"], 0.0)
}

func TestHasteWeightUsesEffectiveHaste(t *testing.T) {
	dps := func(sb StatBlock) float64 {
		return AutoDPS(sb, Weapon{AvgDamage: 100, MinDamage: 100, MaxDamage: 100, DelaySecs: 4})
	}
	// Same effective haste (60) reached two ways must yield the same haste weight.
	wA := DeriveWeights(StatBlock{Haste: 60}, dps)["haste"]
	wB := DeriveWeights(StatBlock{Haste: 35, HasteEffect: 25}, dps)["haste"]
	require.InDelta(t, wA, wB, 1e-6)
	// And it must differ from evaluating as if HasteEffect were ignored (Haste=35).
	wIgnored := DeriveWeights(StatBlock{Haste: 35}, dps)["haste"]
	require.Greater(t, math.Abs(wB-wIgnored), 1e-6, "expected wB and wIgnored to differ by more than 1e-6")
}
