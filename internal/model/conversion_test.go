package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffDelayHasteCurve(t *testing.T) {
	w := Weapon{AvgDamage: 160, DelaySecs: 4}
	// haste 200 → effect 125 → effDelay = 4/2.25 ≈ 1.7778
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 200}, w), 1e-4)
	// haste 300 → clamped to stat cap 200 → same effDelay
	require.InDelta(t, 4.0/2.25, effDelay(StatBlock{Haste: 300}, w), 1e-4)
}

func TestDPSModFactorCurveCapped(t *testing.T) {
	require.Equal(t, 1.0, dpsModFactor(StatBlock{}))
	require.InDelta(t, 1.66, dpsModFactor(StatBlock{DPSMod: 100}), 1e-9) // effect 66
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 200}), 1e-9) // effect 125
	require.InDelta(t, 2.25, dpsModFactor(StatBlock{DPSMod: 300}), 1e-9) // capped
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
