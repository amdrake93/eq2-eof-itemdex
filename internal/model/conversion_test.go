package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffDelayHasteCurve(t *testing.T) {
	w := Weapon{AvgDamage: 100, DelaySecs: 4.0}
	// haste 200 → f = 0.800348·200 − 0.00127275·200² = 109.16 → floor 109
	require.InDelta(t, 4.0/2.09, effDelay(StatBlock{Haste: 200}, w), 1e-4)
	// haste 300 = hard cap → f(300) = 125.56 → floor 125
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 300}, w), 1e-4)
	// haste 400 → clamped to the 300 cap
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 400}, w), 1e-4)
}

func TestDPSModFactorCurveCapped(t *testing.T) {
	require.InDelta(t, 1.0, dpsModFactor(StatBlock{}), 1e-9)
	require.InDelta(t, 1.67, dpsModFactor(StatBlock{DPSMod: 100}), 1e-9) // f=67.31 → 67
	require.InDelta(t, 2.09, dpsModFactor(StatBlock{DPSMod: 200}), 1e-9) // f=109.16 → 109
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 300}), 1e-9) // hard cap
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 500}), 1e-9) // overcap clamps
}

func TestFlurryFactorNoHasteContribution(t *testing.T) {
	// Haste no longer feeds flurry: with Flurry 0 and Haste 300, factor == 1.0.
	require.Equal(t, 1.0, flurryFactor(StatBlock{Haste: 300}))
	// Gear flurry only: 10 → 1 + 0.10*3 = 1.30.
	require.InDelta(t, 1.30, flurryFactor(StatBlock{Flurry: 10}), 1e-9)
}

func TestAutoDPSMultiAttackCurve(t *testing.T) {
	w := Weapon{AvgDamage: 160, DelaySecs: 4} // 40 swings/sec at zero haste
	require.InDelta(t, 40*1.37, AutoDPS(StatBlock{MultiAttack: 34.2}, w), 1e-9)
	require.InDelta(t, 40*1.91, AutoDPS(StatBlock{MultiAttack: 100}, w), 1e-9)
	require.InDelta(t, 40*2.02, AutoDPS(StatBlock{MultiAttack: 120}, w), 1e-9)
}
