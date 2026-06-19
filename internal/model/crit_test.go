package model

import (
	"testing"

	"github.com/amdrake93/eq2-eof-itemdex/internal/spell"
	"github.com/stretchr/testify/require"
)

func TestCritFactorHybrid(t *testing.T) {
	require.InDelta(t, 1.0, critFactor(StatBlock{CritChance: 0}, 139, 775), 1e-9)
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 100, 100), 1e-9)
	require.InDelta(t, 1.50, critFactor(StatBlock{CritChance: 100}, 316, 386), 1e-3)
	require.InDelta(t, 1.511, critFactor(StatBlock{CritChance: 100}, 215, 359), 1e-3)
	require.InDelta(t, 1.868, critFactor(StatBlock{CritChance: 100}, 139, 775), 1e-3)
	full := critFactor(StatBlock{CritChance: 100}, 139, 775)
	half := critFactor(StatBlock{CritChance: 50}, 139, 775)
	require.InDelta(t, 1+0.5*(full-1), half, 1e-9)
	require.InDelta(t, full, critFactor(StatBlock{CritChance: 150}, 139, 775), 1e-9)
	require.InDelta(t, 1.64, critFactor(StatBlock{CritChance: 100, CritBonus: 14}, 50, 50), 1e-9)
}

func TestCritAbilityModRides(t *testing.T) {
	ca := spell.CombatArt{
		Name:       "T",
		Components: []spell.Component{{Kind: spell.DirectHit, MinDamage: 100, MaxDamage: 100}},
	}
	got := CAEffectiveDamage(StatBlock{CritChance: 100, AbilityMod: 68}, ca)
	require.InDelta(t, (100+68)*1.5, got, 1e-6)
}

// TestCritCalibration2026_06_19 pins the crit model to the live tooltip/log reads.
// Range-shift floor confirmed via the auto-attack pile-up at max+1 (data/autoattacktest.txt:
// 11/14 crits exactly 776 on a 139–775 weapon; empirical avg crit/non-crit = 1.85).
func TestCritCalibration2026_06_19(t *testing.T) {
	full := func(lo, hi float64) float64 { return critFactor(StatBlock{CritChance: 100}, lo, hi) }

	// Single-valued (Strike of Consistency, Quick Strike DoT): flat ×1.50.
	require.InDelta(t, 1.50, full(61, 61), 1e-9)

	// Narrow range below 1.5:1 (Hilt Strike 316–386): still flat ×1.50 — its crits
	// exceeded any pure range-shift ceiling, ruling out range-shift-only.
	require.InDelta(t, 1.50, full(316, 386), 1e-3)

	// Typical CA 1.667:1 (Quick Strike 285–475): floor grazes bottom → ~×1.511.
	require.InDelta(t, 1.511, full(285, 475), 1e-3)

	// Wide weapon (Modinthalis 139–775): floor dominates → ~×1.868 (measured 1.85,
	// within ~1% — the modeled value is conservative vs the slightly low-skewed log).
	require.InDelta(t, 1.85, full(139, 775), 0.03)
}
